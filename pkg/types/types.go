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
	"encoding/json"

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

// netConfAlias is used to avoid infinite recursion when marshaling NetConf.
// The embedded types.NetConf has a custom MarshalJSON that only marshals its own fields,
// which would cause OVS-specific fields (like BrName) to be lost during marshaling.
type netConfAlias NetConf

// MarshalJSON implements custom JSON marshaling for NetConf.
// This is necessary because the embedded types.NetConf (which is types.PluginConf)
// has its own MarshalJSON method that only marshals PluginConf fields, causing
// OVS-specific fields like BrName to be lost. By defining our own MarshalJSON,
// we ensure all fields are properly marshaled.
func (n NetConf) MarshalJSON() ([]byte, error) {
	return json.Marshal(netConfAlias(n))
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

// mirrorNetConfAlias is used to avoid infinite recursion when marshaling MirrorNetConf.
type mirrorNetConfAlias MirrorNetConf

// MarshalJSON implements custom JSON marshaling for MirrorNetConf.
// This is necessary for the same reason as NetConf.MarshalJSON - the embedded
// types.NetConf has a custom MarshalJSON that would cause mirror-specific fields
// to be lost during marshaling.
func (n MirrorNetConf) MarshalJSON() ([]byte, error) {
	return json.Marshal(mirrorNetConfAlias(n))
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

// CachedNetConf containing NetConfig, original smartnic vf interface name
// and kernel/userspace device driver mode of the smartnic vf interface
// (the last two are set only in case of ovs hareware offload scenario).
// this is intended to be used only for storing and retrieving config
// to/from a data store (example file cache).
type CachedNetConf struct {
	Netconf       *NetConf
	OrigIfName    string
	UserspaceMode bool
}

// CachedPrevResultNetConf containing PrevResult.
// this is intended to be used only for storing and retrieving config
// to/from a data store (example file cache).
// This is required with CNI spec < 0.4.0 (like 0.3.0 and 0.3.1),
// because prevResult wasn't available in cmdDel on those versions.
type CachedPrevResultNetConf struct {
	PrevResult *current.Result
}
