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
	"flag"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var _ = Describe("ovs-cni tests", func() {
	Describe("pod availability tests", func() {
		var kubeconfig *string
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		flag.Parse()
		config, _ := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		clientset, _ := kubernetes.NewForConfig(config)
		pods, _ := clientset.CoreV1().Pods("").List(v1.ListOptions{})
		Context("pod availability tests", func() {
			It("assert pods exists", func() {
				Expect(len(pods.Items)).Should(BeNumerically(">", 0))
			})
		})
	})
})
