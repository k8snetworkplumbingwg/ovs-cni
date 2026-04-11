/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2018 Red Hat, Inc.
 *
 */

package cluster

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	netclient "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/client/clientset/versioned/typed/k8s.cni.cncf.io/v1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ClusterAPI object containing k8s cni cncf api client referneces
type ClusterAPI struct {
	Clientset  *kubernetes.Clientset
	NetClient  *netclient.K8sCniCncfIoV1Client
	RestConfig *restclient.Config
	Namespace  string
}

var namespaceSeq int32

// NewClusterAPI creates and returns new cluster API object
func NewClusterAPI(kubeconfig string) *ClusterAPI {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).ToNot(HaveOccurred())
	clientset, err := kubernetes.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred())
	netClient, err := netclient.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred())

	return &ClusterAPI{
		Clientset:  clientset,
		NetClient:  netClient,
		RestConfig: restConfig,
	}
}

// NewTestNamespace creates a uniquely-named test namespace and sets it on the API.
// Using unique namespaces per test allows asynchronous cleanup, avoiding the need
// to wait for pod termination before the next test can start.
func (api *ClusterAPI) NewTestNamespace() {
	seq := atomic.AddInt32(&namespaceSeq, 1)
	api.Namespace = fmt.Sprintf("test-ns-%d", seq)
	By(fmt.Sprintf("Creating %s namespace", api.Namespace))
	_, err := api.Clientset.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: api.Namespace}}, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed creating test namespace")
}

// DeleteTestNamespaceAsync deletes the current test namespace without waiting for
// completion. Kubernetes will garbage-collect all namespaced resources (pods, NADs).
func (api *ClusterAPI) DeleteTestNamespaceAsync() {
	By(fmt.Sprintf("Deleting %s namespace asynchronously", api.Namespace))
	err := api.Clientset.CoreV1().Namespaces().Delete(context.TODO(), api.Namespace, metav1.DeleteOptions{})
	Expect(err).To(SatisfyAny(BeNil(), WithTransform(apierrors.IsNotFound, BeTrue())), "Should succeed deleting namespace")
}

// CleanupTestNamespaces removes all namespaces with test prefixes from previous runs
func (api *ClusterAPI) CleanupTestNamespaces() {
	By("Cleaning up test namespaces")
	nsList, err := api.Clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())

	hasTestNs := false
	for _, ns := range nsList.Items {
		if strings.HasPrefix(ns.Name, "test-ns-") || ns.Name == "test-namespace" {
			hasTestNs = true
			_ = api.Clientset.CoreV1().Namespaces().Delete(context.TODO(), ns.Name, metav1.DeleteOptions{})
		}
	}

	if !hasTestNs {
		return
	}

	EventuallyWithOffset(1, func() bool {
		nsList, err := api.Clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return false
		}
		for _, ns := range nsList.Items {
			if strings.HasPrefix(ns.Name, "test-ns-") || ns.Name == "test-namespace" {
				return false
			}
		}
		return true
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Should succeed cleaning up test namespaces")
}

// CreatePrivilegedPodOnly creates a pod without waiting for it to become ready.
// Use WaitForPodReady to wait for readiness after creating multiple pods in parallel.
func (api *ClusterAPI) CreatePrivilegedPodOnly(podName, nadName, bridgeName, cidr, additionalCommands string) {
	By(fmt.Sprintf("Creating pod %s with priviliged premission and ip %s", podName, cidr))
	privileged := true
	resourceList := make(corev1.ResourceList)
	resourceList[corev1.ResourceName("ovs-cni.network.kubevirt.io/"+bridgeName)] = resource.Quantity{}

	podObject := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      podName,
		Namespace: api.Namespace,
		// This annotation makes sure the pod is assigned to a node that has this ovs bridge resource
		Annotations: map[string]string{"k8s.v1.cni.cncf.io/networks": nadName},
	},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test",
			Image:           "quay.io/jitesoft/alpine",
			Command:         []string{"sh", "-c", fmt.Sprintf("ip address add %s dev net1; "+additionalCommands+" sleep 99999", cidr)},
			Resources:       corev1.ResourceRequirements{Limits: resourceList},
			SecurityContext: &corev1.SecurityContext{Privileged: &privileged}}}}}

	_, err := api.Clientset.CoreV1().Pods(api.Namespace).Create(context.TODO(), podObject, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed creating pod object")
}

