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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubevirt.io/ovs-cni/tests"
)

var _ = Describe("ovs-cni-marker", func() {
	Describe("bridge resource reporting", func() {
		It("should be reported only when available on node", func() {
			out, err := tests.RunOnNode("node01", "sudo ovs-vsctl add-br br-test")
			if err != nil {
				panic(fmt.Errorf("%v: %s", err, out))
			}
			defer tests.RunOnNode("node01", "sudo ovs-vsctl --if-exists del-br br-test")

			Eventually(func() bool {
				node, err := clientset.CoreV1().Nodes().Get("node01", v1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				capacity, reported := node.Status.Capacity["ovs-cni.network.kubevirt.io/br-test"]
				if !reported {
					return false
				}
				capacityInt, _ := capacity.AsInt64()
				if capacityInt != int64(1000) {
					return false
				}
				return true
			}, 20*time.Second, 5*time.Second).Should(Equal(true))

			out, err = tests.RunOnNode("node01", "sudo ovs-vsctl --if-exists del-br br-test")
			if err != nil {
				panic(fmt.Errorf("%v: %s", err, out))
			}

			Eventually(func() bool {
				node, err := clientset.CoreV1().Nodes().Get("node01", v1.GetOptions{})
				Expect(err).ToNot(HaveOccurred())
				_, reported := node.Status.Capacity["ovs-cni.network.kubevirt.io/br-test"]
				return reported

			}, 20*time.Second, 5*time.Second).Should(Equal(false))
		})
	})
})
