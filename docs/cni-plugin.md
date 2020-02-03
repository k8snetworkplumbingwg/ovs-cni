# Open vSwitch CNI Plugin

## Overview

With ovs plugin, containers (on the same host) are plugged into an Open vSwitch
bridge (virtual switch) that resides in the host network namespace. It's host
adminitrator's responsibility to create such bridge and optionally connect it to
broader network, be it using L2 directly, NAT or an overlay. The containers
receive one end of the veth pair and the other end is connected to the bridge.

Please note that Open vSwitch must be installed and running on the host.

## Example Configuration

```json
{
    "name": "mynet",
    "type": "ovs",
    "bridge": "mynet0",
    "vlan": 100
}
```

## Network Configuration Reference

* `name` (string, required): the name of the network.
* `type` (string, required): "ovs".
* `bridge` (string, required): name of the bridge to use.
* `vlan` (integer, optional): VLAN ID of attached port. Trunk port if not
   specified.

## Manual Testing

```shell
# Build the binary
make build-plugin

# Create a new namespace
ip netns add ns1

# Create OVS bridge on the host
ovs-vsctl add-br br1

# Run ADD command connecting the namespace to the bridge
cat <<EOF | CNI_COMMAND=ADD CNI_CONTAINERID=ns1 CNI_NETNS=/var/run/netns/ns1 CNI_IFNAME=eth2 CNI_PATH=`pwd` ./cmd/plugin/plugin
{
    "cniVersion": "0.3.1",
    "name": "mynet",
    "type": "ovs",
    "bridge": "br1",
    "vlan": 100
}
EOF

# Check that a veth pair was connected inside the namespace
ip netns exec ns1 ip link

# Check that the other side of veth pair is connected as a port on the bridge and with requested VLAN tag
ovs-vsctl show

# Run DEL command removing the veth pair and OVS port
cat <<EOF | CNI_COMMAND=DEL CNI_CONTAINERID=ns1 CNI_NETNS=/var/run/netns/ns1 CNI_IFNAME=eth2 CNI_PATH=/opt/cni/bin ./cmd/plugin/plugin
{
    "cniVersion": "0.3.1",
    "name": "mynet",
    "type": "ovs",
    "bridge": "br1",
    "vlan": 100
}
EOF

# Check that veth pair was removed from the namespace
ip netns exec ns1 ip link

# Check that the port was removed from the OVS bridge
ovs-vsctl show

# Delete OVS bridge
ovs-vsctl del-br br1

# Delete the namespace
ip netns del ns1
```

## Go Tests

This plugin also have Go test coverage. To run tests, Open vSwitch must be
installed and its service running. Since those tests configure host networking,
they must be executed by root.
This also needs `host-local` ipam plugin to be present in one of the `PATH` directory.

```shell
sudo --preserve-env make test-pkg-plugin
```

## OVS Hardware Offload

This provides an option to enable OVS hardware offload with ovs-cni while using OVS data-plane
in Mellanox ConnectX-4 Lx onwards NIC hardware (Mellanox Embedded Switch or eSwitch).
The OVS hardware offload is using SR-IOV technology with VF representor host net-device.
The VF representor plays the same role as TAP devices in Para-Virtual (PV) setup.
A packet sent through the VF representor on the host arrives to the VF, and a packet sent
through the VF is received by its representor.

### Prerequisites

[sriov-network-device-plugin](https://github.com/intel/sriov-network-device-plugin)
[multus-cni](https://github.com/intel/multus-cni)
[network-resources-injector](https://github.com/intel/network-resources-injector)

### Mellanox SR-IOV Configuation

To enable OVS hardware offloading, create VFs from a Mellanox PF, configure VFs in `switchdev` mode and
set `hw-offload=true` for Open vSwitch.
For more information [refer Mellanox doc] (https://www.mellanox.com/related-docs/prod_software/ASAP2_Hardware_Offloading_for_vSwitches_User_Manual_v4.4.pdf)

### Device Plugin configuation

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

Deploy SR-IOV network device plugin as daemonset see https://github.com/intel/sriov-network-device-plugin

### Network and POD configuation

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
      "cniVersion": "0.3.1",
      "type": "ovs",
      "bridge": "br-snic0",
      "trunk": "1050-1059"
    }'
```

Now deploy a pod with the following config to attach VF into container and its representor
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
