// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// This package collects routines that plugins might need to use no
// matter the interface backend (veth, sriov vf, userspace...)
package common

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"time"

	"github.com/j-keck/arping"
	"github.com/vishvananda/netlink"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/config"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

const (
	portTypeAccess = "access"
	portTypeTrunk  = "trunk"
)

type OvsPortConfig struct {
	Type    string
	Trunks  []uint
	VlanTag uint
}

func GetEnvArgs(envArgsString string) (*types.EnvArgs, error) {
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

// IsOvsHardwareOffloadEnabled when device id is set, then ovs hardware offload
// is enabled.
func IsOvsHardwareOffloadEnabled(deviceID string) bool {
	return deviceID != ""
}

func SplitVlanIds(trunks []*types.Trunk) ([]uint, error) {
	vlans := make(map[uint]bool)
	for _, item := range trunks {
		var minID uint = 0
		var maxID uint = 0
		if item.MinID != nil {
			minID = *item.MinID
			if minID > 4096 {
				return nil, errors.New("incorrect trunk minID parameter")
			}
		}
		if item.MaxID != nil {
			maxID = *item.MaxID
			if maxID > 4096 {
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
		var id uint
		if item.ID != nil {
			id = *item.ID
			if minID > 4096 {
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

func ParseOvsPortConfig(netconf *types.NetConf) (*OvsPortConfig, error) {
	var vlanTagNum uint = 0
	trunks := make([]uint, 0)
	portType := portTypeAccess
	if netconf.VlanTag == nil || len(netconf.Trunk) > 0 {
		portType = portTypeTrunk
		if len(netconf.Trunk) > 0 {
			trunkVlanIds, err := SplitVlanIds(netconf.Trunk)
			if err != nil {
				return nil, err
			}
			trunks = append(trunks, trunkVlanIds...)
		}
	} else if netconf.VlanTag != nil {
		vlanTagNum = *netconf.VlanTag
	}

	return &OvsPortConfig{
		Type:    portType,
		Trunks:  trunks,
		VlanTag: vlanTagNum,
	}, nil
}

// GetBridgeName checks the bridgeName and ovnPort variables to resolve
// bridge name to defaults if needed
func GetBridgeName(bridgeName, ovnPort string) (string, error) {
	if bridgeName != "" {
		return bridgeName, nil
	} else if bridgeName == "" && ovnPort != "" {
		return "br-int", nil
	}

	return "", fmt.Errorf("failed to get bridge name")
}

// CleanPorts removes all ports whose interfaces have an error.
func CleanPorts(ovsDriver *ovsdb.OvsBridgeDriver) error {
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

func RefetchIface(iface *current.Interface) error {
	link, err := netlink.LinkByName(iface.Name)
	if err != nil {
		return fmt.Errorf("failed to refetch interface %s: %v", iface.Name, err)
	}
	iface.Mac = link.Attrs().HardwareAddr.String()
	return nil
}

func AttachIfaceToBridge(ovsDriver *ovsdb.OvsBridgeDriver, hostIfaceName string, contIfaceName string, ofportRequest uint, vlanTag uint, trunks []uint, portType string, intfType string, contNetnsPath string, ovnPortName string, contPodUid string) error {
	err := ovsDriver.CreatePort(hostIfaceName, contNetnsPath, contIfaceName, ovnPortName, ofportRequest, vlanTag, trunks, portType, intfType, contPodUid)
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

func RemoveOvsPort(ovsDriver *ovsdb.OvsBridgeDriver, portName string) error {
	return ovsDriver.DeletePort(portName)
}

func CleanupOvsPortBestEffort(ovsDriver *ovsdb.OvsBridgeDriver, ifaceName string, contNetns string) (string, bool, error) {
	portName, portFound, err := ovsDriver.GetOvsPortForContIface(ifaceName, contNetns)

	if err != nil {
		return "", false, fmt.Errorf("Failed to obtain OVS port for given connection: %v", err)
	}
	if portFound {
		if err := RemoveOvsPort(ovsDriver, portName); err != nil {
			return portName, portFound, err
		}
	}

	return portName, portFound, nil
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

func assignMacToLink(link netlink.Link, mac net.HardwareAddr, name string) error {
	err := netlink.LinkSetHardwareAddr(link, mac)
	if err != nil {
		return fmt.Errorf("failed to set container iface %q MAC %q: %v", name, mac.String(), err)
	}
	return nil
}

func ManagedIPAMAddCall(
	ovsDriver *ovsdb.OvsBridgeDriver,
	args *skel.CmdArgs,
	netconf *types.NetConf,
	mac string,
	hostIface,
	contIface *current.Interface,
	contNetns ns.NetNS,
	ovsHWOffloadEnabled bool,
) (*current.Result, error) {
	var r cnitypes.Result
	r, err := ipam.ExecAdd(netconf.IPAM.Type, args.StdinData)
	defer func() {
		if err != nil {
			if err := ipam.ExecDel(netconf.IPAM.Type, args.StdinData); err != nil {
				log.Printf("Failed best-effort cleanup IPAM configuration: %v", err)
			}
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("failed to set up IPAM plugin type %q: %v", netconf.IPAM.Type, err)
	}

	// Convert the IPAM result into the current Result type
	var newResult *current.Result
	newResult, err = current.NewResultFromResult(r)
	if err != nil {
		return nil, err
	}

	if len(newResult.IPs) == 0 {
		return nil, errors.New("IPAM plugin returned missing IP config")
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
		return nil, err
	}

	err = contNetns.Do(func(_ ns.NetNS) error {
		if mac == "" && !ovsHWOffloadEnabled && len(newResult.IPs) >= 1 {
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
		return nil, err
	}
	result := newResult
	result.Interfaces = []*current.Interface{hostIface, result.Interfaces[0]}

	for ifIndex, ifCfg := range result.Interfaces {
		// Adjust interface index with new container interface index in result.Interfaces
		if ifCfg.Name == args.IfName {
			for ipIndex := range result.IPs {
				result.IPs[ipIndex].Interface = current.Int(ifIndex)
			}
		}
	}

	return result, nil
}

func ValidateInterface(intf current.Interface, isHost bool, hwOffload bool) error {
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
		trunkVlanIds, err := SplitVlanIds(netconf.Trunk)
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

func CacheLoadAndCheck(args *skel.CmdArgs, netconf *types.NetConf) (*types.CachedNetConf, error) {
	cRef := config.GetCRef(args.ContainerID, args.IfName)
	cache, err := config.LoadConfFromCache(cRef)
	if err != nil {
		return nil, err
	}

	err = validateCache(cache, netconf)
	if err != nil {
		return nil, err
	}

	return cache, nil
}

// ParsePrevResult parses the previous CNI result from netconf
func ParsePrevResult(netconf *types.NetConf) (*current.Result, error) {
	if netconf.NetConf.RawPrevResult == nil {
		return nil, fmt.Errorf("Required prevResult missing")
	}
	if err := version.ParsePrevResult(&netconf.NetConf); err != nil {
		return nil, err
	}
	result, err := current.NewResultFromResult(netconf.NetConf.PrevResult)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ExtractInterfaces extracts and validates host and container interfaces from result
func ExtractInterfaces(args *skel.CmdArgs, result *current.Result, ovsHWOffloadEnable bool) (hostIntf, contIntf current.Interface, err error) {
	for _, intf := range result.Interfaces {
		if args.IfName == intf.Name {
			if args.Netns == intf.Sandbox {
				contIntf = *intf
			}
		} else {
			// Check prevResults for ips against values found in the host
			if err := ValidateInterface(*intf, true, ovsHWOffloadEnable); err != nil {
				return current.Interface{}, current.Interface{}, err
			}
			hostIntf = *intf
		}
	}

	// The namespace must be the same as what was configured
	if args.Netns != contIntf.Sandbox {
		return current.Interface{}, current.Interface{}, fmt.Errorf("Sandbox in prevResult %s doesn't match configured netns: %s",
			contIntf.Sandbox, args.Netns)
	}

	return hostIntf, contIntf, nil
}

// ValidateNetnsAttachment validates IPs and routes in the container namespace
func ValidateNetnsAttachment(args *skel.CmdArgs, result *current.Result, contIntf current.Interface, ovsHWOffloadEnable bool) error {
	netns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer func() { _ = netns.Close() }()

	// Check prevResults for ips and routes against values found in the container
	return netns.Do(func(_ ns.NetNS) error {
		// Check interface against values found in the container
		if err := ValidateInterface(contIntf, false, ovsHWOffloadEnable); err != nil {
			return err
		}

		if err := ip.ValidateExpectedInterfaceIPs(args.IfName, result.IPs); err != nil {
			return err
		}

		if err := ip.ValidateExpectedRoute(result.Routes); err != nil {
			return err
		}
		return nil
	})
}

// Checks that host and container interfaces are properly configured, as
// well as the ovs port is assigned to the expected bridge.
//
// Bridge name from netconf must be updated with the expected one before
// calling the function.
func ValidateAttachment(args *skel.CmdArgs, netconf *types.NetConf, cache *types.CachedNetConf) error {
	ovsHWOffloadEnable := IsOvsHardwareOffloadEnabled(netconf.DeviceID)

	result, err := ParsePrevResult(netconf)
	if err != nil {
		return err
	}

	hostIntf, contIntf, err := ExtractInterfaces(args, result, ovsHWOffloadEnable)
	if err != nil {
		return err
	}

	err = ValidateNetnsAttachment(args, result, contIntf, ovsHWOffloadEnable)
	if err != nil {
		return err
	}

	// ovs specific check
	return ValidateOvs(args, netconf, hostIntf.Name)
}
