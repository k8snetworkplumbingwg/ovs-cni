// Copyright 2026 Dai Sheng
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

package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/ovsdb"
)

const resourcePrefix = "ovs-cni.network.kubevirt.io/"

// VxlanController maintains VXLAN tunnel connectivity between nodes
type VxlanController struct {
	myNodeName string
	kubeClient kubernetes.Interface
	ovsDriver  *ovsdb.OvsDriver
	informer   cache.SharedIndexInformer
	nodeLister corev1listers.NodeLister
}

// RunVxlanController initializes and starts the controller
func RunVxlanController(nodeName, ovsSocket string) error {
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to get k8s config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}
	ovsDriver, err := ovsdb.NewOvsDriver(ovsSocket)
	if err != nil {
		return fmt.Errorf("failed to create OVS driver: %w", err)
	}
	informerFactory := informers.NewSharedInformerFactory(clientset, time.Minute*10)
	nodeInformer := informerFactory.Core().V1().Nodes().Informer()
	nodeLister := informerFactory.Core().V1().Nodes().Lister()

	ctrl := &VxlanController{
		myNodeName: nodeName,
		kubeClient: clientset,
		ovsDriver:  ovsDriver,
		informer:   nodeInformer,
		nodeLister: nodeLister,
	}

	// Register event handlers for node lifecycle events (Add, Update, Delete)
	nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ctrl.onNodeAdd,
		UpdateFunc: ctrl.onNodeUpdate,
		DeleteFunc: ctrl.onNodeDelete,
	})

	stopCh := make(chan struct{})
	defer close(stopCh)
	go nodeInformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, nodeInformer.HasSynced) {
		return fmt.Errorf("timed out waiting for caches to sync")
	}

	glog.Infof("Smart VXLAN Controller synced and listening for bridge changes")
	<-stopCh
	return nil
}

func (c *VxlanController) onNodeAdd(obj interface{}) {
	node := obj.(*corev1.Node)
	// Treat Add as a change from nil to newNode
	c.reconcileNodeChange(nil, node)
}

func (c *VxlanController) onNodeUpdate(oldObj, newObj interface{}) {
	oldNode := oldObj.(*corev1.Node)
	newNode := newObj.(*corev1.Node)
	c.reconcileNodeChange(oldNode, newNode)
}

func (c *VxlanController) onNodeDelete(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		// Handle DeletedFinalStateUnknown (happens when the watcher misses the delete event
		// but we get it from the cache expiration)
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			glog.Errorf("Couldn't get object from tombstone %#v", obj)
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			glog.Errorf("Tombstone contained object that is not a Node %#v", obj)
			return
		}
	}
	// Treat Delete as a change from oldNode to nil
	c.reconcileNodeChange(node, nil)
}

