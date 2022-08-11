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
	"fmt"
	"net"

	"github.com/k8snetworkplumbingwg/ovs-cni/tests/node"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// var _ = Describe("ovs-cni 0.3.0", func() { testFunc("0.3.0") })
var _ = Describe("ovs-cni 0.3.1", func() { testFunc("0.3.1") })

// var _ = Describe("ovs-cni 0.4.0", func() { testFunc("0.4.0") })

var testFunc = func(version string) {
	// Describe("pod availability tests", func() {
	// 	Context("When ovs-cni is deployed on the cluster", func() {
	// 		Specify("ovs-cni pod should be up and running", func() {
	// 			pods, _ := clusterApi.Clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: "app=ovs-cni"})
	// 			Expect(len(pods.Items)).To(BeNumerically(">", 0), "should have at least 1 ovs-cni pod deployed")
	// 		})
	// 	})
	// })

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
				const nadProducerName = "ovs-net-prod"
				const nadConsumerName = "ovs-net-cons"

				BeforeEach(func() {
					ovsPluginProd := `{ "type": "ovs", "bridge": "` + bridgeName + `", "vlan": 100 }`
					mirrorConfProd := `{ "name": "mirror-1", "ingress": true, "egress": true }`
					mirrorProducer := `{ "type": "ovs-mirror-producer", "bridge": "` + bridgeName + `", "mirrors": [` + mirrorConfProd + `] }`
					plugins := `[` + ovsPluginProd + `, ` + mirrorProducer + `]`
					clusterApi.CreateNetworkAttachmentDefinition(nadProducerName, bridgeName, `{ "cniVersion": "`+version+`", "plugins": `+plugins+`}`)

					ovsPluginCons := `{ "type": "ovs", "bridge": "` + bridgeName + `", "vlan": 0 }`
					mirrorConfCons := `{ "name": "mirror-1" }`
					mirrorConsumer := `{ "type": "ovs-mirror-consumer", "bridge": "` + bridgeName + `", "mirrors": [` + mirrorConfCons + `] }`
					pluginsConsumer := `[` + ovsPluginCons + `, ` + mirrorConsumer + `]`
					clusterApi.CreateNetworkAttachmentDefinition(nadConsumerName, bridgeName, `{ "cniVersion": "`+version+`", "plugins": `+pluginsConsumer+`}`)
				})

				// AfterEach(func() {
				// 	clusterApi.RemoveNetworkAttachmentDefinition(nadName)
				// })

				Context("and two pods are connected through it", func() {
					const (
						podProd1Name = "pod-prod-1"
						podProd2Name = "pod-prod-2"
						podConsName  = "pod-cons"
						cidrPodProd1 = "10.0.0.1/24"
						cidrPodProd2 = "10.0.0.2/24"
						cidrCons     = "10.1.0.1/24"
					)
					BeforeEach(func() {
						clusterApi.CreatePrivilegedPodWithIP(podProd1Name, nadProducerName, bridgeName, cidrPodProd1)
						clusterApi.CreatePrivilegedPodWithIP(podProd2Name, nadProducerName, bridgeName, cidrPodProd2)
						clusterApi.CreatePrivilegedPodWithIP(podConsName, nadConsumerName, bridgeName, cidrCons)
					})
					// AfterEach(func() {
					// 	clusterApi.DeletePodsInTestNamespace()
					// })

					Specify("they should be able to communicate over the network", func() {
						By("Checking pods connectivity by pinging from one to the other")
						ipPodProd1, _, err := net.ParseCIDR(cidrPodProd1)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should succeed parsing podProd1's cidr: %s", cidrPodProd1))
						ipPodProd2, _, err := net.ParseCIDR(cidrPodProd2)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should succeed parsing podProd2's cidr: %s", cidrPodProd2))

						err = clusterApi.InstallOnPod(podConsName, "test", "tcpdump")
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to install 'tcpdump' on pod '%s'", podConsName))

						err = clusterApi.TcpdumpOnPod(podConsName, "test")
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to run tcpdump on pod '%s'", podConsName))

						err = clusterApi.PingFromPod(podProd1Name, "test", ipPodProd2.String())
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to ping from pod '%s@%s' to pod '%s@%s'", podProd1Name, ipPodProd1.String(), podProd2Name, ipPodProd2.String()))

						result, err := clusterApi.ReadTCPDumpFromPod(podConsName, "test")
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to read 'tcdump' log file from pod '%s'", podConsName))
						Expect(result).To(ContainSubstring("IP " + ipPodProd1.String() + " > " + ipPodProd2.String()))
					})
				})
			})
		})
	})
}
