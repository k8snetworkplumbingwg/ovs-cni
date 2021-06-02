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

import "github.com/containernetworking/cni/pkg/types"

// NetConf extends types.NetConf for ovs-cni
type NetConf struct {
	types.NetConf
	BrName                 string   `json:"bridge,omitempty"`
	VlanTag                *uint    `json:"vlan"`
	MTU                    int      `json:"mtu"`
	Trunk                  []*Trunk `json:"trunk,omitempty"`
	DeviceID               string   `json:"deviceID"` // PCI address of a VF in valid sysfs format
	ConfigurationPath      string   `json:"configuration_path"`
	SocketFile             string   `json:"socket_file"`
	LinkStateCheckRetries  int      `json:"link_state_check_retries"`
	LinkStateCheckInterval int      `json:"link_state_check_interval"`
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
