package vdpa

import (
	"fmt"
	"net"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/k8snetworkplumbingwg/govdpa/pkg/kvdpa"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
	"github.com/vishvananda/netlink"
)

type VdpaDeviceType string

const (
	VdpaDeviceTypeNone        = ""
	VdpaDeviceTypeKernelVhost = "VdpaKernelVhost"
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

func GetDeviceType(vdpaDev *kvdpa.VdpaDevice) (VdpaDeviceType, error) {
	if vdpaDev == nil {
		return VdpaDeviceTypeNone, nil
	}

	switch (*vdpaDev).Driver() {
	case kvdpa.VhostVdpaDriver:
		return VdpaDeviceTypeKernelVhost, nil
	default:
		return VdpaDeviceTypeNone, fmt.Errorf("unknown vdpa device type")
	}
}

func DeviceIDToVdpaType(deviceID string) (VdpaDeviceType, error) {
	vdpaDev, err := GetVdpaDeviceFromID(deviceID)
	if err != nil {
		return VdpaDeviceTypeNone, err
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
	case VdpaDeviceTypeNone:
		return nil, nil, fmt.Errorf("non-vdpa devices can not be configured as such")
	case VdpaDeviceTypeKernelVhost:
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

	hostVdpaMac, err := getVdpaMacAddr(vdpaDevice)
	if err != nil {
		return nil, nil, err
	}
	hostIface.Mac = hostVdpaMac.String()

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
		contIface.Mac = hostIface.Mac
	}

	vfNetDevice, err := sriov.GetNetVF(deviceID)
	if err != nil {
		return nil, nil, err
	}

	err = sriov.MoveVFToNetns(vfNetDevice, contNetns)
	if err != nil {
		return nil, nil, err
	}

	err = contNetns.Do(func(hostNS ns.NetNS) error {
		contIface.Name = ifName
		_, err = sriov.RenameLink(vfNetDevice, contIface.Name)
		if err != nil {
			return err
		}
		link, err := netlink.LinkByName(contIface.Name)
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

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return hostIface, contIface, nil
}
