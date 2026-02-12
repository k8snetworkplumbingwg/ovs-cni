// Copyright (c) 2026 VEXXHOST, Inc.
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

package types

import (
	"encoding/json"
	"testing"
)

func TestNetConfArgsCNIParsing(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		expectedOvnPort string
		expectedMAC    string
	}{
		{
			name: "args.cni with OvnPort",
			config: `{
				"cniVersion": "1.0.0",
				"name": "test-net",
				"type": "ovs",
				"bridge": "br-int",
				"args": {
					"cni": {
						"OvnPort": "test-port-id"
					}
				}
			}`,
			expectedOvnPort: "test-port-id",
			expectedMAC:    "",
		},
		{
			name: "args.cni with OvnPort and MAC",
			config: `{
				"cniVersion": "1.0.0",
				"name": "test-net",
				"type": "ovs",
				"bridge": "br-int",
				"args": {
					"cni": {
						"OvnPort": "my-ovn-port",
						"MAC": "fa:16:3e:aa:bb:cc"
					}
				}
			}`,
			expectedOvnPort: "my-ovn-port",
			expectedMAC:    "fa:16:3e:aa:bb:cc",
		},
		{
			name: "no args section",
			config: `{
				"cniVersion": "1.0.0",
				"name": "test-net",
				"type": "ovs",
				"bridge": "br-int"
			}`,
			expectedOvnPort: "",
			expectedMAC:    "",
		},
		{
			name: "empty args.cni",
			config: `{
				"cniVersion": "1.0.0",
				"name": "test-net",
				"type": "ovs",
				"bridge": "br-int",
				"args": {
					"cni": {}
				}
			}`,
			expectedOvnPort: "",
			expectedMAC:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var netconf NetConf
			if err := json.Unmarshal([]byte(tt.config), &netconf); err != nil {
				t.Fatalf("Failed to unmarshal config: %v", err)
			}

			var gotOvnPort, gotMAC string
			if netconf.Args != nil && netconf.Args.CNI != nil {
				gotOvnPort = netconf.Args.CNI.OvnPort
				gotMAC = netconf.Args.CNI.MAC
			}

			if gotOvnPort != tt.expectedOvnPort {
				t.Errorf("OvnPort = %q, want %q", gotOvnPort, tt.expectedOvnPort)
			}
			if gotMAC != tt.expectedMAC {
				t.Errorf("MAC = %q, want %q", gotMAC, tt.expectedMAC)
			}
		})
	}
}