// reconcileNodeChange handles the logic for diffing bridge state and applying changes.
// It handles Add (oldNode=nil), Update (both!=nil), and Delete (newNode=nil).
func (c *VxlanController) reconcileNodeChange(oldNode, newNode *corev1.Node) {
	var oldBridges, newBridges map[string]bool
	var targetNodeName string
	var peerIP string

	if oldNode != nil {
		oldBridges = getOvsBridgesFromNode(oldNode)
		targetNodeName = oldNode.Name
	} else {
		oldBridges = make(map[string]bool)
	}

	if newNode != nil {
		newBridges = getOvsBridgesFromNode(newNode)
		targetNodeName = newNode.Name
		peerIP = getNodeInternalIP(newNode)
	} else {
		newBridges = make(map[string]bool)
	}

	addedBridges := diffBridges(newBridges, oldBridges)
	removedBridges := diffBridges(oldBridges, newBridges)

	if len(addedBridges) == 0 && len(removedBridges) == 0 {
		return
	}

	// Scenario A: Local node change (local bridge created, need to connect to peers)
	if targetNodeName == c.myNodeName {
		// If local node is deleted (newNode == nil), we don't need to do anything as the pod is terminating anyway.
		if newNode == nil {
			return
		}
		for brName := range addedBridges {
			glog.Infof("Local node created bridge %q. Searching for peers...", brName)
			c.connectToPeersWithBridge(brName)
		}
		// Ignore local bridge deletion; OVS automatically removes ports when the bridge is deleted
		return
	}

	// Scenario B: Peer node change (need to check if we have the same bridge locally)

	// Handle remote bridge creation
	if newNode != nil && peerIP != "" {
		for brName := range addedBridges {
			if exist, _ := c.ovsDriver.IsBridgePresent(brName); exist {
				portName := fmt.Sprintf("vx-%s-%s", brName, targetNodeName)
				glog.Infof("Remote node %s created bridge %q. Establishing VXLAN %s to %s", targetNodeName, brName, portName, peerIP)

				bDriver := c.ovsDriver.NewBridgeDriverFromExisting(brName)

				if err := bDriver.CreateVxlanPort(portName, peerIP); err != nil {
					glog.Errorf("Failed to create VXLAN port %s: %v", portName, err)
				}
			}
		}
	}

	// Handle remote bridge deletion
	for brName := range removedBridges {
		portName := fmt.Sprintf("vx-%s-%s", brName, targetNodeName)

		if exist, _ := c.ovsDriver.IsBridgePresent(brName); exist {
			glog.Infof("Remote node %s removed bridge %q (or node deleted). Tearing down VXLAN %s", targetNodeName, brName, portName)

			bDriver := c.ovsDriver.NewBridgeDriverFromExisting(brName)

			if err := bDriver.DeletePort(portName); err != nil {
				glog.Warningf("Failed to delete VXLAN port %s: %v", portName, err)
			}
		}
	}
}

// connectToPeersWithBridge iterates over all nodes to find peers with the same bridge and establishes connectivity
func (c *VxlanController) connectToPeersWithBridge(brName string) {
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		glog.Errorf("Failed to list nodes for bridge %q sync: %v", brName, err)
		return
	}

	bDriver := c.ovsDriver.NewBridgeDriverFromExisting(brName)

	for _, node := range nodes {
		if node.Name == c.myNodeName {
			continue
		}

		bridges := getOvsBridgesFromNode(node)
		if !bridges[brName] {
			continue
		}

		peerIP := getNodeInternalIP(node)
		if peerIP == "" {
			glog.Warningf("Peer %s has bridge %q but no InternalIP, skipping", node.Name, brName)
			continue
		}

		// Establish VXLAN tunnel
		portName := fmt.Sprintf("vx-%s-%s", brName, node.Name)
		glog.Infof("Peer %s also has bridge %q. Establishing VXLAN %s to %s", node.Name, brName, portName, peerIP)

		if err := bDriver.CreateVxlanPort(portName, peerIP); err != nil {
			glog.Errorf("Failed to establish VXLAN tunnel %s to %s: %v", portName, node.Name, err)
		}
	}
}

// --- Helper functions ---

// getOvsBridgesFromNode extracts OVS bridge names from Node.Status.Capacity
func getOvsBridgesFromNode(node *corev1.Node) map[string]bool {
	bridges := make(map[string]bool)
	if node == nil {
		return bridges
	}
	for resourceName := range node.Status.Capacity {
		if strings.HasPrefix(resourceName.String(), resourcePrefix) {
			brName := strings.TrimPrefix(resourceName.String(), resourcePrefix)
			bridges[brName] = true
		}
	}
	return bridges
}

// diffBridges returns bridges present in setA but not in setB
func diffBridges(setA, setB map[string]bool) map[string]bool {
	diff := make(map[string]bool)
	for br := range setA {
		if !setB[br] {
			diff[br] = true
		}
	}
	return diff
}

func getNodeInternalIP(node *corev1.Node) string {
	if node == nil {
		return ""
	}
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			return address.Address
		}
	}
	return ""
}
