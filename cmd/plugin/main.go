// Copyright 2018-2019 Red Hat, Inc.
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

package main

import (
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/utils/buildversion"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/plugin"
)

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{
		Add:   plugin.CmdAdd,
		Check: plugin.CmdCheck,
		Del:   plugin.CmdDel,
	}, version.All, buildversion.BuildString("OVS bridge"))
}
