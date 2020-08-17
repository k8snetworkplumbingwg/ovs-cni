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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ovs-cni", func() {
	Describe("pod availability tests", func() {
		Context("When ovs-cni is deployed on the cluster", func() {
			Specify("ovs-cni pod should be up and running", func() {
				pods, _ := clusterApi.Clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: "app=ovs-cni"})
				Expect(len(pods.Items)).To(BeNumerically(">", 0), "should have at least 1 ovs-cni pod deployed")
			})
		})
	})
})
