/*
Copyright 2026 NVIDIA CORPORATION & AFFILIATES

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sriovnet

import (
	"fmt"
	"net"
	"strconv"

	"github.com/vishvananda/netlink"

	"github.com/k8snetworkplumbingwg/sriovnet/pkg/utils/docacaps"
	"github.com/k8snetworkplumbingwg/sriovnet/pkg/utils/netlinkops"
)

// RepresentorPortParams contains the base port parameters for locating representors.
type RepresentorPortParams struct {
	// The PCI device address on which the representor is anchored
	ECPF string
	// The controller number
	ControllerNumber uint32
	// The PF number
	PFNumber uint16
}

// GetVfRepresentorFromPortParams returns the VF representor netdev name for a given port parameters and VF index
func GetVfRepresentorFromPortParams(pp *RepresentorPortParams, vfIndex uint32) (string, error) {
	if pp == nil {
		return "", fmt.Errorf("port parameters are nil")
	}

	rep, err := getRepresentorDevlink(pp.ECPF, PORT_FLAVOUR_PCI_VF, vfIndex, nil, &pp.ControllerNumber, &pp.PFNumber)
	if err != nil {
		return "", fmt.Errorf("failed to get representor netdev name for VF %d: %w", vfIndex, err)
	}
	return rep, nil
}

// GetSfRepresentorFromPortParams returns the SF representor netdev name for a given port parameters and SF index
func GetSfRepresentorFromPortParams(pp *RepresentorPortParams, sfIndex uint32) (string, error) {
	if pp == nil {
		return "", fmt.Errorf("port parameters are nil")
	}

	rep, err := getRepresentorDevlink(pp.ECPF, PORT_FLAVOUR_PCI_SF, sfIndex, nil, &pp.ControllerNumber, &pp.PFNumber)
	if err != nil {
		return "", fmt.Errorf("failed to get representor netdev name for SF %d: %w", sfIndex, err)
	}
	return rep, nil
}

// GetPfRepresentorFromPortParams returns the PF representor netdev name for a given port parameters
func GetPfRepresentorFromPortParams(pp *RepresentorPortParams) (string, error) {
	if pp == nil {
		return "", fmt.Errorf("port parameters are nil")
	}

	rep, err := getRepresentorDevlink(pp.ECPF, PORT_FLAVOUR_PCI_PF, uint32(pp.PFNumber), nil, &pp.ControllerNumber, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get representor netdev name for PF %d: %w", pp.PFNumber, err)
	}
	return rep, nil
}

// GetPFRepresentorPortParamsFromMAC returns the representor port parameters from the provided MAC address.
//
// Note: This function will work properly only when MAC addresses are unique in the system.
// If multiple ports have the same MAC address, the function will return error.
func GetPFRepresentorPortParamsFromMAC(mac net.HardwareAddr) (*RepresentorPortParams, error) {
	macStr := mac.String()

	if macStr == "" {
		return nil, fmt.Errorf("invalid MAC address %s", macStr)
	}

	// list all devlink ports
	ports, err := netlinkops.GetNetlinkOps().DevLinkGetAllPortList()
	if err != nil {
		return nil, fmt.Errorf("failed to list devlink ports: %w", err)
	}

	// find the port with the given MAC address
	var foundPorts []*netlink.DevlinkPort
	for _, port := range ports {
		if port.BusName != "pci" {
			continue
		}

		if port.PortFlavour != uint16(PORT_FLAVOUR_PCI_PF) {
			continue
		}

		if port.Fn != nil && port.Fn.HwAddr.String() == macStr {
			foundPorts = append(foundPorts, port)
		}
	}

	if len(foundPorts) == 0 {
		return nil, fmt.Errorf("no matching devlink port found with MAC address %s", mac.String())
	}

	if len(foundPorts) > 1 {
		return nil, fmt.Errorf("multiple matching(%d) devlink ports found with MAC address %s", len(foundPorts), mac.String())
	}

	port := foundPorts[0]
	if port.DeviceName == "" || port.ControllerNumber == nil || port.PfNumber == nil {
		return nil, fmt.Errorf("unexpected result from netlink. devlink port with MAC address %s has missing attributes", mac.String())
	}

	return &RepresentorPortParams{
		ECPF:             port.DeviceName,
		ControllerNumber: *port.ControllerNumber,
		PFNumber:         *port.PfNumber,
	}, nil
}

// GetRepresentorPortParamsFromVUID returns the representor port parameters from the provided VUID.
// a VUID is a unique identifier of a specific NIC function/vport
// for a given PF, the vuid may be extracted from the device PCI VPD (VU keyword)
// example: [VU] Vendor specific: e4092a71f9c1f0118000b45cb5355194MLNXS0D0F0
//
// Note: this function relies on the doca_caps CLI tool to be present in the system.
// if the application is running in a container, the tool may be mounted from the host.
// to override the default doca_caps path set the SRIOVNET_DOCA_CAPS_BIN environment variable to the desired path.
func GetRepresentorPortParamsFromVUID(vuid string) (*RepresentorPortParams, error) {
	dev, err := docacaps.NewDocaCaps().GetDocaCapRepDevByVUID(vuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get rep dev from doca_caps by VUID %q: %w", vuid, err)
	}

	ecpf := dev.ECPFPCIAddress
	if ecpf == "" {
		return nil, fmt.Errorf("unexpected result from doca_caps: rep dev with VUID %q has missing ecpf PCI address", vuid)
	}

	controllerNumberStr := dev.Attributes["host_index"]
	if controllerNumberStr == "" {
		return nil, fmt.Errorf("unexpected result from doca_caps: rep dev with VUID %q has missing host_index attribute", vuid)
	}
	controllerNumber, err := strconv.ParseUint(controllerNumberStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("unexpected result from doca_caps: rep dev with VUID %q has invalid host_index attribute: %s: %w", vuid, controllerNumberStr, err)
	}

	pfNumStr := dev.Attributes["pf_index"]
	if pfNumStr == "" {
		return nil, fmt.Errorf("unexpected result from doca_caps: rep dev with VUID %q has missing pf_index attribute", vuid)
	}
	pfNum, err := strconv.ParseUint(pfNumStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("unexpected result from doca_caps: rep dev with VUID %q has invalid pf_index attribute: %s: %w", vuid, pfNumStr, err)
	}

	return &RepresentorPortParams{
		ECPF:             ecpf,
		ControllerNumber: uint32(controllerNumber),
		PFNumber:         uint16(pfNum),
	}, nil
}
