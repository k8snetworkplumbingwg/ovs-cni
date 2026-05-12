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

package vdpa

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
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
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

	ovsDriver, err := ovsdb.NewOvsDriver(netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := sriov.GetBridgeName(ovsDriver, netconf.BrName, ovnPort, netconf.DeviceID)
	if err != nil {
		return err
	}
	// save discovered bridge name to the netconf struct to make
	// sure it is save in the cache.
	// we need to cache discovered bridge name to make sure that we will
	// use the right bridge name in CmdDel
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

	vdpaDev, err := getVdpaDeviceFromID(netconf.DeviceID)
	if err != nil {
		return err
	}
	vdpaDevType, err := getDeviceType(vdpaDev)
	if err != nil {
		return err
	}

	// Cache NetConf for CmdDel
	if err = utils.SaveCache(config.GetCRef(args.ContainerID, args.IfName),
		&types.CachedNetConf{Netconf: netconf, UserspaceMode: false, VdpaType: vdpaDevType}); err != nil {
		return fmt.Errorf("error saving NetConf %q", err)
	}

	hostIface, contIface, err := setupVdpaInterface(contNetns, args.IfName, netconf.DeviceID, mac, vdpaDev, netconf.MTU)
	if err != nil {
		return err
	}

	if err = common.AttachIfaceToBridge(ovsBridgeDriver,
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
		// Unlike veth pair, OVS port will not be automatically removed
		// if the following IPAM configuration fails and netns gets removed.
		_, _, cleanupErr := common.CleanupOvsPortBestEffort(ovsBridgeDriver, args.IfName, args.Netns)
		if cleanupErr != nil {
			log.Printf("Failed best-effort cleanup: %v", cleanupErr)
		}
		return err
	}

	result := &current.Result{
		Interfaces: []*current.Interface{hostIface, contIface},
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
	ovsDriver, err := ovsdb.NewOvsDriver(cache.Netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := sriov.GetBridgeName(ovsDriver, cache.Netconf.BrName, ovnPort, cache.Netconf.DeviceID)
	if err != nil {
		return err
	}

	ovsBridgeDriver, err := ovsdb.NewOvsBridgeDriver(bridgeName, cache.Netconf.SocketFile)
	if err != nil {
		return err
	}

	// The CNI_NETNS parameter may be empty according to version 0.4.0
	// of the CNI spec (https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md).
	if args.Netns == "" {
		// SR-IOV Case - The sriov device is moved into host network namespace when args.Netns is empty.
		// This happens container is killed due to an error (example: CrashLoopBackOff, OOMKilled)
		var rep string
		if rep, err = sriov.GetNetRepresentor(cache.Netconf.DeviceID); err != nil {
			return err
		}
		if err = common.RemoveOvsPort(ovsBridgeDriver, rep); err != nil {
			// Don't throw err as delete can be called multiple times because of error in ResetVF and ovs
			// port is already deleted in a previous invocation.
			log.Printf("Error: %v\n", err)
		}
		return nil
	}

	// Unlike veth pair, OVS port will not be automatically removed when
	// container namespace is gone. Find port matching DEL arguments and remove
	// it explicitly.
	_, _, err = common.CleanupOvsPortBestEffort(ovsBridgeDriver, args.IfName, args.Netns)
	if err != nil {
		return err
	}

	// removes all ports whose interfaces have an error
	return common.CleanPorts(ovsBridgeDriver)
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

	// Discover bridge name using SR-IOV specific logic
	ovsDriver, err := ovsdb.NewOvsDriver(netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := sriov.GetBridgeName(ovsDriver, netconf.BrName, ovnPort, netconf.DeviceID)
	if err != nil {
		return err
	}
	netconf.BrName = bridgeName

	cache, err := common.CacheLoadAndCheck(args, netconf)
	if err != nil {
		return err
	}

	result, err := common.ParsePrevResult(netconf)
	if err != nil {
		return err
	}

	hostIntf, contIntf, err := common.ExtractInterfaces(args, result, true)
	if err != nil {
		return err
	}

	err = validateVdpaDevice(contIntf, netconf.DeviceID, cache.VdpaType)
	if err != nil {
		return err
	}

	// ovs specific check
	return common.ValidateOvs(args, netconf, hostIntf.Name)
}
