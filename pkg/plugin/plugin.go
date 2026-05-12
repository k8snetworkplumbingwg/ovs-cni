// Copyright 2018-2019 Red Hat, Inc.
// Copyright 2014 CNI authors
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

// Go version 1.10 or greater is required. Before that, switching namespaces in
// long running processes in go did not work in a reliable way.
//go:build go1.10
// +build go1.10

package plugin

import (
	"log"
	"runtime"

	"github.com/containernetworking/cni/pkg/skel"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/common"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/config"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/deviceinfo"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/sriov"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/utils"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/vdpa"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/veth"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func logCall(command string, args *skel.CmdArgs) {
	log.Printf("CNI %s was called for container ID: %s, network namespace %s, interface name %s, configuration: %s",
		command, args.ContainerID, args.Netns, args.IfName, string(args.StdinData[:]))
}

// CmdAdd add handler for attaching container into network
func CmdAdd(args *skel.CmdArgs) error {
	logCall("ADD", args)

	netconf, err := config.LoadConf(args.StdinData)
	if err != nil {
		return err
	}

	deviceInfo, err := deviceinfo.GetDeviceInfo(netconf)
	if err != nil {
		return err
	}

	if vdpa.IsVdpa(deviceInfo) {
		return vdpa.CmdAdd(args, netconf)
	}

	if !common.IsOvsHardwareOffloadEnabled(netconf.DeviceID) {
		return veth.CmdAdd(args, netconf)
	}

	return sriov.CmdAdd(args, netconf)
}

// CmdDel remove handler for deleting container from network
func CmdDel(args *skel.CmdArgs) error {
	logCall("DEL", args)

	cRef := config.GetCRef(args.ContainerID, args.IfName)
	cache, err := config.LoadConfFromCache(cRef)
	if err != nil {
		// If cmdDel() fails, cached netconf is cleaned up by
		// the followed defer call. However, subsequence calls
		// of cmdDel() from kubelet fail in a dead loop due to
		// cached netconf doesn't exist.
		// Return nil when loadConfFromCache fails since the rest
		// of cmdDel() code relies on netconf as input argument
		// and there is no meaning to continue.
		return nil
	}

	defer func() {
		if err == nil {
			if err := utils.CleanCache(cRef); err != nil {
				log.Printf("Failed cleaning up cache: %v", err)
			}
		}
	}()

	if vdpa.CachedDeviceIsVdpa(cache) {
		err = vdpa.CmdDel(args, cache)
		return err
	}

	if !common.IsOvsHardwareOffloadEnabled(cache.Netconf.DeviceID) {
		err = veth.CmdDel(args, cache)
		return err
	}

	err = sriov.CmdDel(args, cache)
	return err
}

// CmdCheck check handler to make sure networking is as expected.
func CmdCheck(args *skel.CmdArgs) error {
	logCall("CHECK", args)

	netconf, err := config.LoadConf(args.StdinData)
	if err != nil {
		return err
	}

	deviceInfo, err := deviceinfo.GetDeviceInfo(netconf)
	if err != nil {
		return err
	}

	if vdpa.IsVdpa(deviceInfo) {
		return vdpa.CmdCheck(args, netconf)
	}

	if !common.IsOvsHardwareOffloadEnabled(netconf.DeviceID) {
		return veth.CmdCheck(args, netconf)
	}

	return sriov.CmdCheck(args, netconf)
}