// WaitForPodReady waits for a specific pod to be in Ready state
func (api *ClusterAPI) WaitForPodReady(podName string) {
	By(fmt.Sprintf("Waiting for pod %s to be ready", podName))
	Eventually(func() bool {
		pod, err := api.Clientset.CoreV1().Pods(api.Namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			return false
		}

		if len(pod.Status.ContainerStatuses) > 0 {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if !containerStatus.Ready {
					return false
				}
			}

			return true
		}
		return false

	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Should succeed getting pod container to Ready state")
}

// CreatePrivilegedPodWithIP creates a pod attached with ovs via secondary network
// and waits for it to become ready.
func (api *ClusterAPI) CreatePrivilegedPodWithIP(podName, nadName, bridgeName, cidr, additionalCommands string) {
	api.CreatePrivilegedPodOnly(podName, nadName, bridgeName, cidr, additionalCommands)
	api.WaitForPodReady(podName)
}

// DeletePodsInTestNamespace deletes all the pods in the test namespace
func (api *ClusterAPI) DeletePodsInTestNamespace() {
	By(fmt.Sprintf("Cleaning Pods in %s namespace", api.Namespace))
	podList, err := api.Clientset.CoreV1().Pods(api.Namespace).List(context.TODO(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed getting pod list in test namespace")

	for _, pod := range podList.Items {
		err = api.Clientset.CoreV1().Pods(api.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Should succeed deleting pod %s", pod.Name))
	}

	Eventually(func() []corev1.Pod {
		podsList, err := api.Clientset.CoreV1().Pods(api.Namespace).List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred(), "Should succeed getting pod list in test namespace after deletion")
		return podsList.Items
	}, 6*time.Minute, time.Second).Should(BeEmpty(), "Failed to Delete pods")
}

// CreateNetworkAttachmentDefinition creates nad object on the test namespace
func (api *ClusterAPI) CreateNetworkAttachmentDefinition(nadName, bridgeName, config string) {
	By(fmt.Sprintf("Adding NetworkAttachmentDefinition %s of ovs bridge", nadName))
	nad := &netv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nadName,
			Namespace:   api.Namespace,
			Annotations: map[string]string{"k8s.v1.cni.cncf.io/resourceName": "ovs-cni.network.kubevirt.io/" + bridgeName},
		},
		Spec: netv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}

	_, err := api.NetClient.NetworkAttachmentDefinitions(nad.Namespace).Create(context.TODO(), nad, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed creating nad NetworkAttachmentDefinition")
}

// RemoveNetworkAttachmentDefinition deletes nad object from test namespace
func (api *ClusterAPI) RemoveNetworkAttachmentDefinition(nadName string) {
	By("Cleaning NetworkAttachmentDefinition")
	err := api.NetClient.NetworkAttachmentDefinitions(api.Namespace).Delete(context.TODO(), nadName, metav1.DeleteOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed deleting nad NetworkAttachmentDefinition")
}

// PingFromPod run the ping command on the pod container towards targetIP
func (api *ClusterAPI) PingFromPod(podName, containerName, targetIP string) error {
	out, _, err := api.execOnPod(podName, containerName, api.Namespace, "ping -c 5 "+targetIP)
	if err != nil {
		return errors.Wrapf(err, "Failed to run exec on pod %s", podName)
	}

	if !strings.Contains(out, "0% packet loss") {
		return fmt.Errorf("ping failed. output: %s", out)
	}

	return nil
}

// ReadFileFromPod run the cat command on the pod container to read the content of a file
func (api *ClusterAPI) ReadFileFromPod(podName, containerName, filePath string) (string, error) {
	out, _, err := api.execOnPod(podName, containerName, api.Namespace, "cat "+filePath)
	if err != nil {
		return "", errors.Wrapf(err, "Failed to run exec on pod %s", podName)
	}

	return out, nil
}

func (api *ClusterAPI) execOnPod(podName, containerName, namespace, command string) (string, string, error) {
	req := api.Clientset.CoreV1().RESTClient().
		Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec")
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		return "", "", errors.Wrap(err, "error creating scheme")
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(&corev1.PodExecOptions{
		Command:   strings.Fields(command),
		Container: containerName,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, parameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(api.RestConfig, "POST", req.URL())
	if err != nil {
		return "", "", errors.Wrap(err, "error creating remote post command")
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return "", "", errors.Wrapf(err, "error running remote post command %s on pod %s/%s", command, namespace, podName)
	}

	return stdout.String(), stderr.String(), nil
}
