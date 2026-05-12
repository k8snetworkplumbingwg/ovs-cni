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

package veth

import (
	"fmt"
	"log"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/common"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/config"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/utils"
)

func CmdAdd(args *skel.CmdArgs, netconf *types.NetConf) error {
	envArgs, err := common.GetEnvArgs(args.Args)
	if err != nil {
		return err
	}

	var mac string
	var ovnPort string
	var contPodUid string
	if envArgs != nil {
		mac = string(envArgs.MAC)
		ovnPort = string(envArgs.OvnPort)
		contPodUid = string(envArgs.K8S_POD_UID)
	}

	portCfg, err := common.ParseOvsPortConfig(netconf)
	if err != nil {
		return err
	}

	bridgeName, err := common.GetBridgeName(netconf.BrName, ovnPort)
	if err != nil {
		return err
	}
	netconf.BrName = bridgeName

	ovsBridgeDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, netconf.SocketFile)
	if err != nil {
		return err
	}

	// removes all ports whose interfaces have an error
	if err := common.CleanPorts(ovsBridgeDriver); err != nil {
		return err
	}

	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to open netns %q: %v", args.Netns, err)
	}
	defer func() { _ = contNetns.Close() }()

	// Cache NetConf for CmdDel
	if err = utils.SaveCache(config.GetCRef(args.ContainerID, args.IfName),
		&types.CachedNetConf{Netconf: netconf, OrigIfName: "", UserspaceMode: false}); err != nil {
		return fmt.Errorf("error saving NetConf %q", err)
	}

	hostIface, contIface, err := SetupVeth(contNetns, args.IfName, mac, netconf.MTU)
	if err != nil {
		return err
	}

	if err = common.AttachIfaceToBridge(
		ovsBridgeDriver,
		hostIface.Name,
		contIface.Name,
		netconf.OfportRequest,
		portCfg.VlanTag,
		portCfg.Trunks,
		portCfg.Type,
		netconf.InterfaceType,
		args.Netns,
		ovnPort,
		contPodUid,
	); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_, _, cleanupErr := common.CleanupOvsPortBestEffort(ovsBridgeDriver, args.IfName, args.Netns)
			if cleanupErr != nil {
				log.Printf("Failed best-effort cleanup: %v", cleanupErr)
			}
		}
	}()

	// Refetch the host interface MAC since OVS may change it when
	// attaching the port to the bridge.
	if err = common.RefetchIface(hostIface); err != nil {
		return err
	}

	result := &current.Result{
		Interfaces: []*current.Interface{hostIface, contIface},
	}

	if netconf.IPAM.Type != "" {
		result, err = common.ManagedIPAMAddCall(
			ovsBridgeDriver, args, netconf, mac, hostIface, contIface, contNetns, false,
		)
		if err != nil {
			return err
		}
	}

	return cnitypes.PrintResult(result, netconf.CNIVersion)
}
