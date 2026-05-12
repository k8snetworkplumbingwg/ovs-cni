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
	"errors"
	"fmt"
	"log"
	"sort"

	"github.com/vishvananda/netlink"

	current "github.com/containernetworking/cni/pkg/types/100"

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
