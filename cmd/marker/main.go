// Copyright 2018 Red Hat, Inc.
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
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/golang/glog"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/cache"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/marker"
)

func main() {
	nodeName := flag.String("node-name", "", "name of kubernetes node")
	ovsSocket := flag.String("ovs-socket", "", "address of openvswitch database connection")

	const defaultUpdateInterval = 60 * time.Second
	updateInterval := flag.Int("update-interval", int(defaultUpdateInterval.Seconds()), fmt.Sprintf("interval between updates in seconds, %d by default", int(defaultUpdateInterval.Seconds())))

	const defaultReconcileInterval = 10 * time.Minute
	reconcileInterval := flag.Int("reconcile-interval", int(defaultReconcileInterval.Minutes()), fmt.Sprintf("interval between node bridges reconcile in minutes, %d by default", int(defaultReconcileInterval.Minutes())))

	flag.Parse()

	if *nodeName == "" {
		glog.Fatal("node-name must be set")
	}

	if *ovsSocket == "" {
		glog.Fatal("ovs-socket must be set")
	}

	var socketType, path string
	ovsSocketTokens := strings.Split(*ovsSocket, ":")
	if len(ovsSocketTokens) < 2 {
		/*
		 * ovsSocket should consist of comma separated socket type and socket
		 * detail. If no socket type is specified, it is assumed to be a unix
		 * domain socket, for backwards compatibility.
		 */
		socketType = "unix"
		path = *ovsSocket
	} else {
		socketType = ovsSocketTokens[0]
		path = ovsSocketTokens[1]
	}

	if socketType == "unix" {
		for {
			_, err := os.Stat(path)
			if err == nil {
				glog.Info("Found the OVS socket")
				break
			} else if os.IsNotExist(err) {
				glog.Infof("Given ovs-socket %q was not found, waiting for the socket to appear", path)
				time.Sleep(time.Minute)
			} else {
				glog.Fatalf("Failed opening the OVS socket with: %v", err)
			}
		}
	}

	markerApp, err := marker.NewMarker(*nodeName, socketType+":"+path)
	if err != nil {
		glog.Fatalf("Failed to create a new marker object: %v", err)
	}

	markerCache := cache.Cache{}
	wait.PollImmediateInfinite(time.Duration(*updateInterval)*time.Second, func() (bool, error) {
		if time.Now().Sub(markerCache.LastRefreshTime()) >= time.Duration(*reconcileInterval)*time.Minute {
			reportedBridges, err := markerApp.GetReportedResources()
			if err != nil {
				glog.Errorf("GetReportedResources failed: %v", err)
			}

			if !reflect.DeepEqual(markerCache.Bridges(), reportedBridges) {
				glog.Warningf("cached bridges are different than the reported bridges on node %s", *nodeName)
			}

			markerCache.Refresh(reportedBridges)
		}

		err := markerApp.Update(&markerCache)
		if err != nil {
			glog.Fatalf("Update failed: %v", err)
		}

		return false, nil
	})
}
