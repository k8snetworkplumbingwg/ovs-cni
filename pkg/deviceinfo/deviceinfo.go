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

package deviceinfo

import (
	"encoding/json"
	"os"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/k8snetworkplumbingwg/ovs-cni/pkg/types"
)

const RootDeviceInfoDirectory = "/var/run/k8s.cni.cncf.io/devinfo/cni"

func readDeviceInfo(devInfoPath string) (*netv1.DeviceInfo, error) {
	devInfoBytes, err := os.ReadFile(devInfoPath)
	if err != nil {
		return nil, err
	}

	var deviceInfo netv1.DeviceInfo
	err = json.Unmarshal(devInfoBytes, &deviceInfo)
	if err != nil {
		return nil, err
	}

	return &deviceInfo, nil
}

func GetDeviceInfo(netconf *types.NetConf) (*netv1.DeviceInfo, error) {
	if netconf.RuntimeConfig == nil || netconf.RuntimeConfig.CNIDeviceInfoFile == "" {
		return nil, nil
	}

	return readDeviceInfo(netconf.RuntimeConfig.CNIDeviceInfoFile)
}
