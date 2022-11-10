// Copyright (c) 2021 Red Hat, Inc.
// Copyright (c) 2021 CNI authors
// Copyright (c) 2021 Nordix Foundation.
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

package types

import (
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
)

// NetConfs can be either NetConf or MirrorNetConf
type NetConfs interface {
	NetConf | MirrorNetConf
}

// NetConf extends types.NetConf for ovs-cni
type NetConf struct {
	types.NetConf
	BrName                 string   `json:"bridge,omitempty"`
	VlanTag                *uint    `json:"vlan"`
	MTU                    int      `json:"mtu"`
	Trunk                  []*Trunk `json:"trunk,omitempty"`
	DeviceID               string   `json:"deviceID"`       // PCI address of a VF in valid sysfs format
	OfportRequest          uint     `json:"ofport_request"` // OpenFlow port number in range 1 to 65,279
	InterfaceType          string   `json:"interface_type"` // The type of interface on ovs.
	ConfigurationPath      string   `json:"configuration_path"`
	SocketFile             string   `json:"socket_file"`
	LinkStateCheckRetries  int      `json:"link_state_check_retries"`
	LinkStateCheckInterval int      `json:"link_state_check_interval"`
}

// MirrorNetConf extends types.NetConf for ovs-mirrors
type MirrorNetConf struct {
	types.NetConf

	// support chaining for master interface and IP decisions
	// occurring prior to running mirror plugin
	RawPrevResult *map[string]interface{} `json:"prevResult"`
	PrevResult    *current.Result         `json:"-"`

	BrName            string    `json:"bridge,omitempty"`
	ConfigurationPath string    `json:"configuration_path"`
	SocketFile        string    `json:"socket_file"`
	Mirrors           []*Mirror `json:"mirrors"`
}

// Mirror configuration
type Mirror struct {
	Name    string `json:"name"`
	Ingress bool   `json:"ingress,omitempty"`
	Egress  bool   `json:"egress,omitempty"`
}

// Trunk containing selective vlan IDs
type Trunk struct {
	MinID *uint `json:"minID,omitempty"`
	MaxID *uint `json:"maxID,omitempty"`
	ID    *uint `json:"id,omitempty"`
}

// CachedNetConf containing NetConfig and original smartnic vf interface
// name (set only in case of ovs hareware offload scenario).
// this is intended to be used only for storing and retrieving config
// to/from a data store (example file cache).
type CachedNetConf struct {
	Netconf    *NetConf
	OrigIfName string
}

// CachedPrevResultNetConf containing PrevResult.
// this is intended to be used only for storing and retrieving config
// to/from a data store (example file cache).
// This is required with CNI spec < 0.4.0 (like 0.3.0 and 0.3.1),
// because prevResult wasn't available in cmdDel on those versions.
type CachedPrevResultNetConf struct {
	PrevResult *current.Result
}
