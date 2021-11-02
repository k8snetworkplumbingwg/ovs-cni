# OVS Hardware Offload

This provides an option to enable OVS hardware offload with ovs-cni while using OVS data-plane
in Mellanox ConnectX-4 Lx onwards NIC hardware (Mellanox Embedded Switch or eSwitch).
The OVS hardware offload is using SR-IOV technology with VF representor host net-device.
The VF representor plays the same role as TAP devices in Para-Virtual (PV) setup.
A packet sent through the VF representor on the host arrives to the VF, and a packet sent
through the VF is received by its representor.

## Prerequisites

- Mellanox ConnectX-5 NIC
- [sriov-network-device-plugin](https://github.com/intel/sriov-network-device-plugin)
- [multus-cni](https://github.com/intel/multus-cni)
- [network-resources-injector](https://github.com/intel/network-resources-injector)

## Mellanox SR-IOV Configuation

In order to enable Open vSwitch hardware offloading, the following steps
are required. Please make sure you have root privileges to run the commands
below.

Check the Number of VF Supported on the NIC

```
cat /sys/class/net/snic0l/device/sriov_totalvfs
64
cat /sys/class/net/snic0r/device/sriov_totalvfs
64
```

Create the VFs after a reboot

```
echo 8 > /sys/class/net/snic0l/device/sriov_numvfs
echo 8 > /sys/class/net/snic0r/device/sriov_numvfs
```

Unbind the VF kernel drivers

```
dpdk-devbind -u 0000:18:00.2 0000:18:00.3 0000:18:00.4 0000:18:00.5 0000:18:00.6 0000:18:00.7 0000:18:01.0 0000:18:01.1
dpdk-devbind -u 0000:18:08.2 0000:18:08.3 0000:18:08.4 0000:18:08.5 0000:18:08.6 0000:18:08.7 0000:18:09.0 0000:18:09.1
```

Change the PFs to switchdev mode

```
echo switchdev > /sys/class/net/snic0l/compat/devlink/mode
echo switchdev > /sys/class/net/snic0r/compat/devlink/mode
```

Check that this created 16 representor devices

```
ip l | egrep -c "enp24s0f[01]_[0-7]"
16
```

Bind the VF kernel drivers again

```
dpdk-devbind -b mlx5_core 0000:18:00.2 0000:18:00.3 0000:18:00.4 0000:18:00.5 0000:18:00.6 0000:18:00.7 0000:18:01.0 0000:18:01.1
dpdk-devbind -b mlx5_core 0000:18:08.2 0000:18:08.3 0000:18:08.4 0000:18:08.5 0000:18:08.6 0000:18:08.7 0000:18:09.0 0000:18:09.1
```

Assign the representor MAC addresses to the VF MAC adresses

```
for i in `seq 0 7`; do mac=`ip l show smartvf0_$i | grep -o "link/ether [^ ]*" | cut -d' ' -f2`; echo "vf $i: $mac"; ip l set snic0l vf $i mac $mac; done
for i in `seq 0 7`; do j=$((i + 8)); mac=`ip l show smartvf0_$j | grep -o "link/ether [^ ]*" | cut -d' ' -f2`; echo "vf $i: $mac"; ip l set snic0r vf $i mac $mac; done
```

OVS bridge onfiguration

```
ovs-vsctl set Open_vSwitch . other_config:hw-offload=true
systemctl restart openvswitch
ovs-vsctl add-br br-snic0
ovs-vsctl add-port br-snic0 bond_snic0
```

For more information [refer Mellanox doc](https://www.mellanox.com/related-docs/prod_software/ASAP2_Hardware_Offloading_for_vSwitches_User_Manual_v4.4.pdf)

## Device Plugin configuation

The device plugin would create the resouce pools based on the configurations given in the `/etc/pcidp/config.json`.
This configuration file is in json format as shown below:

```json
{
    "resourceList": [{
            "resourceName": "mellanox_snic0",
            "selectors": {
                "vendors": ["15b3"],
                "devices": ["1018"],
                "drivers": ["mlx5_core"],
                "pfNames": ["snic0l","snic0r"]
            }
        }
    ]
}
```

Deploy SR-IOV network device plugin as daemonset see [device plugin](https://github.com/intel/sriov-network-device-plugin)

## Network and POD configuation

After deploying `multus`, `ovs-cni` and `network-resources-injector`, Create a NetworkAttachementDefinition CRD object
with the following config.

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-trunk-net
  annotations:
    k8s.v1.cni.cncf.io/resourceName: intel.com/mellanox_snic0
spec:
  config: '{
      "cniVersion": "0.4.0",
      "type": "ovs",
      "bridge": "br-snic0",
      "trunk": [ {"minID": 1050, "maxID": 1059} ]
    }'
```

Now deploy a pod with the following config to attach VF into container and its representor net device
attached with ovs bridge `br-snic0`.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: ovs-offload-pod
  annotations:
    k8s.v1.cni.cncf.io/networks: ovs-trunk-net
spec:
  containers:
  - name: ovs-offload-container
    command: ["/bin/bash", "-c"]
    args:
    - |
      while true; do sleep 1000; done
    image: registry.suse.com/suse/sle15:15.1
```

The `network-resources-injector` webhook parses `k8s.v1.cni.cncf.io/networks` annotation to retrieve appropriate
resource name from its NetworkAttachementDefinition CRD object and then included in requests/limits of pod specification.

The device plugin allocates the requested device into pod container and ```multus``` cni plugin forwards the device's
pci address into ```ovs-cni``` through ```deviceID``` parameter.
