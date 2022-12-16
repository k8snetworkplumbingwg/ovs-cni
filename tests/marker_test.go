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

package tests_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/k8snetworkplumbingwg/ovs-cni/tests/node"
)

var _ = Describe("ovs-cni-marker", func() {
	Describe("bridge resource reporting", func() {
		It("should be reported only when available on node", func() {
			out, err := node.RunAtNode("node01", "sudo ovs-vsctl add-br br-test")
			Expect(err).NotTo(HaveOccurred(), "Failed creating a test bridge: %v: %v", err, out)
			defer func() {
				out, err = node.RunAtNode("node01", "sudo ovs-vsctl --if-exists del-br br-test")
				Expect(err).NotTo(HaveOccurred(), "Failed cleaning up a test bridge: %v: %v", err, out)
			}()

			Eventually(func() bool {
				node, err := clusterApi.Clientset.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				capacity, reported := node.Status.Capacity["ovs-cni.network.kubevirt.io/br-test"]
				if !reported {
					return false
				}
				capacityInt, _ := capacity.AsInt64()
				return capacityInt == int64(1000)
			}, 180*time.Second, 5*time.Second).Should(Equal(true))

			out, err = node.RunAtNode("node01", "sudo ovs-vsctl --if-exists del-br br-test")
			Expect(err).NotTo(HaveOccurred(), "Failed removing a test bridge: %v: %v", err, out)

			Eventually(func() bool {
				node, err := clusterApi.Clientset.CoreV1().Nodes().Get(context.TODO(), "node01", metav1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				_, reported := node.Status.Capacity["ovs-cni.network.kubevirt.io/br-test"]
				return reported

			}, 180*time.Second, 5*time.Second).Should(Equal(false))
		})
	})

	Describe("Health Check", func() {
		It("Should not restart ovs-cni-marker container", func() {
			Consistently(func() bool {
				pods, err := clusterApi.Clientset.CoreV1().Pods("").List(context.TODO(),
					metav1.ListOptions{LabelSelector: "app=ovs-cni"})
				Expect(err).ToNot(HaveOccurred(), "should fetch ovs-cni pods successfully")
				Expect(pods.Items).ToNot(BeEmpty(), "ovs-cni pods should be running")

				for _, pod := range pods.Items {
					restartCount := pod.Status.ContainerStatuses[0].RestartCount
					if restartCount > 0 {
						return false
					}
				}
				return true
			}, 240*time.Second, 60*time.Second).Should(Equal(true))
		})
	})
})
