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

package sriov

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/containernetworking/cni/pkg/skel"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/k8snetworkplumbingwg/sriovnet"
	"github.com/vishvananda/netlink"
)

var (
	// SysBusPci is sysfs pci device directory
	SysBusPci       = "/sys/bus/pci/devices"
	rePciDeviceName = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{2}:[01][0-9a-f]\.[0-7]$`)
	reAuxDeviceName = regexp.MustCompile(`^\w+.\w+.\d+$`)
)

// IsPCIDeviceName check if passed device id is a PCI device name
func IsPCIDeviceName(deviceID string) bool {
	return rePciDeviceName.MatchString(deviceID)
}

// IsAuxDeviceName check if passed device id is a Auxiliary device name
func IsAuxDeviceName(deviceID string) bool {
	return reAuxDeviceName.MatchString(deviceID)
}

// GetVFLinkName retrieves interface name for given pci address
func GetVFLinkName(pciAddr string) (string, error) {
	var names []string
	vfDir := filepath.Join(SysBusPci, pciAddr, "net")
	if _, err := os.Lstat(vfDir); err != nil {
		return "", err
	}

	fInfos, err := os.ReadDir(vfDir)
	if err != nil {
		return "", fmt.Errorf("failed to read net dir of the device %s: %v", pciAddr, err)
	}

	if len(fInfos) == 0 {
		return "", fmt.Errorf("VF device %s sysfs path (%s) has no entries", pciAddr, vfDir)
	}

	names = make([]string, 0)
	for _, f := range fInfos {
		names = append(names, f.Name())
	}

	return names[0], nil
}

// GetSFLinkName retrieves aux interface name for given pci address
func GetAuxLinkName(auxDev string) (string, error) {
	var names []string
	names, err := sriovnet.GetNetDevicesFromAux(auxDev)
	if err != nil {
		return "", err
	}
	// Make sure we have 1 netdevice per pci address
	numNetDevices := len(names)
	if numNetDevices != 1 {
		return "", fmt.Errorf("failed to get one netdevice interface (count %d) per Device ID %s", numNetDevices, auxDev)
	}
	return names[0], nil
}

// IsOvsHardwareOffloadEnabled when device id is set, then ovs hardware offload
// is enabled.
func IsOvsHardwareOffloadEnabled(deviceID string) bool {
	return deviceID != ""
}

// GetBridgeUplinkNameByDeviceID tries to automatically resolve uplink interface name
// for provided VF deviceID by following the sequence:
// VF pci address > PF pci address > Bond (optional, if PF is part of a bond)
// return list of candidate names
func GetBridgeUplinkNameByDeviceID(deviceID string) ([]string, error) {
	pfName, err := sriovnet.GetUplinkRepresentor(deviceID)
	if err != nil {
		return nil, err
	}
	pfLink, err := netlink.LinkByName(pfName)
	if err != nil {
		return nil, fmt.Errorf("failed to get link info for uplink %s: %v", pfName, err)
	}
	bond, err := getBondInterface(pfLink)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent link for uplink %s: %v", pfName, err)
	}
	if bond == nil {
		// PF has no parent bond, return only PF name
		return []string{pfLink.Attrs().Name}, nil
	}
	// for some OVS datapathes, to use bond configuration it is required to attach primary PF (usually first one) to the ovs instead of the bond interface.
	// Example:
	// 		- Bond interface bond0 (contains PF0 + PF1)
	//		- OVS bridge br0 (only PF0 is attached)
	//		- VF representors from PF0 and PF1 can be attached to OVS bridge br0, traffic will be offloaded and sent through bond0
	//
	// to support autobridge selection for VFs from the PF1 (which is part of the bond, but not directly attached to the ovs),
	// we need to add other interfaces that are part of the bond as candidates, for PF1 candidates list will be: [bond0, PF0, PF1]
	bondMembers, err := getBondMembers(bond)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve list of bond members for bond %s, uplink %s: %v", bond.Attrs().Name, pfName, err)
	}
	return bondMembers, nil
}

// getBondInterface returns a parent bond interface for the link if it exists
func getBondInterface(link netlink.Link) (netlink.Link, error) {
	if link.Attrs().MasterIndex == 0 {
		return nil, nil
	}
	bondLink, err := netlink.LinkByIndex(link.Attrs().MasterIndex)
	if err != nil {
		return nil, err
	}
	if bondLink.Type() != "bond" {
		return nil, nil
	}
	return bondLink, nil
}

// getBondMembers returns list with name of the bond and all bond members
func getBondMembers(bond netlink.Link) ([]string, error) {
	allLinks, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	result := []string{bond.Attrs().Name}
	for _, link := range allLinks {
		if link.Attrs().MasterIndex == bond.Attrs().Index {
			result = append(result, link.Attrs().Name)
		}
	}
	return result, nil
}

// GetNetRepresentor returns representor name for passed device ID. Supported devices are Virtual Function
// or Scalable Function
func GetNetRepresentor(deviceID string) (string, error) {
	var rep, uplink string
	var err error
	var index int

	if IsPCIDeviceName(deviceID) { // PCI device
		uplink, err = sriovnet.GetUplinkRepresentor(deviceID)
		if err != nil {
			return "", err
		}
		index, err = sriovnet.GetVfIndexByPciAddress(deviceID)
		if err != nil {
			return "", err
		}
		rep, err = sriovnet.GetVfRepresentor(uplink, index)
	} else if IsAuxDeviceName(deviceID) { // Auxiliary device
		uplink, err = sriovnet.GetUplinkRepresentorFromAux(deviceID)
		if err != nil {
			return "", err
		}
		index, err = sriovnet.GetSfIndexByAuxDev(deviceID)
		if err != nil {
			return "", err
		}
		rep, err = sriovnet.GetSfRepresentor(uplink, index)
	} else {
		return "", fmt.Errorf("cannot determine device type for id '%s'", deviceID)
	}
	if err != nil {
		return "", err
	}
	return rep, nil
}

// SetupSriovInterface moves smartVF into container namespace, rename it with ifName and also returns host interface with VF's representor device
func SetupSriovInterface(contNetns ns.NetNS, containerID, ifName string, mtu int, deviceID string) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}
	var netDevices []string
	var err error

	if IsPCIDeviceName(deviceID) {
		// get smart VF netdevice from PCI
		netDevices, err = sriovnet.GetNetDevicesFromPci(deviceID)
		if err != nil {
			return nil, nil, err
		}
	} else {
		netDevices, err = sriovnet.GetNetDevicesFromAux(deviceID)
		if err != nil {
			return nil, nil, err
		}
	}

	// Make sure we have 1 netdevice per pci address
	if len(netDevices) != 1 {
		return nil, nil, fmt.Errorf("failed to get one netdevice interface per %s", deviceID)
	}
	netDevice := netDevices[0]

	// network representor device for smartvf
	rep, err := GetNetRepresentor(deviceID)
	if err != nil {
		return nil, nil, err
	}

	hostIface.Name = rep

	link, err := netlink.LinkByName(hostIface.Name)
	if err != nil {
		return nil, nil, err
	}
	hostIface.Mac = link.Attrs().HardwareAddr.String()

	// set MTU on smart VF representor
	if mtu != 0 {
		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return nil, nil, fmt.Errorf("failed to set MTU on %s: %v", hostIface.Name, err)
		}
	}

	// Move smart VF to Container namespace
	err = moveIfToNetns(netDevice, contNetns)
	if err != nil {
		return nil, nil, err
	}

	err = contNetns.Do(func(hostNS ns.NetNS) error {
		contIface.Name = ifName
		_, err = renameLink(netDevice, contIface.Name)
		if err != nil {
			return err
		}
		link, err = netlink.LinkByName(contIface.Name)
		if err != nil {
			return err
		}
		if mtu != 0 {
			if err = netlink.LinkSetMTU(link, mtu); err != nil {
				return err
			}
		}
		err = netlink.LinkSetUp(link)
		if err != nil {
			return err
		}
		contIface.Sandbox = contNetns.Path()
		contIface.Mac = link.Attrs().HardwareAddr.String()

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return hostIface, contIface, nil
}

func moveIfToNetns(ifname string, netns ns.NetNS) error {
	vfDev, err := netlink.LinkByName(ifname)
	if err != nil {
		return fmt.Errorf("failed to lookup vf device %v: %q", ifname, err)
	}

	// move VF device to ns
	if err = netlink.LinkSetNsFd(vfDev, int(netns.Fd())); err != nil {
		return fmt.Errorf("failed to move device %+v to netns: %q", ifname, err)
	}

	return nil
}

func renameLink(curName, newName string) (netlink.Link, error) {
	link, err := netlink.LinkByName(curName)
	if err != nil {
		return nil, err
	}

	if err := netlink.LinkSetDown(link); err != nil {
		return nil, err
	}
	if err := netlink.LinkSetName(link, newName); err != nil {
		return nil, err
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return nil, err
	}

	return link, nil
}

// ReleaseVF release the VF from container namespace into host namespace
func ReleaseVF(args *skel.CmdArgs, origIfName string) error {
	hostNs, err := ns.GetCurrentNS()
	if err != nil {
		return fmt.Errorf("failed to get host netns: %v", err)
	}
	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open container netns %q: %v", args.Netns, err)
	}

	return contNetns.Do(func(_ ns.NetNS) error {
		// rename VF device back to its original name
		linkObj, err := renameLink(args.IfName, origIfName)
		if err != nil {
			return err
		}
		// move VF device to host netns
		if err = netlink.LinkSetNsFd(linkObj, int(hostNs.Fd())); err != nil {
			return fmt.Errorf("failed to move interface %s to host netns: %v", origIfName, err)
		}
		return nil
	})

}

// ResetOffloadDev reset the VF which accidently moved into default network namespace by a container failure
func ResetOffloadDev(args *skel.CmdArgs, deviceID, origIfName string) error {
	// get smart VF netdevice from PCI
	var netDevices []string
	var err error

	if IsPCIDeviceName(deviceID) {
		// get smart VF netdevice from PCI
		netDevices, err = sriovnet.GetNetDevicesFromPci(deviceID)
		if err != nil {
			return err
		}
	} else {
		netDevices, err = sriovnet.GetNetDevicesFromAux(deviceID)
		if err != nil {
			return err
		}
	}
	// Make sure we have 1 netdevice per pci address
	if len(netDevices) != 1 {
		// This would happen if netdevice is not yet visible in default network namespace.
		// so return ErrLinkNotFound error so that meta plugin can attempt multiple times
		// until link is available.
		return ip.ErrLinkNotFound
	}
	_, err = renameLink(netDevices[0], origIfName)
	if err != nil {
		return err
	}

	return nil
}
