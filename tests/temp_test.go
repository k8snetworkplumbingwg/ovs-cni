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
