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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/cache"
	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
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

// Marker object containing k8s in cluster api and ovs config
type Marker struct {
	nodeName  string
	clientset kubernetes.Interface
	ovsdb     *ovsdb.OvsDriver
}

// NewMarker creates new Marker object
func NewMarker(nodeName string, ovsSocket string) (*Marker, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Error while obtaining cluster config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Error building example clientset: %v", err)
	}

	ovsDriver, err := ovsdb.NewOvsDriver(ovsSocket)
	if err != nil {
		return nil, fmt.Errorf("Error creating the ovsdb connection: %v", err)
	}

	return &Marker{clientset: clientset, nodeName: nodeName, ovsdb: ovsDriver}, nil
}

func (m *Marker) getAvailableResources() (map[string]bool, error) {
	bridges, err := m.ovsdb.BridgeList()
	if err != nil {
		return nil, err
	}

	availableResources := make(map[string]bool)
	for _, bridgeName := range bridges {
		availableResources[bridgeName] = true
	}

	return availableResources, nil
}

// GetReportedResources returns bridges that are reported on the node object
func (m *Marker) GetReportedResources() (map[string]bool, error) {
	reportedResources := make(map[string]bool)
	node, err := m.clientset.
		CoreV1().
		Nodes().
		Get(context.TODO(), m.nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %v", err)
	}

	for nodeResourceName := range node.Status.Capacity {
		splitNodeResourceName := strings.Split(nodeResourceName.String(), "/")
		if len(splitNodeResourceName) == 2 && splitNodeResourceName[0] == resourceNamespace {
			reportedResources[splitNodeResourceName[1]] = true
		}
	}

	return reportedResources, nil
}

// Update reports ovs bridge status to api server
func (m *Marker) Update(cache *cache.Cache) error {
	availableResources, err := m.getAvailableResources()
	if err != nil {
		return fmt.Errorf("failed to list available resources: %v", err)
	}

	reportedResources := cache.Bridges()

	patchOperations := make([]patchOperation, 0)

	for reportedResource := range reportedResources {
		if _, available := availableResources[reportedResource]; !available {
			patchOperations = append(patchOperations, patchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("/status/capacity/%s~1%s", resourceNamespace, reportedResource),
			})
		}
	}

	for availableResource := range availableResources {
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

	_, err = m.clientset.
		CoreV1().
		Nodes().
		Patch(context.TODO(), m.nodeName, types.JSONPatchType, payloadBytes, metav1.PatchOptions{}, "status")
	if err != nil {
		return fmt.Errorf("failed to apply patch %s on node: %v", payloadBytes, err)
	}

	cache.Refresh(availableResources)
	return nil
}
