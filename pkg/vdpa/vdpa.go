package vdpa

import (
	"fmt"
	"net"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/vishvananda/netlink"

	"github.com/k8snetworkplumbingwg/govdpa/pkg/kvdpa"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

func pciAddressFrom(deviceID string) string {
	return fmt.Sprintf("pci/%s", deviceID)
}

func GetVdpaDeviceFromID(deviceID string) (*kvdpa.VdpaDevice, error) {
	// Don't panic if the device is not vdpa
	if deviceID == "" {
		return nil, nil
	}

	var vdpaDev *kvdpa.VdpaDevice
	vdpaDevs, err := kvdpa.GetVdpaDevicesByPciAddress(pciAddressFrom(deviceID))
	if err != nil {
		return nil, fmt.Errorf("failed to run vdpa netlink command")
	}

	if len(vdpaDevs) == 1 {
		vdpaDev = &vdpaDevs[0]
	} else if len(vdpaDevs) > 1 {
		return nil, fmt.Errorf("multiple vdpa devices attached to the same pci mgmt device are not supported")
	}

	return vdpaDev, nil
}

func GetDeviceType(vdpaDev *kvdpa.VdpaDevice) (types.VdpaDeviceType, error) {
	if vdpaDev == nil {
		return types.VdpaDeviceTypeNone, nil
	}

	switch (*vdpaDev).Driver() {
	case kvdpa.VhostVdpaDriver:
		return types.VdpaDeviceTypeKernelVhost, nil
	default:
		return types.VdpaDeviceTypeNone, fmt.Errorf("unknown vdpa device type")
	}
}

func DeviceIDToVdpaType(deviceID string) (types.VdpaDeviceType, error) {
	vdpaDev, err := GetVdpaDeviceFromID(deviceID)
	if err != nil {
		return types.VdpaDeviceTypeNone, err
	}
	return GetDeviceType(vdpaDev)
}

func getVdpaMacAddr(vdpaDevice *kvdpa.VdpaDevice) (net.HardwareAddr, error) {
	cfg, err := netlink.VDPAGetDevConfigByName((*vdpaDevice).Name())
	if err != nil {
		return nil, err
	}

	return cfg.Net.Cfg.MACAddr, nil
}

func SetupVdpaInterface(
	contNetns ns.NetNS,
	ifName,
	deviceID,
	mac string,
	vdpaDevice *kvdpa.VdpaDevice,
	mtu int,
) (*current.Interface, *current.Interface, error) {
	vdpaDeviceType, err := GetDeviceType(vdpaDevice)
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
