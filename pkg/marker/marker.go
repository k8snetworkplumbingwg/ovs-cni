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

package marker

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	clientset kubernetes.Interface
)

const (
	// Expose available bridges as resources in format ovs-cni.network.kubevirt.io/[bridge name]
	resourceNamespace = "ovs-cni.network.kubevirt.io"
	// Kubernetes API does not support infinite resources, assume that 1000 connections is enough
	resourceDefaultValue = "1000"
)

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func init() {
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("Error while obtaining cluster config: %v", err)
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("Error building example clientset: %v", err)
	}
}

func getAvailableResources(ovsSocket string) (map[string]bool, error) {
	outputRaw, err := exec.Command(
		"ovs-vsctl",
		"--db", ovsSocket,
		"list-br",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list OVS bridges: %v", string(outputRaw))
	}

	availableResources := make(map[string]bool)
	for _, bridgeName := range strings.Split(strings.TrimSpace(string(outputRaw)), "\n") {
		availableResources[bridgeName] = true
	}

	return availableResources, nil
}

func getReportedResources(nodeName string) (map[string]bool, error) {
	reportedResources := make(map[string]bool)
	node, err := clientset.
		CoreV1().
		Nodes().
		Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %v", err)
	}

	for nodeResourceName, _ := range node.Status.Capacity {
		splitNodeResourceName := strings.Split(nodeResourceName.String(), "/")
		if len(splitNodeResourceName) == 2 && splitNodeResourceName[0] == resourceNamespace {
			reportedResources[splitNodeResourceName[1]] = true
		}
	}

	return reportedResources, nil
}

func Update(nodeName string, ovsSocket string) error {
	availableResources, err := getAvailableResources(ovsSocket)
	if err != nil {
		return fmt.Errorf("failed to list available resources: %v", err)
	}

	reportedResources, err := getReportedResources(nodeName)
	if err != nil {
		return fmt.Errorf("failed to list reported resources: %v", err)
	}

	patchOperations := make([]patchOperation, 0)

	for reportedResource, _ := range reportedResources {
		if _, available := availableResources[reportedResource]; !available {
			patchOperations = append(patchOperations, patchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("/status/capacity/%s~1%s", resourceNamespace, reportedResource),
			})
		}
	}

	for availableResource, _ := range availableResources {
		if _, reported := reportedResources[availableResource]; !reported {
			patchOperations = append(patchOperations, patchOperation{
				Op:    "add",
				Path:  fmt.Sprintf("/status/capacity/%s~1%s", resourceNamespace, availableResource),
				Value: resourceDefaultValue,
			})
		}
	}

	if len(patchOperations) == 0 {
		return nil
	}

	payloadBytes, err := json.Marshal(patchOperations)
	if err != nil {
		return fmt.Errorf("failed to marshal patch operations %s: %v", patchOperations, err)
	}

	_, err = clientset.
		CoreV1().
		Nodes().
		Patch(nodeName, types.JSONPatchType, payloadBytes, "status")
	if err != nil {
		return fmt.Errorf("failed to apply patch %s on node: %v", payloadBytes, err)
	}

	return nil
}
