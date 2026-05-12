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

package vdpa

import (
	"fmt"
	"net"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/vishvananda/netlink"

	"github.com/k8snetworkplumbingwg/govdpa/pkg/kvdpa"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

func IsVdpa(deviceInfo *netv1.DeviceInfo) bool {
	return deviceInfo != nil &&
		deviceInfo.Type == netv1.DeviceInfoTypeVDPA &&
		deviceInfo.Vdpa != nil
}

func CachedDeviceIsVdpa(cache *types.CachedNetConf) bool {
	return cache.VdpaType != types.VdpaDeviceTypeNone
}

func pciAddressFrom(deviceID string) string {
	return fmt.Sprintf("pci/%s", deviceID)
}

func getVdpaDeviceFromID(deviceID string) (*kvdpa.VdpaDevice, error) {
	// Don't panic if the device is not vdpa
	if deviceID == "" {
		return nil, nil
	}

	var vdpaDev *kvdpa.VdpaDevice
	vdpaDevs, err := kvdpa.GetVdpaDevicesByPciAddress(pciAddressFrom(deviceID))
	if err != nil {
		return nil, fmt.Errorf("failed to get devices for PCI address %q: %w", deviceID, err)
	}

	if len(vdpaDevs) == 1 {
		vdpaDev = &vdpaDevs[0]
	} else if len(vdpaDevs) > 1 {
		return nil, fmt.Errorf("multiple vdpa devices attached to the same pci mgmt device are not supported")
	} else {
		return nil, fmt.Errorf("could not find vdpa devices assigned to mgmt device %q", deviceID)
	}

	return vdpaDev, nil
}

func getDeviceType(vdpaDev *kvdpa.VdpaDevice) (types.VdpaDeviceType, error) {
	if vdpaDev == nil {
		return types.VdpaDeviceTypeNone, nil
	}

	switch driver := (*vdpaDev).Driver(); driver {
	case kvdpa.VhostVdpaDriver:
		return types.VdpaDeviceTypeKernelVhost, nil
	default:
		return types.VdpaDeviceTypeNone, fmt.Errorf("unknown vdpa device type: %q", driver)
	}
}

func getVdpaMTU(vdpaDevice *kvdpa.VdpaDevice) (uint16, error) {
	cfg, err := netlink.VDPAGetDevConfigByName((*vdpaDevice).Name())
	if err != nil {
		return 0, err
	}

	return cfg.Net.Cfg.MTU, nil
}

func getVdpaMacAddr(vdpaDevice *kvdpa.VdpaDevice) (net.HardwareAddr, error) {
	cfg, err := netlink.VDPAGetDevConfigByName((*vdpaDevice).Name())
	if err != nil {
		return nil, err
	}

	return cfg.Net.Cfg.MACAddr, nil
}

func setupVdpaInterface(
	contNetns ns.NetNS,
	ifName,
	deviceID,
	mac string,
	vdpaDevice *kvdpa.VdpaDevice,
	mtu int,
) (*current.Interface, *current.Interface, error) {
	vdpaDeviceType, err := getDeviceType(vdpaDevice)
	if err != nil {
		return nil, nil, err
	}
	switch vdpaDeviceType {
	case types.VdpaDeviceTypeNone:
		return nil, nil, fmt.Errorf("non-vdpa devices can not be configured as such")
	case types.VdpaDeviceTypeKernelVhost:
		return setupKernelVdpaVhost(contNetns, ifName, deviceID, mac, vdpaDevice, mtu)
	default:
		return nil, nil, fmt.Errorf("unknown vdpa device type")
	}
}

func setupKernelVdpaVhost(
	contNetns ns.NetNS,
	ifName,
	deviceID,
	mac string,
	vdpaDevice *kvdpa.VdpaDevice,
	mtu int,
) (*current.Interface, *current.Interface, error) {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}

	// network representor device for smartvf
	rep, err := sriov.GetNetRepresentor(deviceID)
	if err != nil {
		return nil, nil, err
	}
	hostIface.Name = rep

	repLink, err := netlink.LinkByName(hostIface.Name)
	if err != nil {
		return nil, nil, err
	}
	hostIface.Mac = repLink.Attrs().HardwareAddr.String()

	// parse MAC address if provided from args as described
	// in the CNI spec (https://github.com/containernetworking/cni/blob/main/CONVENTIONS.md)
	var hwaddr net.HardwareAddr
	if mac != "" {
		hwaddr, err = net.ParseMAC(mac)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse MAC address %q: %v", mac, err)
		}
	}

	// If provided, set it to the vdpa device, not the VF
	if hwaddr != nil {
		if err := kvdpa.SetVdpaDeviceMac((*vdpaDevice).Name(), hwaddr); err != nil {
			return nil, nil, err
		}
		contIface.Mac = mac
	} else {
		vdpaMacAddr, err := getVdpaMacAddr(vdpaDevice)
		if err != nil {
			return nil, nil, err
		}
		contIface.Mac = vdpaMacAddr.String()
	}

	if mtu != 0 {
		if err = netlink.LinkSetMTU(repLink, mtu); err != nil {
			return nil, nil, err
		}

		vfNetName, err := sriov.GetNetVF(deviceID)
		if err != nil {
			return nil, nil, err
		}
		vfLink, err := netlink.LinkByName(vfNetName)
		if err != nil {
			return nil, nil, err
		}
		if err = netlink.LinkSetMTU(vfLink, mtu); err != nil {
			return nil, nil, err
		}
	}

	contIface.Name = ifName
	contIface.Sandbox = contNetns.Path()

	return hostIface, contIface, nil
}

func validateVdpaDevice(intf current.Interface, pciAddr string, vdpaType types.VdpaDeviceType) error {
	switch vdpaType {
	case types.VdpaDeviceTypeNone:
		return fmt.Errorf("non-vdpa devices can not be configured as such")
	case types.VdpaDeviceTypeKernelVhost:
		return validateKernelVdpaVhost(intf, pciAddr)
	default:
		return fmt.Errorf("unknown vdpa device type")
	}
}

func validateKernelVdpaVhost(intf current.Interface, pciAddr string) error {
	vdpaDev, err := getVdpaDeviceFromID(pciAddr)
	if err != nil {
		return err
	}

	if intf.Mac != "" {
		macAddr, err := getVdpaMacAddr(vdpaDev)
		if err != nil {
			return err
		}

		if intf.Mac != macAddr.String() {
			return fmt.Errorf(
				"Interface %s Mac %s does not match %s Mac: %s",
				intf.Name, intf.Mac, (*vdpaDev).Name(), macAddr.String(),
			)
		}
	}

	if intf.Mtu != 0 {
		mtu, err := getVdpaMTU(vdpaDev)
		if err != nil {
			return err
		}
		if intf.Mtu != int(mtu) {
			return fmt.Errorf(
				"Interface %s MTU %d does not match %s MTU: %d",
				intf.Name, intf.Mtu, (*vdpaDev).Name(), mtu,
			)
		}

		vfNetName, err := sriov.GetNetVF(pciAddr)
		if err != nil {
			return err
		}
		vfLink, err := netlink.LinkByName(vfNetName)
		if err != nil {
			return err
		}
		vfMtu := vfLink.Attrs().MTU
		if intf.Mtu != vfMtu {
			return fmt.Errorf(
				"Interface %s MTU %d does not match %s MTU: %d",
				intf.Name, intf.Mtu, vfNetName, vfMtu,
			)
		}
	}

	return nil
}
