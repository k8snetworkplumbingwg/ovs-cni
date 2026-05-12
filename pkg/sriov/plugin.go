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

package sriov

import (
	"fmt"
	"log"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
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

	ovsDriver, err := ovsdb.NewOvsDriver(netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := GetBridgeName(ovsDriver, netconf.BrName, ovnPort, netconf.DeviceID)
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

	// check if the device driver is the type of userspace driver
	userspaceMode, err := HasUserspaceDriver(netconf.DeviceID)
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

	// userspace driver does not create a network interface for the VF on the host
	var origIfName string
	if !userspaceMode {
		origIfName, err = GetVFLinkName(netconf.DeviceID)
		if err != nil {
			return err
		}
	}

	// Cache NetConf for CmdDel
	if err = utils.SaveCache(config.GetCRef(args.ContainerID, args.IfName),
		&types.CachedNetConf{Netconf: netconf, OrigIfName: origIfName, UserspaceMode: userspaceMode}); err != nil {
		return fmt.Errorf("error saving NetConf %q", err)
	}

	hostIface, contIface, err := SetupSriovInterface(contNetns, args.ContainerID, args.IfName, mac, netconf.MTU, netconf.DeviceID, userspaceMode)
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
		return err
	}

	defer func() {
		if err != nil {
			// Unlike veth pair, OVS port will not be automatically removed
			// if the following IPAM configuration fails and netns gets removed.
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

	// run the IPAM plugin
	// userspace driver does not support IPAM plugin,
	// because there is no network interface for the VF on the host
	if netconf.IPAM.Type != "" && !userspaceMode {
		result, err = common.ManagedIPAMAddCall(
			ovsBridgeDriver, args, netconf, mac, hostIface, contIface, contNetns, true,
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
	ovsDriver, err := ovsdb.NewOvsDriver(cache.Netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := GetBridgeName(ovsDriver, cache.Netconf.BrName, ovnPort, cache.Netconf.DeviceID)
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

	// The CNI_NETNS parameter may be empty according to version 0.4.0
	// of the CNI spec (https://github.com/containernetworking/cni/blob/spec-v0.4.0/SPEC.md).
	if args.Netns == "" {
		// SR-IOV Case - The sriov device is moved into host network namespace when args.Netns is empty.
		// This happens container is killed due to an error (example: CrashLoopBackOff, OOMKilled)
		var rep string
		if rep, err = GetNetRepresentor(cache.Netconf.DeviceID); err != nil {
			return err
		}
		if err = common.RemoveOvsPort(ovsBridgeDriver, rep); err != nil {
			// Don't throw err as delete can be called multiple times because of error in ResetVF and ovs
			// port is already deleted in a previous invocation.
			log.Printf("Error: %v\n", err)
		}
		// there is no network interface in case of userspace driver, so OrigIfName is empty
		if !cache.UserspaceMode {
			if err = ResetVF(args, cache.Netconf.DeviceID, cache.OrigIfName); err != nil {
				return err
			}
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

	// there is no network interface in case of userspace driver, so OrigIfName is empty
	if !cache.UserspaceMode {
		err = ReleaseVF(args, cache.OrigIfName)
		if err != nil {
			// try to reset vf into original state as much as possible in case of error
			if err := ResetVF(args, cache.Netconf.DeviceID, cache.OrigIfName); err != nil {
				log.Printf("Failed best-effort cleanup of VF %s: %v", cache.OrigIfName, err)
			}
		}
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

	// Discover bridge name using SR-IOV specific logic
	ovsDriver, err := ovsdb.NewOvsDriver(netconf.SocketFile)
	if err != nil {
		return err
	}
	bridgeName, err := GetBridgeName(ovsDriver, netconf.BrName, ovnPort, netconf.DeviceID)
	if err != nil {
		return err
	}
	netconf.BrName = bridgeName

	// check cache
	cache, err := common.CacheLoadAndCheck(args, netconf)
	if err != nil {
		return err
	}

	// TODO: CmdCheck for userspace driver
	if cache.UserspaceMode {
		return nil
	}

	// run the IPAM plugin
	if netconf.NetConf.IPAM.Type != "" {
		if err := ipam.ExecCheck(netconf.NetConf.IPAM.Type, args.StdinData); err != nil {
			return fmt.Errorf("failed to check with IPAM plugin type %q: %v", netconf.NetConf.IPAM.Type, err)
		}
	}

	return common.ValidateAttachment(args, netconf, cache)
}
