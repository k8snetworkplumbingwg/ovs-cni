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
	"errors"
	"fmt"
	"log"
	"runtime"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/config"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/utils"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
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

func getPortUUID(ovsDriver *ovsdb.OvsBridgeDriver, interfaces []*current.Interface) (string, error) {
	for _, iface := range interfaces {
		uuid, err := ovsDriver.GetPortUUID(iface.Name)
		if err == nil {
			return uuid.GoUUID, nil
		}
	}

	return "", errors.New("cannot find port in db")
}

func attachPortToMirror(ovsDriver *ovsdb.OvsBridgeDriver, portUUIDStr string, mirror *types.Mirror) error {
	err := ovsDriver.AttachPortToMirrorConsumer(portUUIDStr, mirror.Name)
	if err != nil {
		return err
	}

	return nil
}

func detachPortFromMirror(ovsDriver *ovsdb.OvsBridgeDriver, portUUIDStr string, mirror *types.Mirror) error {
	err := ovsDriver.DetachPortFromMirrorConsumer(portUUIDStr, mirror.Name)
	if err != nil {
		return err
	}

	return nil
}

// CmdAdd add handler for attaching container into network
func CmdAdd(args *skel.CmdArgs) error {
	logCall("ADD", args)

	netconf, err := config.LoadMirrorConf(args.StdinData)
	if err != nil {
		return err
	}

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(netconf.BrName, netconf.SocketFile)
	if err != nil {
		return err
	}

	// removes all empty mirrors
	if err := ovsDriver.CleanEmptyMirrors(); err != nil {
		return err
	}

	// Cache PrevResult for CmdDel
	if err = utils.SaveCache(config.GetCRef(args.ContainerID, args.IfName)+"_cons",
		&types.CachedPrevResultNetConf{PrevResult: netconf.PrevResult}); err != nil {
		return fmt.Errorf("error saving NetConf %q", err)
	}

	portUUID, err := getPortUUID(ovsDriver, netconf.PrevResult.Interfaces)
	if err != nil {
		return fmt.Errorf("cannot get existing portUuid from db %v", err)
	}

	for _, mirror := range netconf.Mirrors {

		err = ovsDriver.CreateMirror(netconf.BrName, mirror.Name)
		if err != nil {
			return fmt.Errorf("cannot create mirror %s: %v ", mirror.Name, err)
		}

		alreadyAttached, err := ovsDriver.IsMirrorConsumerAlreadyAttached(mirror.Name)
		if err != nil {
			return fmt.Errorf("cannot check if mirror %s has already an output port with error: %v ", mirror.Name, err)
		}
		if alreadyAttached {
			return fmt.Errorf("cannot attach port %s to mirror %s because there is already another port. Error: %v", portUUID, mirror.Name, err)
		}

		if err = attachPortToMirror(ovsDriver, portUUID, mirror); err != nil {
			return fmt.Errorf("cannot attach port %s to mirror %s: %v", portUUID, mirror.Name, err)
		}
	}

	result := &current.Result{
		Interfaces: netconf.PrevResult.Interfaces,
	}

	return cnitypes.PrintResult(result, netconf.CNIVersion)
}

// CmdDel remove handler for deleting container from network
func CmdDel(args *skel.CmdArgs) error {
	logCall("DEL", args)

	cRef := config.GetCRef(args.ContainerID, args.IfName)
	cache, err := config.LoadPrevResultConfFromCache(cRef + "_cons")
	if err != nil {
		// If cmdDel() fails, cached prevResult is cleaned up by
		// the followed defer call. However, subsequence calls
		// of cmdDel() from kubelet fail in a dead loop due to
		// cached prevResult doesn't exist.
		// Return nil when LoadPrevResultConfFromCache fails since the rest
		// of cmdDel() code relies on prevResult as input argument
		// and there is no meaning to continue.
		return nil
	}

	defer func() {
		if err == nil {
			if err := utils.CleanCache(cRef + "_cons"); err != nil {
				log.Printf("Failed cleaning up cache: %v", err)
			}
		}
	}()

	netconf, err := config.LoadMirrorConf(args.StdinData)
	if err != nil {
		return err
	}
	// add prevResult, because missing in CNI spec < 0.4.0
	netconf.PrevResult = cache.PrevResult

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(netconf.BrName, netconf.SocketFile)
	if err != nil {
		return err
	}

	portUUID, err := getPortUUID(ovsDriver, netconf.PrevResult.Interfaces)
	if err != nil {
		return fmt.Errorf("cannot get existing portUuid from db %v", err)
	}

	for _, mirror := range netconf.Mirrors {

		mirrorExist, err := ovsDriver.IsMirrorPresent(mirror.Name)
		if err != nil {
			return err
		}
		if !mirrorExist {
			// skip error because CNI spec states that "Plugins should generally complete a DEL action without error even if some resources are missing"
			continue
		}

		if err = detachPortFromMirror(ovsDriver, portUUID, mirror); err != nil {
			return fmt.Errorf("cannot detach port %s from mirror %s: %v", portUUID, mirror.Name, err)
		}

		used, err := ovsDriver.IsMirrorUsed(netconf.BrName, mirror.Name)
		if err != nil {
			return fmt.Errorf("cannot check if mirror %s is used: %v ", mirror.Name, err)
		}

		// if this mirror is not used we can remove it
		if !used {
			err = ovsDriver.DeleteMirror(netconf.BrName, mirror.Name)
			if err != nil {
				return fmt.Errorf("cannot delete mirror %s: %v ", mirror.Name, err)
			}
		}
	}

	// removes all empty mirrors
	if err := ovsDriver.CleanEmptyMirrors(); err != nil {
		return err
	}

	result := &current.Result{
		Interfaces: netconf.PrevResult.Interfaces,
	}

	return cnitypes.PrintResult(result, netconf.CNIVersion)
}

// CmdCheck check handler to make sure networking is as expected.
func CmdCheck(args *skel.CmdArgs) error {
	logCall("CHECK", args)

	netconf, err := config.LoadMirrorConf(args.StdinData)
	if err != nil {
		return err
	}

	ovsDriver, err := ovsdb.NewOvsBridgeDriver(netconf.BrName, netconf.SocketFile)
	if err != nil {
		return err
	}

	portUUID, err := getPortUUID(ovsDriver, netconf.PrevResult.Interfaces)
	if err != nil {
		return fmt.Errorf("cannot get existing portUuid from db %v", err)
	}

	for _, mirror := range netconf.Mirrors {

		mirrorExist, err := ovsDriver.CheckMirrorConsumerWithPorts(mirror.Name, portUUID)
		if err != nil {
			return err
		}

		if !mirrorExist {
			return fmt.Errorf("mirror %s not present", mirror.Name)
		}
	}

	return nil
}
