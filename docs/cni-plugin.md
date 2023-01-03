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

Another example with a port which has an interface of type system:

```json
{
   "name": "overlaynet",
   "type": "ovs",
   "bridge": "mynet1",
   "interface_type": "system"
}
```

## Network Configuration Reference

* `name` (string, required): the name of the network.
* `type` (string, required): "ovs".
* `bridge` (string, required): name of the bridge to use.
* `vlan` (integer, optional): VLAN ID of attached port. Trunk port if not
   specified.
* `mtu` (integer, optional): MTU.
* `trunk` (optional): List of VLAN ID's and/or ranges of accepted VLAN
  ID's.
* `ofport_request` (integer, optional): request a static OpenFlow port number in range 1 to 65,279
* `interface_type` (string, optional): type of the interface belongs to ports. if value is "", ovs will use default interface of type 'internal'
* `configuration_path` (optional): configuration file containing ovsdb
  socket file path, etc.

### Flatfile Configuation

There is one option for flat file configuration:

* `configuration_path`: A file path to a OVS CNI configuration file.

OVS CNI will look for the configuration in these locations, in this order:

* The location specified by the `configuration_path` option.
* `/etc/kubernetes/cni/net.d/ovs.d/ovs.conf`
* `/etc/cni/net.d/ovs.d/ovs.conf`

You may specify the `configuration_path` to point to another location should it be desired.

Any options added to the `ovs.conf` are overridden by configuration options that are in the
CNI configuration (e.g. in a custom resource `NetworkAttachmentDefinition` used by Multus CNI
or in the first file ASCII-betically in the CNI configuration directory -- which is
`/etc/cni/net.d/` by default).

The sample content of ovs.conf (in JSON format) is as follows:

```json
{
  "socket_file": "unix:/usr/local/var/run/openvswitch/db.sock",
  "link_state_check_retries": 5,
  "link_state_check_interval": 1000
}
```

The `socket_file` consist of socket type and socket detail like these.

* `unix:<path to unix domain socket>`
* `tcp:<ip address>:<port number>`
* `ssl:<ip address>:<port number>`

If no socket type is specified, it is assumed to be a unix domain socket, for backwards compatibility.

The `link_state_check_interval` is in milliseconds.

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
    "cniVersion": "0.4.0",
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
    "cniVersion": "0.4.0",
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

```shell
sudo --preserve-env make test-pkg-plugin
```
