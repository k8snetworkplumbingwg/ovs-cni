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
* `bridge` (string, optional): name of the bridge to use, can be omitted if `ovnPort` is set in CNI_ARGS, or if `deviceID` is set
* `deviceID` (string, optional): PCI address of a Virtual Function in valid sysfs format to use in HW offloading mode. This value is usually set by Multus.
* `vlan` (integer, optional): VLAN ID of attached port. Trunk port if not
   specified.
* `mtu` (integer, optional): MTU.
* `trunk` (optional): List of VLAN ID's and/or ranges of accepted VLAN
  ID's.
* `ofport_request` (integer, optional): request a static OpenFlow port number in range 1 to 65,279
* `interface_type` (string, optional): type of the interface belongs to ports. if value is "", ovs will use default interface of type 'internal'
* `configuration_path` (optional): configuration file containing ovsdb
  socket file path, etc.

The following are *per-invocation* arguments rather than static network
configuration. They are not set in the `NetworkAttachmentDefinition` `config`
body, but passed per pod through CNI args (see
[Per-Pod Arguments](#per-pod-arguments-mac--ovnport)):

* `mac` (string, optional): MAC address to assign to the container interface.
* `ovnPort` (string, optional): value written to the OVS interface's
  `external_ids:iface-id`, used to bind the port to an OVN logical switch port.
  When set, `bridge` may be omitted (it is derived from the port).


_*Note:* if `deviceID` is provided, then it is possible to omit `bridge` argument. Bridge will be automatically selected by the CNI plugin by following
the chain: Virtual Function PCI address (provided in `deviceID` argument) > Physical Function > Bond interface 
(optional, if Physical Function is part of a bond interface) > ovs bridge_

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

## Per-Pod Arguments (`mac` / `ovnPort`)

Some parameters are specific to a single pod attachment rather than to the
network as a whole, and therefore cannot live in the shared
`NetworkAttachmentDefinition` `config`. ovs-cni accepts two such per-invocation
arguments:

* `mac` — MAC address for the container interface.
* `ovnPort` — value stored in the OVS interface's `external_ids:iface-id`,
  which binds the port to a pre-created OVN logical switch port.

They can be supplied through either of the two standard CNI mechanisms:

### 1. `CNI_ARGS` environment variable

When invoking the plugin directly, pass them as `CNI_ARGS`:

```shell
CNI_ARGS="mac=0a:58:0a:f4:00:07;ovnPort=my-logical-port" ...
```

### 2. Multus network annotation (`cni-args`)

When Multus is in use, `CNI_ARGS` is not available per network. Instead, attach
the parameters to the individual network selection using the `cni-args` field of
the `k8s.v1.cni.cncf.io/networks` annotation. Multus forwards `cni-args` to the
delegate plugin inside the CNI config's `args.cni` object, and ovs-cni reads
`mac` / `ovnPort` from there:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: |
      [
        {
          "name": "ovs-conf",
          "cni-args": {
            "mac": "0a:58:0a:f4:00:07",
            "ovnPort": "my-logical-port"
          }
        }
      ]
spec:
  containers:
  - name: samplepod
    command: ["/bin/sh", "-c", "sleep 99999"]
    image: alpine
```

This is the path that lets a per-pod `ovnPort` (for example, one allocated by an
external OVN/Neutron controller and written onto the pod) actually reach the OVS
port, which is not possible through `CNI_ARGS` alone under Multus.

Key matching for these arguments is **case-insensitive** (`ovnPort`, `ovnport`
and `OvnPort` are all accepted), so a casing difference in a hand-written
annotation does not silently drop the value. Entries under
`args.cni` that are not JSON strings, or whose keys ovs-cni does not recognize,
are ignored, so unrelated `cni-args` used by other plugins do not interfere.

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
make cni-tests
```
