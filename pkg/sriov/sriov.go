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

package sriov

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/pkg/skel"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/k8snetworkplumbingwg/sriovnet"
	"github.com/vishvananda/netlink"
)

var (
	// SysBusPci is sysfs pci device directory
	SysBusPci        = "/sys/bus/pci/devices"
	UserspaceDrivers = []string{"vfio-pci", "uio_pci_generic", "igb_uio"}
)

// GetVFLinkName retrives interface name for given pci address
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

// IsOvsHardwareOffloadEnabled when device id is set, then ovs hardware offload
// is enabled.
func IsOvsHardwareOffloadEnabled(deviceID string) bool {
	return deviceID != ""
}

// HasUserspaceDriver checks if a device is attached to userspace driver
// This method is copied from https://github.com/k8snetworkplumbingwg/sriov-cni/blob/8af83a33b2cac8e2df0bd6276b76658eb7c790ab/pkg/utils/utils.go#L222
func HasUserspaceDriver(pciAddr string) (bool, error) {
	driverLink := filepath.Join(SysBusPci, pciAddr, "driver")
	driverPath, err := filepath.EvalSymlinks(driverLink)
	if err != nil {
		return false, err
	}
	driverStat, err := os.Stat(driverPath)
	if err != nil {
		return false, err
	}
	driverName := driverStat.Name()
	for _, drv := range UserspaceDrivers {
		if driverName == drv {
			return true, nil
		}
	}
	return false, nil
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

// GetNetRepresentor retrieves network representor device for smartvf
func GetNetRepresentor(deviceID string) (string, error) {
	// get Uplink netdevice.  The uplink is basically the PF name of the deviceID (smart VF).
	// The uplink is later used to retrieve the representor for the smart VF.
	uplink, err := sriovnet.GetUplinkRepresentor(deviceID)
	if err != nil {
		return "", err
	}

	// get smart VF index from PCI
	vfIndex, err := sriovnet.GetVfIndexByPciAddress(deviceID)
	if err != nil {
		return "", err
	}

	// get smart VF representor interface. This is a host net device which represents
	// smart VF attached inside the container by device plugin. It can be considered
	// as one end of veth pair whereas other end is smartVF. The VF representor would
	// get added into ovs bridge for the control plane configuration.
	rep, err := sriovnet.GetVfRepresentor(uplink, vfIndex)
	if err != nil {
		return "", err
	}

	return rep, nil
}

func GetNetVF(deviceID string) (string, error) {
	// get smart VF netdevice from PCI
	vfNetdevices, err := sriovnet.GetNetDevicesFromPci(deviceID)
	if err != nil {
		return "", err
	}

	// Make sure we have 1 netdevice per pci address
	if len(vfNetdevices) != 1 {
		return "", fmt.Errorf("failed to get one netdevice interface per %s", deviceID)
	}
	return vfNetdevices[0], nil
}

// setupKernelSriovContIface moves smartVF into container namespace,
// configures the smartVF and also fills in the contIface fields
func setupKernelSriovContIface(contNetns ns.NetNS, contIface *current.Interface, deviceID string, pfLink netlink.Link, vfIdx int, ifName string, hwaddr net.HardwareAddr, mtu int) error {
	vfNetdevice, err := GetNetVF(deviceID)
	if err != nil {
		return err
	}

	// if MAC address is provided, set it to the VF by using PF netlink
	// which is accessible in the host namespace, not in the container namespace
	if hwaddr != nil {
		if err := netlink.LinkSetVfHardwareAddr(pfLink, vfIdx, hwaddr); err != nil {
			return err
		}
	}

	// Move smart VF to Container namespace
	err = MoveVFToNetns(vfNetdevice, contNetns)
	if err != nil {
		return err
	}

	err = contNetns.Do(func(hostNS ns.NetNS) error {
		contIface.Name = ifName
		_, err = renameLink(vfNetdevice, contIface.Name)
		if err != nil {
			return err
		}
		link, err := netlink.LinkByName(contIface.Name)
		if err != nil {
			return err
		}
		// if MAC address is provided, set it to the kernel VF netdevice
		// otherwise, read the MAC address from the kernel VF netdevice
		if hwaddr != nil {
			if err = netlink.LinkSetHardwareAddr(link, hwaddr); err != nil {
				return err
			}
			contIface.Mac = hwaddr.String()
		} else {
			contIface.Mac = link.Attrs().HardwareAddr.String()
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

		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// setupUserspaceSriovContIface configures smartVF via PF netlink and fills in the contIface fields
func setupUserspaceSriovContIface(contNetns ns.NetNS, contIface *current.Interface, pfLink netlink.Link, vfIdx int, ifName string, hwaddr net.HardwareAddr) error {
	contIface.Name = ifName
	contIface.Sandbox = contNetns.Path()

	// if MAC address is provided, set it to the VF by using PF netlink
	if hwaddr != nil {
		if err := netlink.LinkSetVfHardwareAddr(pfLink, vfIdx, hwaddr); err != nil {
			return err
		}
		contIface.Mac = hwaddr.String()
	} else {
		vfInfo := pfLink.Attrs().Vfs[vfIdx]
		contIface.Mac = vfInfo.Mac.String()
	}

	return nil
}

// SetupSriovInterface configures smartVF and returns VF's representor device as host interface and VF's netdevice as container interface
func SetupSriovInterface(contNetns ns.NetNS, containerID, ifName, mac string, mtu int, deviceID string, userspaceMode bool) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}

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

	// get PF netlink and VF index from PCI address
	pfIface, err := sriovnet.GetUplinkRepresentor(deviceID)
	if err != nil {
		return nil, nil, err
	}
	pfLink, err := netlink.LinkByName(pfIface)
	if err != nil {
		return nil, nil, err
	}
	vfIdx, err := sriovnet.GetVfIndexByPciAddress(deviceID)
	if err != nil {
		return nil, nil, err
	}

	// make sure PF netlink and VF index are valid
	if len(pfLink.Attrs().Vfs) < vfIdx || pfLink.Attrs().Vfs[vfIdx].ID != vfIdx {
		return nil, nil, fmt.Errorf("failed to get vf info from %s at index %d with Vfs %v", pfIface, vfIdx, pfLink.Attrs().Vfs)
	}

	// parse MAC address if provided from args as described
	// in the CNI spec (https://github.com/containernetworking/cni/blob/main/CONVENTIONS.md)
	var hwaddr net.HardwareAddr
	if mac != "" {
		hwaddr, err = net.ParseMAC(mac)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse MAC address %q: %v", mac, err)
		}
	}

	// set MTU on smart VF representor
	if mtu != 0 {
		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return nil, nil, fmt.Errorf("failed to set MTU on %s: %v", hostIface.Name, err)
		}
	}

	if !userspaceMode {
		// configure the smart VF netdevice directly in the container namespace
		if err = setupKernelSriovContIface(contNetns, contIface, deviceID, pfLink, vfIdx, ifName, hwaddr, mtu); err != nil {
			return nil, nil, err
		}
	} else {
		// configure the smart VF netdevice via PF netlink
		if err = setupUserspaceSriovContIface(contNetns, contIface, pfLink, vfIdx, ifName, hwaddr); err != nil {
			return nil, nil, err
		}
	}

	return hostIface, contIface, nil
}

func MoveVFToNetns(ifname string, netns ns.NetNS) error {
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

// ResetVF reset the VF which accidently moved into default network namespace by a container failure
func ResetVF(args *skel.CmdArgs, deviceID, origIfName string) error {
	// get smart VF netdevice from PCI
	vfNetdevices, err := sriovnet.GetNetDevicesFromPci(deviceID)
	if err != nil {
		return err
	}
	// Make sure we have 1 netdevice per pci address
	if len(vfNetdevices) != 1 {
		// This would happen if netdevice is not yet visible in default network namespace.
		// so return ErrLinkNotFound error so that meta plugin can attempt multiple times
		// until link is available.
		return ip.ErrLinkNotFound
	}
	_, err = renameLink(vfNetdevices[0], origIfName)
	if err != nil {
		return err
	}

	return nil
}
