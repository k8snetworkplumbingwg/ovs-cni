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

package sriov

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Mellanox/sriovnet"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types/current"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
)

var (
	// SysBusPci is sysfs pci device directory
	SysBusPci = "/sys/bus/pci/devices"
)

func getVFLinkName(pciAddr string) (string, error) {
	var names []string
	vfDir := filepath.Join(SysBusPci, pciAddr, "net")
	if _, err := os.Lstat(vfDir); err != nil {
		return "", err
	}

	fInfos, err := ioutil.ReadDir(vfDir)
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

// SetupSriovInterface moves smartVF into container namespace, rename it with ifName and also returns host interface with VF's representor device
func SetupSriovInterface(contNetns ns.NetNS, containerID, ifName string, mtu int, deviceID string) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}

	hostIFName, err := getVFLinkName(deviceID)
	if err != nil {
		return nil, nil, err
	}
	// Cache hostIFName for CmdDel
	if err = SaveConf(containerID, ifName, hostIFName); err != nil {
		return nil, nil, fmt.Errorf("error saving hostIFName %q", err)
	}

	// get smart VF netdevice from PCI
	vfNetdevices, err := sriovnet.GetNetDevicesFromPci(deviceID)
	if err != nil {
		return nil, nil, err
	}

	// Make sure we have 1 netdevice per pci address
	if len(vfNetdevices) != 1 {
		return nil, nil, fmt.Errorf("failed to get one netdevice interface per %s", deviceID)
	}
	vfNetdevice := vfNetdevices[0]

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
	err = moveIfToNetns(vfNetdevice, contNetns)
	if err != nil {
		return nil, nil, err
	}

	err = contNetns.Do(func(hostNS ns.NetNS) error {
		contIface.Name = ifName
		_, err = renameLink(vfNetdevice, contIface.Name)
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
func ReleaseVF(args *skel.CmdArgs) error {
	hostIFName, cRefPath, err := LoadHostIFNameFromCache(args)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil && cRefPath != "" {
			CleanCachedConf(cRefPath)
		}
	}()

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
		linkObj, err := renameLink(args.IfName, hostIFName)
		if err != nil {
			return err
		}
		// move VF device to host netns
		if err = netlink.LinkSetNsFd(linkObj, int(hostNs.Fd())); err != nil {
			return fmt.Errorf("failed to move interface %s to host netns: %v", hostIFName, err)
		}
		return nil
	})

}

// ResetVF reset the VF which accidently moved into default network namespace by a container failure
func ResetVF(args *skel.CmdArgs, deviceID string) error {
	hostIFName, cRefPath, err := LoadHostIFNameFromCache(args)
	if err != nil {
		return err
	}
	// get smart VF netdevice from PCI
	vfNetdevices, err := sriovnet.GetNetDevicesFromPci(deviceID)
	if err != nil {
		return err
	}
	// Make sure we have 1 netdevice per pci address
	if len(vfNetdevices) != 1 {
		// This would happen if netdevice is not yet visible in default network namespace.
		// so return ErrLinkNotFound error so that multus can attempt multiple times
		// until link is available.
		return ip.ErrLinkNotFound
	}
	_, err = renameLink(vfNetdevices[0], hostIFName)
	if err != nil {
		return err
	}
	// remove the cache entry if everything cleaned up for the device.
	if cRefPath != "" {
		CleanCachedConf(cRefPath)
	}
	return nil
}
