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
	"net"
	"os"
	"reflect"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/golang/glog"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/cache"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/marker"
)

const (
	UnixSocketType          = "unix"
	TcpSocketType           = "tcp"
	SocketConnectionTimeout = time.Minute
)

func main() {
	nodeName := flag.String("node-name", "", "name of kubernetes node")
	ovsSocket := flag.String("ovs-socket", "", "address of openvswitch database connection")

	const defaultUpdateInterval = 60 * time.Second
	updateInterval := flag.Int("update-interval", int(defaultUpdateInterval.Seconds()), fmt.Sprintf("interval between updates in seconds, %d by default", int(defaultUpdateInterval.Seconds())))

	const defaultReconcileInterval = 10 * time.Minute
	reconcileInterval := flag.Int("reconcile-interval", int(defaultReconcileInterval.Minutes()), fmt.Sprintf("interval between node bridges reconcile in minutes, %d by default", int(defaultReconcileInterval.Minutes())))

	const healthCheckFile = "/tmp/healthy"

	const defaultHealthCheckInterval = 60 * time.Second
	healthCheckInterval := flag.Int("healthcheck-interval", int(defaultHealthCheckInterval.Seconds()),
		fmt.Sprintf("health check interval in seconds, %d by default", int(defaultHealthCheckInterval.Seconds())))

	flag.Parse()

	if *nodeName == "" {
		glog.Fatal("node-name must be set")
	}

	endpoint := parseOvsSocket(ovsSocket)

	markerApp, err := marker.NewMarker(*nodeName, endpoint)
	if err != nil {
		glog.Fatalf("Failed to create a new marker object: %v", err)
	}

	go keepAlive(healthCheckFile, *healthCheckInterval)

	markerCache := cache.Cache{}
	wait.JitterUntil(func() {
		jitteredReconcileInterval := wait.Jitter(time.Duration(*reconcileInterval)*time.Minute, 1.2)
		shouldReconcileNode := time.Since(markerCache.LastRefreshTime()) >= jitteredReconcileInterval
		if shouldReconcileNode {
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

	}, time.Duration(*updateInterval)*time.Second, 1.2, true, wait.NeverStop)
}

func keepAlive(healthCheckFile string, healthCheckInterval int) {
	wait.Forever(func() {
		_, err := os.Stat(healthCheckFile)
		if os.IsNotExist(err) {
			file, err := os.Create(healthCheckFile)
			if err != nil {
				glog.Fatalf("failed to create file: %s, err: %v", healthCheckFile, err)
			}
			defer file.Close()
		} else {
			currentTime := time.Now().Local()
			err = os.Chtimes(healthCheckFile, currentTime, currentTime)
			if err != nil {
				glog.Errorf("failed to change modification time of file: %s, err: %v",
					healthCheckFile, err)
			}
		}

	}, time.Duration(healthCheckInterval)*time.Second)
}

func parseOvsSocket(ovsSocket *string) string {
	if *ovsSocket == "" {
		glog.Fatal("ovs-socket must be set")
	}

	var socketType, address string
	ovsSocketTokens := strings.Split(*ovsSocket, ":")
	if len(ovsSocketTokens) < 2 {
		/*
		 * ovsSocket should consist of comma separated socket type and socket
		 * detail. If no socket type is specified, it is assumed to be a unix
		 * domain socket, for backwards compatibility.
		 */
		socketType = UnixSocketType
		address = *ovsSocket
	} else {
		socketType = ovsSocketTokens[0]
		if socketType == TcpSocketType {
			if len(ovsSocketTokens) != 3 {
				glog.Fatalf("Failed to parse OVS %s socket, must be in this format %s:<host>:<port>", socketType, socketType)
			}
			address = fmt.Sprintf("%s:%s", ovsSocketTokens[1], ovsSocketTokens[2])
		} else {
			// unix socket
			address = ovsSocketTokens[1]
		}
	}
	endpoint := fmt.Sprintf("%s:%s", socketType, address)

	if socketType == UnixSocketType {
		for {
			_, err := os.Stat(address)
			if err == nil {
				glog.Info("Found the OVS socket")
				break
			} else if os.IsNotExist(err) {
				glog.Infof("Given ovs-socket %q was not found, waiting for the socket to appear", address)
				time.Sleep(SocketConnectionTimeout)
			} else {
				glog.Fatalf("Failed opening the OVS socket with: %v", err)
			}
		}
	} else if socketType == TcpSocketType {
		conn, err := net.DialTimeout(socketType, address, SocketConnectionTimeout)
		if err == nil {
			glog.Info("Successfully connected to TCP socket")
			conn.Close()
			return endpoint
		}

		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			glog.Fatalf("Connection to %s timed out", address)
		} else if opErr, ok := err.(*net.OpError); ok {
			if opErr.Op == "dial" {
				glog.Fatalf("Connection to %s failed: %v", address, err)
			} else {
				glog.Fatalf("Unexpected error when connecting to %s: %v", address, err)
			}
		} else {
			glog.Fatalf("Unexpected error when connecting to %s: %v", address, err)
		}
	}
	return endpoint
}
