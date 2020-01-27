// Copyright 2018-2019 Red Hat, Inc.
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
// +build go1.10

package plugin

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"runtime"
	"sort"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"

	"github.com/kubevirt/ovs-cni/pkg/ovsdb"
)

const macSetupRetries = 2

type netConf struct {
	types.NetConf
	BrName  string   `json:"bridge,omitempty"`
	VlanTag *uint    `json:"vlan"`
	MTU     int      `json:"mtu"`
	Trunk   []*trunk `json:"trunk,omitempty"`
}

type trunk struct {
	MinID *uint `json:"minID,omitempty"`
	MaxID *uint `json:"maxID,omitempty"`
	ID    *uint `json:"id,omitempty"`
}

type EnvArgs struct {
	types.CommonArgs
	MAC     types.UnmarshallableString `json:"mac,omitempty"`
	OvnPort types.UnmarshallableString `json:"ovnPort,omitempty"`
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

func getEnvArgs(envArgsString string) (*EnvArgs, error) {
	if envArgsString != "" {
		e := EnvArgs{}
		err := types.LoadArgs(envArgsString, &e)
		if err != nil {
			return nil, err
		}
		return &e, nil
	}
	return nil, nil
}

func getHardwareAddr(ifName string) string {
	ifLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return ""
	}
	return ifLink.Attrs().HardwareAddr.String()

}

func loadNetConf(bytes []byte) (*netConf, error) {
	netconf := &netConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}

	return netconf, nil
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

