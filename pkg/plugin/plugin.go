// Copyright 2018 Red Hat, Inc.
// Copyright 2014 CNI authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Go version 1.10 or greater is required. Before that, switching namespaces in
// long running processes in go did not work in a reliable way.

package plugin

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os/exec"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

const macSetupRetries = 2

type netConf struct {
	types.NetConf
	BrName  string `json:"bridge"`
	VlanTag *uint  `json:"vlan"`
}

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func logCall(command string, args *skel.CmdArgs) {
	log.Printf("CNI %s was called for container ID: %s, network namespace %s, interface name %s, configuration: %s",
		command, args.ContainerID, args.Netns, args.IfName, string(args.StdinData[:]))
}

func assertOvsAvailable() error {
	// ovs-vsctl show will fail if OVS is not installed, running or user does
	// not have rights to use it
	if err := exec.Command("ovs-vsctl", "show").Run(); err != nil {
		return fmt.Errorf("Open vSwitch is not available: %v", err)
	}
	return nil
}

func loadNetConf(bytes []byte) (*netConf, error) {
	netconf := &netConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	if netconf.BrName == "" {
		return nil, fmt.Errorf("\"bridge\" is a required argument")
	}

	return netconf, nil
}

func setupBridge(brName string) (*current.Interface, error) {
	brLink, err := netlink.LinkByName(brName)
	if err != nil {
		return nil, err
	}

	return &current.Interface{
		Name: brName,
		Mac:  brLink.Attrs().HardwareAddr.String(),
	}, nil
}

func generateRandomMac() net.HardwareAddr {
	prefix := []byte{0x02, 0x00, 0x00} // local unicast prefix
	suffix := make([]byte, 3)
	_, err := rand.Read(suffix)
	if err != nil {
		panic(err)
	}
	return net.HardwareAddr(append(prefix, suffix...))
}

func setupVeth(contNetns ns.NetNS, contIfaceName string) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}

	// Enter container network namespace and create veth pair inside. Doing
	// this we will make sure that both ends of the veth pair will be removed
	// when the container is gone.
	err := contNetns.Do(func(hostNetns ns.NetNS) error {
		hostVeth, containerVeth, err := ip.SetupVeth(contIfaceName, 1500, hostNetns)
		if err != nil {
			return err
		}

		containerLink, err := netlink.LinkByName(containerVeth.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup %q: %v", containerVeth.Name, err)
		}

		// In case the MAC address is already assigned to another interface, retry
		var containerMac net.HardwareAddr
		for i := 1; i <= macSetupRetries; i++ {
			containerMac = generateRandomMac()
			err = netlink.LinkSetHardwareAddr(containerLink, containerMac)
			if err != nil && i == macSetupRetries {
				return fmt.Errorf("failed to set container iface %q MAC %q: %v", containerVeth.Name, containerMac.String(), err)
			}
		}

		contIface.Name = containerVeth.Name
		contIface.Mac = containerMac.String()
		contIface.Sandbox = contNetns.Path()
		hostIface.Name = hostVeth.Name
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// Refetch the hostIface since its MAC address may change during network namespace move.
	if err = refetchIface(hostIface); err != nil {
		return nil, nil, err
	}

	return hostIface, contIface, nil
}

func attachIfaceToBridge(hostIfaceName string, contIfaceName string, brName string, vlanTag *uint, contNetnsPath string) error {
	// Set external IDs so we are able to find and remove the port from OVS
	// database when CNI DEL is called.
	command := []string{
		"--", "add-port", brName, hostIfaceName,
		"--", "set", "Port", hostIfaceName, fmt.Sprintf("external-ids:contNetns=%s", contNetnsPath),
		"--", "set", "Port", hostIfaceName, fmt.Sprintf("external-ids:contIface=%s", contIfaceName),
	}
	if vlanTag != nil {
		command = append(command, "--", "set", "Port", hostIfaceName, fmt.Sprintf("tag=%d", *vlanTag))
	}

	output, err := exec.Command("ovs-vsctl", command...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to attach veth to bridge: %s", string(output[:]))
	}

	hostLink, err := netlink.LinkByName(hostIfaceName)
	if err != nil {
		return err
	}

	if err := netlink.LinkSetUp(hostLink); err != nil {
		return err
	}

	return nil
}

func refetchIface(iface *current.Interface) error {
	link, err := netlink.LinkByName(iface.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup %q: %v", iface.Name, err)
	}
	iface.Mac = link.Attrs().HardwareAddr.String()
	return nil
}

func CmdAdd(args *skel.CmdArgs) error {
	logCall("ADD", args)

	if err := assertOvsAvailable(); err != nil {
		return err
	}

	netconf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	brIface, err := setupBridge(netconf.BrName)
	if err != nil {
		return err
	}

	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer contNetns.Close()

	hostIface, contIface, err := setupVeth(contNetns, args.IfName)
	if err != nil {
		return err
	}

	if err = attachIfaceToBridge(hostIface.Name, contIface.Name, brIface.Name, netconf.VlanTag, args.Netns); err != nil {
		return err
	}

	// Refetch the bridge since its MAC address may change when the first
	// veth is added.
	if err = refetchIface(brIface); err != nil {
		return err
	}

	result := &current.Result{
		Interfaces: []*current.Interface{brIface, hostIface, contIface},
	}

	return types.PrintResult(result, netconf.CNIVersion)
}

func getOvsPortForContIface(contIface string, contNetnsPath string) (string, bool, error) {
	// External IDs were set on the port during ADD call.
	portsOutRaw, err := exec.Command(
		"ovs-vsctl", "--format=json", "--column=name",
		"find", "Port",
		fmt.Sprintf("external-ids:contNetns=%s", contNetnsPath),
		fmt.Sprintf("external-ids:contIface=%s", contIface),
	).Output()
	if err != nil {
		return "", false, err
	}

	portsOut := struct {
		Data [][]string
	}{}
	if err = json.Unmarshal(portsOutRaw, &portsOut); err != nil {
		return "", false, err
	}

	if len(portsOut.Data) == 0 {
		return "", false, nil
	}

	portName := portsOut.Data[0][0]
	return portName, true, nil
}

func removeOvsPort(bridge string, portName string) error {
	output, err := exec.Command("ovs-vsctl", "del-port", bridge, portName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove bridge %s port %s: %v", bridge, portName, string(output[:]))
	}

	return err
}

func CmdDel(args *skel.CmdArgs) error {
	logCall("DEL", args)

	if err := assertOvsAvailable(); err != nil {
		return err
	}

	netconf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	if args.Netns == "" {
		panic("This should never happen, if it does, it means caller does not pass container network namespace as a parameter and therefore OVS port cleanup will not work")
	}

	// Unlike veth pair, OVS port will not be automatically removed when
	// container namespace is gone. Find port matching DEL arguments and remove
	// it explicitly.
	portName, portFound, err := getOvsPortForContIface(args.IfName, args.Netns)
	if err != nil {
		return fmt.Errorf("Failed to obtain OVS port for given connection: %v", err)
	}

	// Do not return an error if the port was not found, it may have been
	// already removed by someone.
	if portFound {
		if err := removeOvsPort(netconf.BrName, portName); err != nil {
			return err
		}
	}

	// Delete can be called multiple times, so don't return an error if the
	// device is already removed.
	err = ns.WithNetNSPath(args.Netns, func(ns.NetNS) error {
		err = ip.DelLinkByName(args.IfName)
		if err != nil && err == ip.ErrLinkNotFound {
			return nil
		}
		return err
	})

	return err
}
