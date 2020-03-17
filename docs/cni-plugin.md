# Open vSwitch CNI Plugin

## Overview

With ovs plugin, containers (on the same host) are plugged into an Open vSwitch
bridge (virtual switch) that resides in the host network namespace. It's host
adminitrator's responsibility to create such bridge and optionally connect it to
broader network, be it using L2 directly, NAT or an overlay. The containers
receive one end of the veth pair and the other end is connected to the bridge.

Please note that Open vSwitch must be installed and running on the host.

## Example Configuration

A simple example with VLAN 1000:

```json
{
    "name": "mynet",
    "type": "ovs",
    "bridge": "mynet0",
    "vlan": 100
}
```

Another example with a trunk port and jumbo frames:

```json
{
    "name": "mytrunknet",
    "type": "ovs",
    "bridge": "mynet1",
    "mtu": 9000,
    "trunk": [ { "id" : 42 }, { "minID" : 1000, "maxID" : 1010 } ]
}
```

Another example with QinQ configured.
Note that the Open vSwitch needs to have configured vlan-limit=2 or
vlan-limit=0. This can be done by issuing the following command:

```shell
ovs-vsctl set Open_vSwitch . other_config:vlan-limit=2
```

```json
{
    "name": "myqinqnet",
    "type": "ovs",
    "bridge": "mynet2",
    "vlan": 1000,
    "vlanMode": "dot1q-tunnel",
    "cvlan": [ { "id" : 24 }, { "minID" : 100, "maxID" : 102 } ]
}
```

## Network Configuration Reference

* `name` (string, required): the name of the network.
* `type` (string, required): "ovs".
* `bridge` (string, required): name of the bridge to use.
* `vlan` (integer, optional): VLAN ID of attached port. If not specified, trunk
  port is used by default. If QinQ is enabled (vlanMode set to dot1q-tunnel), vlan
  represents service VLAN.
* `vlanMode` (string, optional): Vlan mode of atached port. One of access, dot1q-tunnel, or trunk.
  If not specified, access is used by default.
* `mtu` (integer, optional): MTU.
* `trunk` (optional): List of VLAN ID's and/or ranges of accepted VLAN
  ID's.
* `cvlan` (optional): List of customer VLAN ID's for 802.1Q tunneling (QinQ).
  Note that the Open vSwitch needs to have configured vlan-limit=2 or
  vlan-limit=0.

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
