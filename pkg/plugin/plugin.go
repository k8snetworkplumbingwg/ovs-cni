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
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net"
	"runtime"
	"sort"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/config"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/utils"
)

const macSetupRetries = 2

// EnvArgs args containing common, desired mac and ovs port name
type EnvArgs struct {
	cnitypes.CommonArgs
	MAC     cnitypes.UnmarshallableString `json:"mac,omitempty"`
	OvnPort cnitypes.UnmarshallableString `json:"ovnPort,omitempty"`
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
		err := cnitypes.LoadArgs(envArgsString, &e)
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

func generateRandomMac() net.HardwareAddr {
	prefix := []byte{0x02, 0x00, 0x00} // local unicast prefix
	suffix := make([]byte, 3)
	_, err := rand.Read(suffix)
	if err != nil {
		panic(err)
	}
	return net.HardwareAddr(append(prefix, suffix...))
}

// IPAddrToHWAddr takes the four octets of IPv4 address (aa.bb.cc.dd, for example) and uses them in creating
// a MAC address (0A:58:AA:BB:CC:DD).  For IPv6, create a hash from the IPv6 string and use that for MAC Address.
// Assumption: the caller will ensure that an empty net.IP{} will NOT be passed.
// This method is copied from https://github.com/ovn-org/ovn-kubernetes/blob/master/go-controller/pkg/util/net.go
func IPAddrToHWAddr(ip net.IP) net.HardwareAddr {
	// Ensure that for IPv4, we are always working with the IP in 4-byte form.
	ip4 := ip.To4()
	if ip4 != nil {
		// safe to use private MAC prefix: 0A:58
		return net.HardwareAddr{0x0A, 0x58, ip4[0], ip4[1], ip4[2], ip4[3]}
	}

	hash := sha256.Sum256([]byte(ip.String()))
	return net.HardwareAddr{0x0A, 0x58, hash[0], hash[1], hash[2], hash[3]}
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

func splitVlanIds(trunks []*types.Trunk) ([]uint, error) {
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

// CmdAdd add handler for attaching container into network
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

	netconf, err := config.LoadConf(args.StdinData)
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

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, netconf.SocketFile)
	if err != nil {
		return err
	}

	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer contNetns.Close()

	var origIfName string
	if sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) {
		origIfName, err = sriov.GetVFLinkName(netconf.DeviceID)
		if err != nil {
			return err
		}
	}

	// Cache NetConf for CmdDel
	if err = utils.SaveCache(config.GetCRef(args.ContainerID, args.IfName),
		&types.CachedNetConf{Netconf: netconf, OrigIfName: origIfName}); err != nil {
		return fmt.Errorf("error saving NetConf %q", err)
	}

	var hostIface, contIface *current.Interface
	if sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) {
		hostIface, contIface, err = sriov.SetupSriovInterface(contNetns, args.ContainerID, args.IfName, netconf.MTU, netconf.DeviceID)
		if err != nil {
			return err
		}
	} else {
		hostIface, contIface, err = setupVeth(contNetns, args.IfName, mac, netconf.MTU)
		if err != nil {
			return err
		}
	}

	if err = attachIfaceToBridge(ovsDriver, hostIface.Name, contIface.Name, vlanTagNum, trunks, portType, args.Netns, ovnPort); err != nil {
		return err
	}

	result := &current.Result{
		Interfaces: []*current.Interface{hostIface, contIface},
	}

	// run the IPAM plugin
	if netconf.IPAM.Type != "" {
		r, err := ipam.ExecAdd(netconf.IPAM.Type, args.StdinData)
		defer func() {
			if err != nil {
				ipam.ExecDel(netconf.IPAM.Type, args.StdinData)
			}
		}()
		if err != nil {
			return fmt.Errorf("failed to set up IPAM plugin type %q: %v", netconf.IPAM.Type, err)
		}

		// Convert the IPAM result into the current Result type
		newResult, err := current.NewResultFromResult(r)
		if err != nil {
			return err
		}

		if len(newResult.IPs) == 0 {
			return errors.New("IPAM plugin returned missing IP config")
		}

		newResult.Interfaces = []*current.Interface{contIface}
		newResult.Interfaces[0].Mac = contIface.Mac

		for _, ipc := range newResult.IPs {
			// All addresses apply to the container interface
			ipc.Interface = current.Int(0)
		}

		// wait until OF port link state becomes up. This is needed to make
		// gratuitous arp for args.IfName to be sent over ovs bridge
		err = waitLinkUp(ovsDriver, hostIface.Name, netconf.LinkStateCheckRetries, netconf.LinkStateCheckInterval)
		if err != nil {
			return err
		}

		err = contNetns.Do(func(_ ns.NetNS) error {
			if mac == "" && !sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) && len(newResult.IPs) >= 1 {
				containerMac := IPAddrToHWAddr(newResult.IPs[0].Address.IP)
				containerLink, err := netlink.LinkByName(args.IfName)
				if err != nil {
					return fmt.Errorf("failed to lookup container interface %q: %v", args.IfName, err)
				}
				err = assignMacToLink(containerLink, containerMac, args.IfName)
				if err != nil {
					return err
				}
				newResult.Interfaces[0].Mac = containerMac.String()
			}
			err := ipam.ConfigureIface(args.IfName, newResult)
			if err != nil {
				return err
			}
			contVeth, err := net.InterfaceByName(args.IfName)
			if err != nil {
				return fmt.Errorf("failed to look up %q: %v", args.IfName, err)
			}
			for _, ipc := range newResult.IPs {
				if ipc.Version == "4" {
					// send gratuitous arp for other ends to refresh its arp cache
					err = arping.GratuitousArpOverIface(ipc.Address.IP, *contVeth)
					if err != nil {
						// ok to ignore returning this error
						log.Printf("error sending garp for ip %s: %v", ipc.Address.IP.String(), err)
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		result = newResult
		result.Interfaces = []*current.Interface{hostIface, result.Interfaces[0]}

		for ifIndex, ifCfg := range result.Interfaces {
			// Adjust interface index with new container interface index in result.Interfaces
			if ifCfg.Name == args.IfName {
				for ipIndex := range result.IPs {
					result.IPs[ipIndex].Interface = current.Int(ifIndex)
				}
			}
		}
	}

	return cnitypes.PrintResult(result, netconf.CNIVersion)
}

func waitLinkUp(ovsDriver *ovsdb.OvsBridgeDriver, ofPortName string, retryCount, interval int) error {
	checkInterval := time.Duration(interval) * time.Millisecond
	for i := 1; i <= retryCount; i++ {
		portState, err := ovsDriver.GetOFPortOpState(ofPortName)
		if err != nil {
			log.Printf("error in retrieving port %s state: %v", ofPortName, err)
		} else {
			if portState == "up" {
				break
			}
		}
		if i == retryCount {
			return fmt.Errorf("The OF port %s state is not up, try increasing number of retries/interval config parameter", ofPortName)
		}
		time.Sleep(checkInterval)
	}
	return nil
}

func getOvsPortForContIface(ovsDriver *ovsdb.OvsBridgeDriver, contIface string, contNetnsPath string) (string, bool, error) {
	// External IDs were set on the port during ADD call.
	return ovsDriver.GetOvsPortForContIface(contIface, contNetnsPath)
}

// cleanPorts removes all ports whose interfaces have an error.
func cleanPorts(ovsDriver *ovsdb.OvsBridgeDriver) error {
	ifaces, err := ovsDriver.FindInterfacesWithError()
	if err != nil {
		return fmt.Errorf("clean ports: %v", err)
	}
	for _, iface := range ifaces {
		log.Printf("Info: interface %s has error: removing corresponding port", iface)
		if err := ovsDriver.DeletePort(iface); err != nil {
			// Don't return an error here, just log its occurrence.
			// Something else may have removed the port already.
			log.Printf("Error: %v\n", err)
		}
	}
	return nil
}

func removeOvsPort(ovsDriver *ovsdb.OvsBridgeDriver, portName string) error {

	return ovsDriver.DeletePort(portName)
}

// CmdDel remove handler for deleting container from network
func CmdDel(args *skel.CmdArgs) error {
	logCall("DEL", args)

	cRef := config.GetCRef(args.ContainerID, args.IfName)
	cache, err := config.LoadConfFromCache(cRef)
	if err != nil {
		// If cmdDel() fails, cached netconf is cleaned up by
		// the followed defer call. However, subsequence calls
		// of cmdDel() from kubelet fail in a dead loop due to
		// cached netconf doesn't exist.
		// Return nil when loadConfFromCache fails since the rest
		// of cmdDel() code relies on netconf as input argument
		// and there is no meaning to continue.
		return nil
	}

	defer func() {
		if err == nil {
			utils.CleanCache(cRef)
		}
	}()

	envArgs, err := getEnvArgs(args.Args)
	if err != nil {
		return err
	}

	var ovnPort string
	if envArgs != nil {
		ovnPort = string(envArgs.OvnPort)
	}

	bridgeName, err := getBridgeName(cache.Netconf.BrName, ovnPort)
	if err != nil {
		return err
	}

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, cache.Netconf.SocketFile)
	if err != nil {
		return err
	}

	if cache.Netconf.IPAM.Type != "" {
		err = ipam.ExecDel(cache.Netconf.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	if args.Netns == "" {
		// The CNI_NETNS parameter may be empty according to version 0.4.0
		// of the CNI spec (https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md).
		if sriov.IsOvsHardwareOffloadEnabled(cache.Netconf.DeviceID) {
			// SR-IOV Case - The sriov device is moved into host network namespace when args.Netns is empty.
			// This happens container is killed due to an error (example: CrashLoopBackOff, OOMKilled)
			var rep string
			if rep, err = sriov.GetNetRepresentor(cache.Netconf.DeviceID); err != nil {
				return err
			}
			if err = removeOvsPort(ovsDriver, rep); err != nil {
				// Don't throw err as delete can be called multiple times because of error in ResetVF and ovs
				// port is already deleted in a previous invocation.
				log.Printf("Error: %v\n", err)
			}
			if err = sriov.ResetVF(args, cache.Netconf.DeviceID, cache.OrigIfName); err != nil {
				return err
			}
		} else {
			// In accordance with the spec we clean up as many resources as possible.
			if err := cleanPorts(ovsDriver); err != nil {
				return err
			}
		}
		return nil
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

	if sriov.IsOvsHardwareOffloadEnabled(cache.Netconf.DeviceID) {
		err = sriov.ReleaseVF(args, cache.OrigIfName)
		if err != nil {
			// try to reset vf into original state as much as possible in case of error
			sriov.ResetVF(args, cache.Netconf.DeviceID, cache.OrigIfName)
		}
	} else {
		err = ns.WithNetNSPath(args.Netns, func(ns.NetNS) error {
			err = ip.DelLinkByName(args.IfName)
			if err != nil {
				// clean up as many stale ovs resources as possible.
				cleanPorts(ovsDriver)
			}
			return err
		})
		// do the following as per cni spec (i.e. Plugins should generally complete a DEL action
		// without error even if some resources are missing)
		if _, ok := err.(ns.NSPathNotExistErr); ok || err == ip.ErrLinkNotFound {
			if portFound {
				ip.DelLinkByName(portName)
			}
			cleanPorts(ovsDriver)
			return nil
		}
	}

	return err
}

// CmdCheck check handler to make sure networking is as expected. yet to be implemented.
func CmdCheck(args *skel.CmdArgs) error {
	logCall("CHECK", args)
	log.Print("CHECK is not yet implemented, pretending everything is fine")
	return nil
}
