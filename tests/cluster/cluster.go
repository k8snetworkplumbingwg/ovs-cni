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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

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
)

type ClusterApi struct {
	Clientset  *kubernetes.Clientset
	NetClient  *netclient.K8sCniCncfIoV1Client
	RestConfig *restclient.Config
}

const testNamespace = "test-namespace"

func NewClusterApi(kubeconfig string) *ClusterApi {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).ToNot(HaveOccurred())
	clientset, err := kubernetes.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred())
	netClient, err := netclient.NewForConfig(restConfig)
	Expect(err).ToNot(HaveOccurred())

	return &ClusterApi{
		Clientset:  clientset,
		NetClient:  netClient,
		RestConfig: restConfig,
	}
}

func (api *ClusterApi) CreateTestNamespace() {
	By(fmt.Sprintf("Creating %s namespace", testNamespace))
	_, err := api.Clientset.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed creating test namespace")
}

func (api *ClusterApi) RemoveTestNamespace() {
	By(fmt.Sprintf("Waiting for namespace %s to be removed, this can take a while ...", testNamespace))
	err := api.Clientset.CoreV1().Namespaces().Delete(context.TODO(), testNamespace, metav1.DeleteOptions{})
	Expect(err).To(SatisfyAny(BeNil(), WithTransform(apierrors.IsNotFound, BeTrue())), "Should succeed deleting namespace if exists")

	EventuallyWithOffset(1, func() error {
		_, err := api.Clientset.CoreV1().Namespaces().Get(context.TODO(), testNamespace, metav1.GetOptions{})
		return err
	}, 120*time.Second, 5*time.Second).Should(SatisfyAll(HaveOccurred(), WithTransform(apierrors.IsNotFound, BeTrue())), "Should succeed terminating the namespace")
}

func (api *ClusterApi) CreatePrivilegedPodWithIp(podName, nadName, bridgeName, cidr string) {
	By(fmt.Sprintf("Creating pod %s with priviliged premission and ip %s", podName, cidr))
	privileged := true
	resourceList := make(corev1.ResourceList)
	resourceList[corev1.ResourceName("ovs-cni.network.kubevirt.io/"+bridgeName)] = resource.Quantity{}

	podObject := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      podName,
		Namespace: testNamespace,
		// This annotation makes sure the pod is assigned to a node that has this ovs bridge resource
		Annotations: map[string]string{"k8s.v1.cni.cncf.io/networks": nadName},
	},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test",
			Image:           "alpine",
			Command:         []string{"sh", "-c", fmt.Sprintf("ip address add %s dev net1; sleep 99999", cidr)},
			Resources:       corev1.ResourceRequirements{Limits: resourceList},
			SecurityContext: &corev1.SecurityContext{Privileged: &privileged}}}}}

	_, err := api.Clientset.CoreV1().Pods(testNamespace).Create(context.TODO(), podObject, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed creating pod object")

	By("Waiting for pod container to be in Running state")
	Eventually(func() bool {
		pod, err := api.Clientset.CoreV1().Pods(testNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			return false
		}

		if len(pod.Status.ContainerStatuses) > 0 {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				if containerStatus.Ready != true {
					return false
				}
			}

			return true
		}
		return false

	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Should succeed getting pod container to Ready state")
}

func (api *ClusterApi) DeletePodsInTestNamespace() {
	By(fmt.Sprintf("Cleaning Pods in %s namespace", testNamespace))
	podList, err := api.Clientset.CoreV1().Pods(testNamespace).List(context.TODO(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed getting pod list in test namespace")

	for _, pod := range podList.Items {
		err = api.Clientset.CoreV1().Pods(testNamespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Should succeed deleting pod %s", pod.Name))
	}

	Eventually(func() []corev1.Pod {
		podsList, err := api.Clientset.CoreV1().Pods(testNamespace).List(context.TODO(), metav1.ListOptions{})
		Expect(err).ToNot(HaveOccurred(), "Should succeed getting pod list in test namespace after deletion")
		return podsList.Items
	}, 6*time.Minute, time.Second).Should(BeEmpty(), "Failed to Delete pods")
}

func (api *ClusterApi) CreateNetworkAttachmentDefinition(nadName, bridgeName, config string) {
	By(fmt.Sprintf("Adding NetworkAttachmentDefinition %s of ovs bridge", nadName))
	nad := &netv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nadName,
			Namespace:   testNamespace,
			Annotations: map[string]string{"k8s.v1.cni.cncf.io/resourceName": "ovs-cni.network.kubevirt.io/" + bridgeName},
		},
		Spec: netv1.NetworkAttachmentDefinitionSpec{
			Config: config,
		},
	}

	_, err := api.NetClient.NetworkAttachmentDefinitions(nad.Namespace).Create(context.TODO(), nad, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed creating nad NetworkAttachmentDefinition")
}

func (api *ClusterApi) RemoveNetworkAttachmentDefinition(nadName string) {
	By("Cleaning NetworkAttachmentDefinition")
	err := api.NetClient.NetworkAttachmentDefinitions(testNamespace).Delete(context.TODO(), nadName, metav1.DeleteOptions{})
	Expect(err).ToNot(HaveOccurred(), "Should succeed deleting nad NetworkAttachmentDefinition")
}

func (api *ClusterApi) PingFromPod(podName, containerName, targetIp string) error {
	out, _, err := api.execOnPod(podName, containerName, testNamespace, "ping -c 5 "+targetIp)
	if err != nil {
		return errors.Wrapf(err, "Failed to run exec on pod %s", podName)
	}

	if !strings.Contains(out, "0% packet loss") {
		return fmt.Errorf("ping failed. output: %s", out)
	}

	return nil
}

func (api *ClusterApi) execOnPod(podName, containerName, namespace, command string) (string, string, error) {
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
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return "", "", errors.Wrapf(err, "error running remote post command %s on pod %s/%s", command, namespace, podName)
	}

	return stdout.String(), stderr.String(), nil
}
