/*
Copyright The Kubernetes NMState Authors.


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo"
)

// Run exec a command with its arguments and return stdout and stderr
func Run(command string, arguments ...string) (string, error) {
	cmd := exec.Command(command, arguments...)

	if _, err := ginkgo.GinkgoWriter.Write([]byte(command + " " + strings.Join(arguments, " ") + "\n")); err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	if _, err := ginkgo.GinkgoWriter.Write([]byte(fmt.Sprintf("stdout: %.500s...\n, stderr %s\n", stdout.String(), stderr.String()))); err != nil {
		return "", err
	}

	return stdout.String(), nil
}
