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

// XXX: This is a temporary netns helper lib that uses nsenter to execute
// commands in network namespaces. It should be replaced with proper netlink
// solution once we use Go 1.10 everywhere and netns Go bug [1] is gone.
// [1] https://www.weave.works/blog/linux-namespaces-and-go-don-t-mix

package plugin

import (
	"bytes"
	"fmt"
	"os/exec"
)

func withNetNS(nsPath string, cmd ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer

	args := append([]string{"--net=" + nsPath}, cmd...)
	c := exec.Command("nsenter", args...)
	c.Stdout = &stdout
	c.Stderr = &stderr
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("%s: %s", string(stderr.Bytes()), err)
	}

	return stdout.Bytes(), nil
}