func setupVeth(contNetns ns.NetNS, contIfaceName string, requestedMac string, mtu int) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}

	// Enter container network namespace and create veth pair inside. Doing
	// this we will make sure that both ends of the veth pair will be removed
	// when the container is gone.
	err := contNetns.Do(func(hostNetns ns.NetNS) error {
		hostVeth, containerVeth, err := ip.SetupVeth(contIfaceName, mtu, hostNetns)
		if err != nil {
			return err
		}

		containerLink, err := netlink.LinkByName(containerVeth.Name)
		if err != nil {
			return fmt.Errorf("failed to lookup %q: %v", containerVeth.Name, err)
		}

		var containerMac net.HardwareAddr
		if requestedMac != "" {
			containerMac, err = net.ParseMAC(requestedMac)
			if err != nil {
				return fmt.Errorf("failed to parse requested MAC  %q: %v", requestedMac, err)
			}
			err = assignMacToLink(containerLink, containerMac, containerVeth.Name)
			if err != nil {
				return err
			}
		} else {
			// In case the MAC address is already assigned to another interface, retry
			for i := 1; i <= macSetupRetries; i++ {
				containerMac = generateRandomMac()
				err = assignMacToLink(containerLink, containerMac, containerVeth.Name)
				if err != nil && i == macSetupRetries {
					return err
				}
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

func assignMacToLink(link netlink.Link, mac net.HardwareAddr, name string) error {
	err := netlink.LinkSetHardwareAddr(link, mac)
	if err != nil {
		return fmt.Errorf("failed to set container iface %q MAC %q: %v", name, mac.String(), err)
	}
	return nil
}

func getBridgeName(bridgeName, ovnPort string) (string, error) {
	if bridgeName != "" {
		return bridgeName, nil
	} else if bridgeName == "" && ovnPort != "" {
		return "br-int", nil
	}

	return "", fmt.Errorf("failed to get bridge name")
}

func attachIfaceToBridge(ovsDriver *ovsdb.OvsBridgeDriver, hostIfaceName string, contIfaceName string, vlanTag uint, trunks []uint, portType string, contNetnsPath string, ovnPortName string) error {
	err := ovsDriver.CreatePort(hostIfaceName, contNetnsPath, contIfaceName, ovnPortName, vlanTag, trunks, portType)
	if err != nil {
		return err
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
	iface.Mac = getHardwareAddr(iface.Name)
	return nil
}

func splitVlanIds(trunks []*trunk) ([]uint, error) {
	vlans := make(map[uint]bool)
	for _, item := range trunks {
		var minID uint = 0
		var maxID uint = 0
		if item.MinID != nil {
			minID = *item.MinID
			if minID < 0 || minID > 4096 {
				return nil, errors.New("incorrect trunk minID parameter")
			}
		}
		if item.MaxID != nil {
			maxID = *item.MaxID
			if maxID < 0 || maxID > 4096 {
				return nil, errors.New("incorrect trunk maxID parameter")
			}
			if maxID < minID {
				return nil, errors.New("minID is greater than maxID in trunk parameter")
			}
		}
		if minID > 0 && maxID > 0 {
			for v := minID; v <= maxID; v++ {
				vlans[v] = true
			}
		}
		var id uint = 0
		if item.ID != nil {
			id = *item.ID
			if id < 0 || minID > 4096 {
				return nil, errors.New("incorrect trunk id parameter")
			}
			vlans[id] = true
		}
	}
	if len(vlans) == 0 {
		return nil, errors.New("trunk parameter is misconfigured")
	}
	vlanIds := make([]uint, 0, len(vlans))
	for k := range vlans {
		vlanIds = append(vlanIds, k)
	}
	sort.Slice(vlanIds, func(i, j int) bool { return vlanIds[i] < vlanIds[j] })
	return vlanIds, nil
}

func CmdAdd(args *skel.CmdArgs) error {
	logCall("ADD", args)

	envArgs, err := getEnvArgs(args.Args)
	if err != nil {
		return err
	}

	var mac string
	var ovnPort string
	if envArgs != nil {
		mac = string(envArgs.MAC)
		ovnPort = string(envArgs.OvnPort)
	}

	netconf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	var vlanTagNum uint = 0
	trunks := make([]uint, 0)
	portType := "access"
	if netconf.VlanTag == nil || len(netconf.Trunk) > 0 {
		portType = "trunk"
		if len(netconf.Trunk) > 0 {
			trunkVlanIds, err := splitVlanIds(netconf.Trunk)
			if err != nil {
				return err
			}
			trunks = append(trunks, trunkVlanIds...)
		}
	} else if netconf.VlanTag != nil {
		vlanTagNum = *netconf.VlanTag
	}

	bridgeName, err := getBridgeName(netconf.BrName, ovnPort)
	if err != nil {
		return err
	}

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName)
	if err != nil {
		return err
	}

	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer contNetns.Close()

	hostIface, contIface, err := setupVeth(contNetns, args.IfName, mac, netconf.MTU)
	if err != nil {
		return err
	}

	if err = attachIfaceToBridge(ovsDriver, hostIface.Name, contIface.Name, vlanTagNum, trunks, portType, args.Netns, ovnPort); err != nil {
		return err
	}

	result := &current.Result{
		Interfaces: []*current.Interface{hostIface, contIface},
	}

	return types.PrintResult(result, netconf.CNIVersion)
}

func getOvsPortForContIface(ovsDriver *ovsdb.OvsBridgeDriver, contIface string, contNetnsPath string) (string, bool, error) {
	// External IDs were set on the port during ADD call.
	return ovsDriver.GetOvsPortForContIface(contIface, contNetnsPath)
}

func removeOvsPort(ovsDriver *ovsdb.OvsBridgeDriver, portName string) error {

	return ovsDriver.DeletePort(portName)
}

func CmdDel(args *skel.CmdArgs) error {
	logCall("DEL", args)

	if args.Netns == "" {
		panic("This should never happen, if it does, it means caller does not pass container network namespace as a parameter and therefore OVS port cleanup will not work")
	}

	envArgs, err := getEnvArgs(args.Args)
	if err != nil {
		return err
	}

	var ovnPort string
	if envArgs != nil {
		ovnPort = string(envArgs.OvnPort)
	}

	netconf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	bridgeName, err := getBridgeName(netconf.BrName, ovnPort)
	if err != nil {
		return err
	}

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName)
	if err != nil {
		return err
	}

	// Unlike veth pair, OVS port will not be automatically removed when
	// container namespace is gone. Find port matching DEL arguments and remove
	// it explicitly.
	portName, portFound, err := getOvsPortForContIface(ovsDriver, args.IfName, args.Netns)
	if err != nil {
		return fmt.Errorf("Failed to obtain OVS port for given connection: %v", err)
	}

	// Do not return an error if the port was not found, it may have been
	// already removed by someone.
	if portFound {
		if err := removeOvsPort(ovsDriver, portName); err != nil {
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

func CmdCheck(args *skel.CmdArgs) error {
	logCall("CHECK", args)
	log.Print("CHECK is not yet implemented, pretending everything is fine")
	return nil
}
