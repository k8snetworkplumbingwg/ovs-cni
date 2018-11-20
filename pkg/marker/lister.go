package marker

import (
	"encoding/json"
	"os/exec"
	"time"

	"github.com/golang/glog"
	"github.com/kubevirt/device-plugin-manager/pkg/dpm"
)

const (
	resourceNamespace     = "ovs-cni.network.kubevirt.io"
	resourceNameAttribute = "ovs-cni-resource"
)

type Lister struct{}

func (Lister) GetResourceNamespace() string {
	return resourceNamespace
}

func (Lister) Discover(pluginListCh chan dpm.PluginNameList) {
	for {
		var plugins = make(dpm.PluginNameList, 0)

		outputRaw, err := exec.Command(
			"ovs-vsctl",
			"--db", "unix:///host/var/run/openvswitch/db.sock",
			"--format", "json",
			"--column", "external_ids",
			"find", "Bridge", "external_ids:ovs-cni-resource!=''",
		).CombinedOutput()
		if err != nil {
			glog.Errorf("Failed to list OVS bridges marked with resources they provide: %v", string(outputRaw))
			time.Sleep(10 * time.Second)
			continue
		}

		var output map[string]interface{}
		err = json.Unmarshal(outputRaw, &output)
		if err != nil {
			glog.Errorf("Failed to unmarshal OVS bridges json %s: %v", string(outputRaw), err)
			time.Sleep(10 * time.Second)
			continue
		}

		for _, row := range output["data"].([]interface{}) {
			resource := row.([]interface{})[0].([]interface{})[1].([]interface{})[0].([]interface{})[1].(string)
			plugins = append(plugins, resource)
		}

		pluginListCh <- plugins
		time.Sleep(10 * time.Second)
	}
}

func (Lister) NewPlugin(resourceName string) dpm.PluginInterface {
	glog.V(3).Infof("Creating device plugin %s", resourceName)
	return &DevicePlugin{resourceName: resourceName}
}
