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
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/k8snetworkplumbingwg/ovs-cni/tests/cmd"
)

// TODO: Use job with a node affinity instead
func runAtNodeWithExtras(node string, command ...string) (string, error) {
	ssh := "./cluster/ssh.sh"
	sshCommand := []string{node, "--"}
	sshCommand = append(sshCommand, command...)
	output, err := cmd.Run(ssh, sshCommand...)
	// Remove first two lines from output, ssh.sh add garbage there
	outputLines := strings.Split(output, "\n")
	if len(outputLines) > 2 {
		output = strings.Join(outputLines[2:], "\n")
	}
	return output, err
}

func RunAtNode(node string, command ...string) (string, error) {
	return runAtNodeWithExtras(node, command...)
}

func RunAtNodes(nodes []string, command ...string) (outputs []string, errs []error) {
	for _, node := range nodes {
		output, err := RunAtNode(node, command...)
		outputs = append(outputs, output)
		errs = append(errs, err)
	}
	return outputs, errs
}

// RemoveOvsBridgeOnNode removes ovs bridge on the node
func RemoveOvsBridgeOnNode(bridgeName string) {
	By("Removing ovs-bridge on the node")
	out, err := RunAtNode("node01", "sudo ovs-vsctl --if-exists del-br "+bridgeName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command on node. stdout: %s", out))
}

// AddOvsBridgeOnNode add ovs bridge on the node
func AddOvsBridgeOnNode(bridgeName string) {
	By("Adding ovs-bridge on the node")
	out, err := RunAtNode("node01", "sudo ovs-vsctl add-br "+bridgeName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to run command on node. stdout: %s", out))
}
