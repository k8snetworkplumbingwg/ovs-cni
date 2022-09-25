// Copyright 2018 Red Hat, Inc.
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

package tests_test

import (
	"context"
	"fmt"
	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/k8snetworkplumbingwg/ovs-cni/tests/node"
)

var _ = Describe("ovs-cni 0.3.0", func() { testFunc("0.3.0") })
var _ = Describe("ovs-cni 0.3.1", func() { testFunc("0.3.1") })
var _ = Describe("ovs-cni 0.4.0", func() { testFunc("0.4.0") })

var testFunc = func(version string) {
	Describe("pod availability tests", func() {
		Context("When ovs-cni is deployed on the cluster", func() {
			Specify("ovs-cni pod should be up and running", func() {
				pods, _ := clusterApi.Clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: "app=ovs-cni"})
				Expect(len(pods.Items)).To(BeNumerically(">", 0), "should have at least 1 ovs-cni pod deployed")
			})
		})
	})

	Describe("pod ovs-bridge connectivity tests", func() {
		Context("when an OVS bridge is configured on a node", func() {
			const bridgeName = "br-test"
			BeforeEach(func() {
				node.AddOvsBridgeOnNode(bridgeName)
			})
			AfterEach(func() {
				node.RemoveOvsBridgeOnNode(bridgeName)
			})

			Context("and a network attachment definition is defined", func() {
				const nadName = "ovs-net"
				BeforeEach(func() {
					clusterApi.CreateNetworkAttachmentDefinition(nadName, bridgeName, `{ "cniVersion": "`+version+`", "type": "ovs", "bridge": "`+bridgeName+`", "vlan": 100 }`)
				})
				AfterEach(func() {
					clusterApi.RemoveNetworkAttachmentDefinition(nadName)
				})

				Context("and two pods are connected through it", func() {
					const (
						pod1Name = "pod-test-1"
						pod2Name = "pod-test-2"
						cidrPod1 = "10.0.0.1/24"
						cidrPod2 = "10.0.0.2/24"
					)
					BeforeEach(func() {
						clusterApi.CreatePrivilegedPodWithIP(pod1Name, nadName, bridgeName, cidrPod1, "")
						clusterApi.CreatePrivilegedPodWithIP(pod2Name, nadName, bridgeName, cidrPod2, "")
					})
					AfterEach(func() {
						clusterApi.DeletePodsInTestNamespace()
					})

					Specify("they should be able to communicate over the network", func() {
						By("Checking pods connectivity by pinging from one to the other")
						ipPod1, _, err := net.ParseCIDR(cidrPod1)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should succeed parsing pod's cidr: %s", cidrPod1))
						ipPod2, _, err := net.ParseCIDR(cidrPod2)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should succeed parsing pod's cidr: %s", cidrPod2))

						err = clusterApi.PingFromPod(pod1Name, "test", ipPod2.String())
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to ping from pod '%s@%s' to pod '%s@%s'", pod1Name, ipPod2.String(), pod2Name, ipPod1.String()))
					})
				})
			})
		})
	})
}
