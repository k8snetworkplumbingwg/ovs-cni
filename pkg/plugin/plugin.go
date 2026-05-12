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
//go:build go1.10
// +build go1.10

package plugin

import (
	"errors"
	"fmt"
	"log"
	"net"
	"runtime"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/common"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/config"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/utils"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/veth"
)

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

func getEnvArgs(envArgsString string) (*types.EnvArgs, error) {
	if envArgsString != "" {
		e := types.EnvArgs{}
		err := cnitypes.LoadArgs(envArgsString, &e)
		if err != nil {
			return nil, err
		}
		return &e, nil
	}
	return nil, nil
}

func assignMacToLink(link netlink.Link, mac net.HardwareAddr, name string) error {
	err := netlink.LinkSetHardwareAddr(link, mac)
	if err != nil {
		return fmt.Errorf("failed to set container iface %q MAC %q: %v", name, mac.String(), err)
	}
	return nil
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
	var contPodUid string
	if envArgs != nil {
		mac = string(envArgs.MAC)
		ovnPort = string(envArgs.OvnPort)
		contPodUid = string(envArgs.K8S_POD_UID)
	}

	netconf, err := config.LoadConf(args.StdinData)
	if err != nil {
		return err
	}

	portCfg, err := common.ParseOvsPortConfig(netconf)
	if err != nil {
		return err
	}

	ovsDriver, err := ovsdb.NewOvsDriver(netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := sriov.GetBridgeName(ovsDriver, netconf.BrName, ovnPort, netconf.DeviceID)
	if err != nil {
		return err
	}
	// save discovered bridge name to the netconf struct to make
	// sure it is save in the cache.
	// we need to cache discovered bridge name to make sure that we will
	// use the right bridge name in CmdDel
	netconf.BrName = bridgeName

	ovsBridgeDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, netconf.SocketFile)
	if err != nil {
		return err
	}

	// check if the device driver is the type of userspace driver
	userspaceMode := false
	if sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) {
		userspaceMode, err = sriov.HasUserspaceDriver(netconf.DeviceID)
		if err != nil {
			return err
		}
	}

	// removes all ports whose interfaces have an error
	if err := common.CleanPorts(ovsBridgeDriver); err != nil {
		return err
	}

	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer func() { _ = contNetns.Close() }()

	// userspace driver does not create a network interface for the VF on the host
	var origIfName string
	if sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) && !userspaceMode {
		origIfName, err = sriov.GetVFLinkName(netconf.DeviceID)
		if err != nil {
			return err
		}
	}

	// Cache NetConf for CmdDel
	if err = utils.SaveCache(config.GetCRef(args.ContainerID, args.IfName),
		&types.CachedNetConf{Netconf: netconf, OrigIfName: origIfName, UserspaceMode: userspaceMode}); err != nil {
		return fmt.Errorf("error saving NetConf %q", err)
	}

	var hostIface, contIface *current.Interface
	if sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) {
		hostIface, contIface, err = sriov.SetupSriovInterface(contNetns, args.ContainerID, args.IfName, mac, netconf.MTU, netconf.DeviceID, userspaceMode)
		if err != nil {
			return err
		}
	} else {
		hostIface, contIface, err = veth.SetupVeth(contNetns, args.IfName, mac, netconf.MTU)
		if err != nil {
			return err
		}
	}

	if err = common.AttachIfaceToBridge(ovsBridgeDriver, hostIface.Name, contIface.Name, netconf.OfportRequest, portCfg.VlanTag, portCfg.Trunks, portCfg.Type, netconf.InterfaceType, args.Netns, ovnPort, contPodUid); err != nil {
		return err
	}

	// Refetch the host interface MAC since OVS may change it when
	// attaching the port to the bridge.
	if err = common.RefetchIface(hostIface); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			// Unlike veth pair, OVS port will not be automatically removed
			// if the following IPAM configuration fails and netns gets removed.
			_, _, err = common.CleanupOvsPortBestEffort(ovsBridgeDriver, args.IfName, args.Netns)
			if err != nil {
				log.Printf("Failed best-effort cleanup: %v", err)
			}
		}
	}()

	result := &current.Result{
		Interfaces: []*current.Interface{hostIface, contIface},
	}

	// run the IPAM plugin
	// userspace driver does not support IPAM plugin,
	// because there is no network interface for the VF on the host
	if netconf.IPAM.Type != "" && !userspaceMode {
		var r cnitypes.Result
		r, err = ipam.ExecAdd(netconf.IPAM.Type, args.StdinData)
		defer func() {
			if err != nil {
				if err := ipam.ExecDel(netconf.IPAM.Type, args.StdinData); err != nil {
					log.Printf("Failed best-effort cleanup IPAM configuration: %v", err)
				}
			}
		}()
		if err != nil {
			return fmt.Errorf("failed to set up IPAM plugin type %q: %v", netconf.IPAM.Type, err)
		}

		// Convert the IPAM result into the current Result type
		var newResult *current.Result
		newResult, err = current.NewResultFromResult(r)
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
		err = waitLinkUp(ovsBridgeDriver, hostIface.Name, netconf.LinkStateCheckRetries, netconf.LinkStateCheckInterval)
		if err != nil {
			return err
		}

		err = contNetns.Do(func(_ ns.NetNS) error {
			if mac == "" && !sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID) && len(newResult.IPs) >= 1 {
				containerMac := common.IPAddrToHWAddr(newResult.IPs[0].Address.IP)
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
				// if ip address version is 4
				if ipc.Address.IP.To4() != nil {
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
			if err := utils.CleanCache(cRef); err != nil {
				log.Printf("Failed cleaning up cache: %v", err)
			}
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
	ovsDriver, err := ovsdb.NewOvsDriver(cache.Netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := sriov.GetBridgeName(ovsDriver, cache.Netconf.BrName, ovnPort, cache.Netconf.DeviceID)
	if err != nil {
		return err
	}

	ovsBridgeDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, cache.Netconf.SocketFile)
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
			if err = common.RemoveOvsPort(ovsBridgeDriver, rep); err != nil {
				// Don't throw err as delete can be called multiple times because of error in ResetVF and ovs
				// port is already deleted in a previous invocation.
				log.Printf("Error: %v\n", err)
			}
			// there is no network interface in case of userspace driver, so OrigIfName is empty
			if !cache.UserspaceMode {
				if err = sriov.ResetVF(args, cache.Netconf.DeviceID, cache.OrigIfName); err != nil {
					return err
				}
			}
		} else {
			// In accordance with the spec we clean up as many resources as possible.
			if err := common.CleanPorts(ovsBridgeDriver); err != nil {
				return err
			}
		}
		return nil
	}

	// Unlike veth pair, OVS port will not be automatically removed when
	// container namespace is gone. Find port matching DEL arguments and remove
	// it explicitly.
	portName, portFound, err := common.CleanupOvsPortBestEffort(ovsBridgeDriver, args.IfName, args.Netns)
	if err != nil {
		return err
	}

	if sriov.IsOvsHardwareOffloadEnabled(cache.Netconf.DeviceID) {
		// there is no network interface in case of userspace driver, so OrigIfName is empty
		if !cache.UserspaceMode {
			err = sriov.ReleaseVF(args, cache.OrigIfName)
			if err != nil {
				// try to reset vf into original state as much as possible in case of error
				if err := sriov.ResetVF(args, cache.Netconf.DeviceID, cache.OrigIfName); err != nil {
					log.Printf("Failed best-effort cleanup of VF %s: %v", cache.OrigIfName, err)
				}
			}
		}
	} else {
		err = ns.WithNetNSPath(args.Netns, func(ns.NetNS) error {
			err = ip.DelLinkByName(args.IfName)
			return err
		})
		// do the following as per cni spec (i.e. Plugins should generally complete a DEL action
		// without error even if some resources are missing)
		if _, ok := err.(ns.NSPathNotExistErr); ok || err == ip.ErrLinkNotFound {
			if portFound {
				if err := ip.DelLinkByName(portName); err != nil {
					log.Printf("Failed best-effort cleanup of %s: %v", portName, err)
				}
			}
			return nil
		}
	}

	// removes all ports whose interfaces have an error
	if err := common.CleanPorts(ovsBridgeDriver); err != nil {
		return err
	}

	return err
}

// CmdCheck check handler to make sure networking is as expected.
func CmdCheck(args *skel.CmdArgs) error {
	logCall("CHECK", args)

	netconf, err := config.LoadConf(args.StdinData)
	if err != nil {
		return err
	}
	ovsHWOffloadEnable := sriov.IsOvsHardwareOffloadEnabled(netconf.DeviceID)

	envArgs, err := getEnvArgs(args.Args)
	if err != nil {
		return err
	}
	var ovnPort string
	if envArgs != nil {
		ovnPort = string(envArgs.OvnPort)
	}
	ovsDriver, err := ovsdb.NewOvsDriver(netconf.SocketFile)
	if err != nil {
		return err
	}
	// cached config may contain bridge name which were automatically
	// discovered in CmdAdd, we need to re-discover the bridge name before we validating the cache
	bridgeName, err := sriov.GetBridgeName(ovsDriver, netconf.BrName, ovnPort, netconf.DeviceID)
	if err != nil {
		return err
	}
	netconf.BrName = bridgeName

	// check cache
	cRef := config.GetCRef(args.ContainerID, args.IfName)
	cache, err := config.LoadConfFromCache(cRef)
	if err != nil {
		return err
	}

	if err := validateCache(cache, netconf); err != nil {
		return err
	}

	// TODO: CmdCheck for userspace driver
	if cache.UserspaceMode {
		return nil
	}

	// run the IPAM plugin
	// userspace driver does not support IPAM plugin,
	// because there is no network interface for the VF on the host
	if netconf.NetConf.IPAM.Type != "" && !cache.UserspaceMode {
		err = ipam.ExecCheck(netconf.NetConf.IPAM.Type, args.StdinData)
		if err != nil {
			return fmt.Errorf("failed to check with IPAM plugin type %q: %v", netconf.NetConf.IPAM.Type, err)
		}
	}

	// Parse previous result.
	if netconf.NetConf.RawPrevResult == nil {
		return fmt.Errorf("Required prevResult missing")
	}
	if err := version.ParsePrevResult(&netconf.NetConf); err != nil {
		return err
	}
	result, err := current.NewResultFromResult(netconf.NetConf.PrevResult)
	if err != nil {
		return err
	}

	var contIntf, hostIntf current.Interface
	// Find interfaces
	for _, intf := range result.Interfaces {
		if args.IfName == intf.Name {
			if args.Netns == intf.Sandbox {
				contIntf = *intf
			}
		} else {
			// Check prevResults for ips against values found in the host
			if err := validateInterface(*intf, true, ovsHWOffloadEnable); err != nil {
				return err
			}
			hostIntf = *intf
		}
	}

	// The namespace must be the same as what was configured
	if args.Netns != contIntf.Sandbox {
		return fmt.Errorf("Sandbox in prevResult %s doesn't match configured netns: %s",
			contIntf.Sandbox, args.Netns)
	}

	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer func() { _ = netns.Close() }()

	// Check prevResults for ips and routes against values found in the container
	if err := netns.Do(func(_ ns.NetNS) error {
		// Check interface against values found in the container
		err := validateInterface(contIntf, false, ovsHWOffloadEnable)
		if err != nil {
			return err
		}

		err = ip.ValidateExpectedInterfaceIPs(args.IfName, result.IPs)
		if err != nil {
			return err
		}

		err = ip.ValidateExpectedRoute(result.Routes)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	// ovs specific check
	if err := ValidateOvs(args, netconf, hostIntf.Name); err != nil {
		return err
	}

	return nil
}

func validateCache(cache *types.CachedNetConf, netconf *types.NetConf) error {
	if cache.Netconf.BrName != netconf.BrName {
		return fmt.Errorf("BrName mismatch. cache=%s,netconf=%s",
			cache.Netconf.BrName, netconf.BrName)
	}

	if cache.Netconf.SocketFile != netconf.SocketFile {
		return fmt.Errorf("SocketFile mismatch. cache=%s,netconf=%s",
			cache.Netconf.SocketFile, netconf.SocketFile)
	}

	if cache.Netconf.IPAM.Type != netconf.IPAM.Type {
		return fmt.Errorf("IPAM mismatch. cache=%s,netconf=%s",
			cache.Netconf.IPAM.Type, netconf.IPAM.Type)
	}

	if cache.Netconf.DeviceID != netconf.DeviceID {
		return fmt.Errorf("DeviceID mismatch. cache=%s,netconf=%s",
			cache.Netconf.DeviceID, netconf.DeviceID)
	}

	return nil
}

func validateInterface(intf current.Interface, isHost bool, hwOffload bool) error {
	var link netlink.Link
	var err error
	var iftype string
	if isHost {
		iftype = "Host"
	} else {
		iftype = "Container"
	}

	if intf.Name == "" {
		return fmt.Errorf("%s interface name missing in prevResult: %v", iftype, intf.Name)
	}
	link, err = netlink.LinkByName(intf.Name)
	if err != nil {
		return fmt.Errorf("Error: %s Interface name in prevResult: %s not found", iftype, intf.Name)
	}
	if !isHost && intf.Sandbox == "" {
		return fmt.Errorf("Error: %s interface %s should not be in host namespace", iftype, link.Attrs().Name)
	}
	if !hwOffload {
		_, isVeth := link.(*netlink.Veth)
		if !isVeth {
			return fmt.Errorf("Error: %s interface %s not of type veth/p2p", iftype, link.Attrs().Name)
		}
	}

	if intf.Mac != "" && intf.Mac != link.Attrs().HardwareAddr.String() {
		return fmt.Errorf("Error: Interface %s Mac %s doesn't match %s Mac: %s", intf.Name, intf.Mac, iftype, link.Attrs().HardwareAddr)
	}

	return nil
}

func ValidateOvs(args *skel.CmdArgs, netconf *types.NetConf, hostIfname string) error {
	ovsBridgeDriver, err := ovsdb.NewOvsBridgeDriver(netconf.BrName, netconf.SocketFile)
	if err != nil {
		return err
	}

	found, err := ovsBridgeDriver.IsBridgePresent(netconf.BrName)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("Error: bridge %s is not found in OVS", netconf.BrName)
	}

	hasError, err := ovsBridgeDriver.InterfaceHasError(hostIfname)
	if err != nil {
		return err
	}
	if hasError {
		return fmt.Errorf("Error: interface %s is in error state", hostIfname)
	}

	vlanMode, tag, trunk, err := ovsBridgeDriver.GetOFPortVlanState(hostIfname)
	if err != nil {
		return fmt.Errorf("Error: Failed to retrieve port %s state: %v", hostIfname, err)
	}

	// check vlan tag
	if netconf.VlanTag == nil {
		if tag != nil {
			return fmt.Errorf("vlan tag mismatch. ovs=%d,netconf=nil", *tag)
		}
	} else {
		if tag == nil {
			return fmt.Errorf("vlan tag mismatch. ovs=nil,netconf=%d", *netconf.VlanTag)
		}
		if *tag != *netconf.VlanTag {
			return fmt.Errorf("vlan tag mismatch. ovs=%d,netconf=%d", *tag, *netconf.VlanTag)
		}
		if vlanMode != "access" {
			return fmt.Errorf("vlan mode mismatch. expected=access,real=%s", vlanMode)
		}
	}

	// check trunk
	netconfTrunks := make([]uint, 0)
	if len(netconf.Trunk) > 0 {
		trunkVlanIds, err := common.SplitVlanIds(netconf.Trunk)
		if err != nil {
			return err
		}
		netconfTrunks = append(netconfTrunks, trunkVlanIds...)
	}
	if len(trunk) != len(netconfTrunks) {
		return fmt.Errorf("trunk mismatch. ovs=%v,netconf=%v", trunk, netconfTrunks)
	}
	if len(netconfTrunks) > 0 {
		for i := 0; i < len(trunk); i++ {
			if trunk[i] != netconfTrunks[i] {
				return fmt.Errorf("trunk mismatch. ovs=%v,netconf=%v", trunk, netconfTrunks)
			}
		}

		if vlanMode != "trunk" {
			return fmt.Errorf("vlan mode mismatch. expected=trunk,real=%s", vlanMode)
		}
	}

	return nil
}
