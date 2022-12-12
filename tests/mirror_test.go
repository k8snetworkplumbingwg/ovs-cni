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
	"time"

	"github.com/k8snetworkplumbingwg/ovs-cni/tests/node"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ovs-mirror 0.3.0", func() { testMirrorFunc("0.3.0") })
var _ = Describe("ovs-mirror 0.3.1", func() { testMirrorFunc("0.3.1") })
var _ = Describe("ovs-mirror 0.4.0", func() { testMirrorFunc("0.4.0") })

var testMirrorFunc = func(version string) {
	Describe("ovs traffic mirroring tests", func() {
		Context("when an OVS bridge is configured on a node", func() {
			const bridgeName = "br-test"
			BeforeEach(func() {
				node.AddOvsBridgeOnNode(bridgeName)
			})
			AfterEach(func() {
				node.RemoveOvsBridgeOnNode(bridgeName)
			})

			Context("and both consumer and producer network attachment definitions are defined with a mirror configuration", func() {
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

				AfterEach(func() {
					clusterApi.RemoveNetworkAttachmentDefinition(nadProducerName)
					clusterApi.RemoveNetworkAttachmentDefinition(nadConsumerName)
				})

				Context("and 3 pods (2 producers and 1 consumer) are connected through it", func() {
					const (
						podProd1Name = "pod-prod-1"
						podProd2Name = "pod-prod-2"
						podConsName  = "pod-cons"
						cidrPodProd1 = "10.0.0.1/24"
						cidrPodProd2 = "10.0.0.2/24"
						cidrCons     = "10.1.0.1/24"
					)
					BeforeEach(func() {
						consAdditionalCommands := "apk add tcpdump; tcpdump -i net1 > /tcpdump.log;"
						clusterApi.CreatePrivilegedPodWithIP(podConsName, nadConsumerName, bridgeName, cidrCons, consAdditionalCommands)
						Eventually(func() error {
							_, err := clusterApi.ReadFileFromPod(podConsName, "test", "/tcpdump.log")
							return err
						}, 120 * time.Second, time.Second).Should(Succeed(), "tcpdump did not start in time");

						clusterApi.CreatePrivilegedPodWithIP(podProd1Name, nadProducerName, bridgeName, cidrPodProd1, "")
						clusterApi.CreatePrivilegedPodWithIP(podProd2Name, nadProducerName, bridgeName, cidrPodProd2, "")
					})
					AfterEach(func() {
						clusterApi.DeletePodsInTestNamespace()
					})

					Specify("consumer pod should be able to monitor network traffic between producer pods", func() {
						By("Running and parsing tcpdump result")
						ipPodProd1, _, err := net.ParseCIDR(cidrPodProd1)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should succeed parsing podProd1's cidr: %s", cidrPodProd1))
						ipPodProd2, _, err := net.ParseCIDR(cidrPodProd2)
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should succeed parsing podProd2's cidr: %s", cidrPodProd2))

						By("Pinging over the network")
						err = clusterApi.PingFromPod(podProd1Name, "test", ipPodProd2.String())
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to ping from pod '%s@%s' to pod '%s@%s'", podProd1Name, ipPodProd1.String(), podProd2Name, ipPodProd2.String()))

						By("Confirming that the communication was recorded")
						tcpDumpResult, err := clusterApi.ReadFileFromPod(podConsName, "test", "/tcpdump.log")
						Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("should be able to read 'tcdump' log file from pod '%s'", podConsName))
						Expect(tcpDumpResult).To(ContainSubstring("IP " + ipPodProd1.String() + " > " + ipPodProd2.String() + ": ICMP echo request"))
						Expect(tcpDumpResult).To(ContainSubstring("IP " + ipPodProd2.String() + " > " + ipPodProd1.String() + ": ICMP echo reply"))
					})
				})
			})
		})
	})
}
