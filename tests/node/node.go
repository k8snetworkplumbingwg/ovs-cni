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

package node

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/k8snetworkplumbingwg/ovs-cni/tests/cmd"
)

// WorkerNode returns the name of the worker node used for tests.
func WorkerNode() string {
	if n := os.Getenv("OVS_WORKER_NODE"); n != "" {
		return n
	}
	return "ovs-cni-worker"
}

func RunAtNode(node string, command ...string) (string, error) {
	ssh := "./cluster/exec.sh"
	sshCommand := []string{node, "--"}
	sshCommand = append(sshCommand, command...)
	output, err := cmd.Run(ssh, sshCommand...)
	return output, err
}

func RunAtNodes(nodes []string, command ...string) (outputs []string, errs []error) {
	for _, node := range nodes {
		output, err := RunAtNode(node, command...)
		outputs = append(outputs, output)
		errs = append(errs, err)
	}
	return outputs, errs
}

// RemoveOvsBridgeOnNode removes ovs bridge on the worker node
func RemoveOvsBridgeOnNode(bridgeName string) {
	By("Removing ovs-bridge on the node")
	out, err := RunAtNode(WorkerNode(), "ovs-vsctl", "--if-exists", "del-br", bridgeName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command on node. stdout: %s", out))
}

// AddOvsBridgeOnNode add ovs bridge on the worker node
func AddOvsBridgeOnNode(bridgeName string) {
	By("Adding ovs-bridge on the node")
	out, err := RunAtNode(WorkerNode(), "ovs-vsctl", "add-br", bridgeName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command on node. stdout: %s", out))
}
