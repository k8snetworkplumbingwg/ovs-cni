# VDPA

This document covers ovs-cni support of vdpa devices. It describes how
they are supported, how to configure the environment and how to attach
vdpa devices to a ovs bridge by using ovs-cni.

## Support

VDPA devices can be backed up by a SR-IOV virtual function or a VDUSE
management device.

VDPA devices can be bound to a virtio or a vhost driver.

Currently, ovs-cni supports VDPA devices backed up by SR-IOV virtual
functions and bound to the vhost_vdpa driver.

## Environment preparation

There are several steps to take before attaching a vdpa device to a pod
through ovs-cni

### Prerequisites

- Mellanox ConnectX-6 dx NIC
- [sriov-network-device-plugin](https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin)
- [multus-cni](https://github.com/k8snetworkplumbingwg/multus-cni)
- [network-resources-injector](https://github.com/k8snetworkplumbingwg/network-resources-injector)

### VDPA device creation

VDPA devices can be created by using the
[sriov-network-operator][sriov-net-op]. However, this document covers
creating vdpa devices without relying on the operator.

#### ConnectX-6 Dx

Create some VFs by:

```bash
echo 2 > /sys/class/net/ens1f0np0/device/sriov_numvfs
```

Unbind the mlx5 driver, change the e-switch mode into switchdev, and
bind the drivers back:

```bash
echo 0000:65:00.0 > sys/bus/pci/drivers/mlx5_core/unbind
echo 0000:65:00.1 > sys/bus/pci/drivers/mlx5_core/unbind
devlink dev eswitch set pci/0000:65:00.0 mode switchdev
devlink dev eswitch set pci/0000:65:00.1 mode switchdev
echo 0000:65:00.0 > sys/bus/pci/drivers/mlx5_core/bind
echo 0000:65:00.1 > sys/bus/pci/drivers/mlx5_core/bind
```

Load drivers

```bash
modprobe vdpa
modprobe vhost_vdpa
modprobe mlx5_vdpa
```

Check the available mgmtdevs:

```bash
vdpa mgmtdev show
```

Create the vdpa devices:
```bash
vdpa dev add name vdpa:0000:65:00.0 mgmtdev pci/0000:65:00.0 mac 02:11:22:33:44:55
vdpa dev add name vdpa:0000:65:00.1 mgmtdev pci/0000:65:00.1 mac 02:11:22:33:44:66
```

### Device plugin configuration

Just for demo purposes this document proposes a device plugin configuration
that selects devices based on the PF name. Write/edit
`/etc/pcidp/config.json` so it looks like:

```json
{
    "resourceList": [{
            "resourceName": "mlx-vdpa",
            "selectors": {
                "pfNames": ["ens1f0np0"]
            }
        }
    ]
}
```

Now the device plugin should advertise the vdpa devices created in the
previous section.

## NAD configuration

ovs-cni relies on the device-info file to know whether the underlying
device being configured for the network is a vdpa device. To make the
CNI device-info aware, the path to the DeviceInfo file must be passed by
multus. To do so, the `CNIDeviceInfoFile` capability must be enabled in
the ovs network.

Assuming that an OVS bridge called `br-vdpa` already exists, a network
attachment definition must be created now, glueing everything together:
```yaml
apiVersion: k8s.cni.cncf.io/v1
kind: NetworkAttachmentDefinition
metadata:
  annotations:
    k8s.v1.cni.cncf.io/resourceName: openshift.io/mlx-vdpa
  name: ovs-vdpa-net
spec:
  config: |-
    {
        "cniVersion": "4.0.0",
        "name": "ovs-vdpa-net",
        "type": "ovs",
        "capabilities": {
            "CNIDeviceInfoFile": true
        },
        "bridge": "br-vdpa",
        "ipam": {}
    }
```

Remember that ipam is not supported for `vhost_vdpa`, as there is not
interface in the host or pod that can be assigned an IP directly. It
must be the guest doing that, which the CNI does not have access to.

Pods attached to `ovs-vdpa-net` will be configured a `vhost_vdpa` device
that is attached to the `br-vdpa` OVS bridge. The ovs-cni will configure
the network device and bridge port attachment. The network resources
injector will handle the resource request injection into the pod spec.
