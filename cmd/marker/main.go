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
	"time"

	"github.com/golang/glog"
	"github.com/kubevirt/ovs-cni/pkg/marker"
)

func main() {
	nodeName := flag.String("node-name", "", "name of kubernetes node")
	pollInterval := flag.Int("poll-interval", 10, "interval between updates in seconds, 10 by default")
	ovsSocket := flag.String("ovs-socket", "", "address of openvswitch database connection")

	flag.Parse()

	if *nodeName == "" {
		glog.Fatal("node-name must be set")
	}

	if *ovsSocket == "" {
		glog.Fatal("ovs-socket must be set")
	}

	markerApp, err := marker.NewMarker(*nodeName, *ovsSocket)
	if err != nil {
		glog.Fatalf("failed to create a new marker object: %v", err)
	}

	for {
		err := markerApp.Update()
		if err != nil {
			glog.Errorf("Update failed: %v", err)
		}
		time.Sleep(time.Duration(*pollInterval) * time.Second)
	}
}
