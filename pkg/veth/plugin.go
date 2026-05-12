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
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ipam"
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

func CmdDel(args *skel.CmdArgs, cache *types.CachedNetConf) error {
	envArgs, err := common.GetEnvArgs(args.Args)
	if err != nil {
		return err
	}

	var ovnPort string
	if envArgs != nil {
		ovnPort = string(envArgs.OvnPort)
	}

	bridgeName, err := common.GetBridgeName(cache.Netconf.BrName, ovnPort)
	if err != nil {
		return err
	}

	ovsBridgeDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, cache.Netconf.SocketFile)
	if err != nil {
		return err
	}

	if cache.Netconf.IPAM.Type != "" {
		err = ipam.ExecDel(cache.Netconf.IPAM.Type, args.StdinData)
		if err != nil {
			return err
		}
	}

	if args.Netns == "" {
		// The CNI_NETNS parameter may be empty according to version 0.4.0
		// of the CNI spec (https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md).
		if err := common.CleanPorts(ovsBridgeDriver); err != nil {
			return err
		}
		return nil
	}

	portName, portFound, err := common.CleanupOvsPortBestEffort(ovsBridgeDriver, args.IfName, args.Netns)
	if err != nil {
		return err
	}

	err = ns.WithNetNSPath(args.Netns, func(ns.NetNS) error {
		err = ip.DelLinkByName(args.IfName)
		return err
	})
	// do the following as per cni spec (i.e. Plugins should generally complete a DEL action
	// without error even if some resources are missing)
	if _, ok := err.(ns.NSPathNotExistErr); ok || err == ip.ErrLinkNotFound {
		if portFound {
			if err := ip.DelLinkByName(portName); err != nil {
				log.Printf("Failed best-effort cleanup of %s: %v", portName, err)
			}
		}
		return nil
	}

	// removes all ports whose interfaces have an error
	if err := common.CleanPorts(ovsBridgeDriver); err != nil {
		return err
	}

	return err
}

func CmdCheck(args *skel.CmdArgs, netconf *types.NetConf) error {
	envArgs, err := common.GetEnvArgs(args.Args)
	if err != nil {
		return err
	}
	var ovnPort string
	if envArgs != nil {
		ovnPort = string(envArgs.OvnPort)
	}

	// Discover bridge name
	bridgeName, err := common.GetBridgeName(netconf.BrName, ovnPort)
	if err != nil {
		return err
	}
	netconf.BrName = bridgeName

	// check cache
	cache, err := common.CacheLoadAndCheck(args, netconf)
	if err != nil {
		return err
	}

	// run the IPAM plugin
	if netconf.NetConf.IPAM.Type != "" {
		if err := ipam.ExecCheck(netconf.NetConf.IPAM.Type, args.StdinData); err != nil {
			return fmt.Errorf("failed to check with IPAM plugin type %q: %v", netconf.NetConf.IPAM.Type, err)
		}
	}

	return common.ValidateAttachment(args, netconf, cache)
}
