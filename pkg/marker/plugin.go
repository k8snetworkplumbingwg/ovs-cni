package marker

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

const (
	fakeDevicePath  = "/var/run/openvswitch/ovs-cni-marker-fakedev"
	devicesPoolSize = 1000
)

type DevicePlugin struct {
	resourceName string
}

func (*DevicePlugin) Start() error {
	err := createFakeBlockDevice()
	if err != nil {
		glog.Fatalf("Failed to create fake block device: %s", err)
	}

	return nil
}

func createFakeBlockDevice() error {
	_, err := os.Stat(fakeDevicePath)
	if err == nil {
		glog.V(3).Info("Fake block device already exists")
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	glog.V(3).Info("Creating fake block device")
	err = exec.Command("mknod", "host/"+fakeDevicePath, "b", "1", "1").Run()
	return err
}

func (dp *DevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	devices := dp.generateDevices()
	s.Send(&pluginapi.ListAndWatchResponse{Devices: devices})
	for {
		time.Sleep(10 * time.Second)
	}
}

func (dp *DevicePlugin) generateDevices() []*pluginapi.Device {
	var devices []*pluginapi.Device
	for i := 0; i < devicesPoolSize; i++ {
		devices = append(devices, &pluginapi.Device{
			ID:     fmt.Sprintf("%s--%02d", dp.resourceName, i),
			Health: pluginapi.Healthy,
		})
	}
	return devices
}

func (*DevicePlugin) Allocate(ctx context.Context, requests *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	var response pluginapi.AllocateResponse

	for _, request := range requests.ContainerRequests {
		var devices []*pluginapi.DeviceSpec
		for _ = range request.DevicesIDs {
			devices = append(devices, &pluginapi.DeviceSpec{
				HostPath:      fakeDevicePath,
				ContainerPath: "/tmp/ovs-cni-marker-fakedev",
				Permissions:   "r",
			})
		}

		response.ContainerResponses = append(response.ContainerResponses, &pluginapi.ContainerAllocateResponse{Devices: devices})

	}

	return &response, nil
}

// GetDevicePluginOptions returns options to be communicated with Device Manager
func (DevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return nil, nil
}

// PreStartContainer is called, if indicated by Device Plugin during registration phase,
// before each container start. Device plugin can run device specific operations
// such as reseting the device before making devices available to the container
func (DevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return nil, nil
}
