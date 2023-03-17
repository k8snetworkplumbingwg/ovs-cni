# Demo

This demo will show you how to configure Open vSwitch bridges on your nodes,
create Kubernetes networks on top of them and use them by pods.

## Requirements

Before we start, make sure that you have your Kubernetes/OpenShift cluster
ready with OVS. In order to do that, you can follow guides of deployment on
[local cluster](deployment-on-local-cluster.md) or your [arbitrary
cluster](deployment-on-arbitrary-cluster.md).

## Create Open vSwitch bridge

First of all, we need to create OVS bridges on all nodes.

```shell
# on all nodes
ovs-vsctl add-br br1
```

This bridge on its own has no access to other nodes and therefore pods
connected to the same bridge on different nodes will not have connectivity to
one another. If you have just a single node, you can skip the rest of this
section and continue to [Creating network attachment
definition](#create-network-attachment-definition).

### Connect Bridges Using Dedicated L2

The easiest way how to interconnect OVS bridge is using L2 connection on
dedicated interface (not used by node itself, without configured IP address).
If you have spare dedicated interface available, connect it as a port to OVS
bridge.

```shell
# on all nodes
ovs-vsctl add-port br1 eth1
```

### Connect Bridges Using Shared L2

You can also share node management interface with OVS bridge. Before you do
that, please keep in mind, that you might lose your connectivity for a moment
as we will move IP address from the interface to the bridge. I would recommend
you doing this using console.

This process depends on your platform, following command is only an
illustrative example and it might break your system.

```shell
# on all nodes
## get original interface IP address
ip address show eth0
## flush original IP address
ip address flush eth0
## attach the interface to OVS bridge
ovs-vsctl add-port br1 eth0
## set the original IP address on the bridge
ip address add $ORIGINAL_INTERFACE_IP_ADDRESS dev br0
```

### Connect Bridges Using VXLAN

Lastly, we can connect OVS bridges using VXLAN tunnels. In order to do so,
create VXLAN port on OVS bridge per each remote node with `options:remote_ip`
set to the remote node IP address.

```shell
# on node01
ovs-vsctl add-port br1 vxlan -- set Interface vxlan type=vxlan options:remote_ip=$NODE02_IP_ADDRESS

# on node02
ovs-vsctl add-port br1 vxlan -- set Interface vxlan type=vxlan options:remote_ip=$NODE01_IP_ADDRESS
```

## Create Network Attachment Definition

With Multus, secondary networks are specified using
`NetworkAttachmentDefinition` objects. Such objects contain information that is
passed to ovs-cni plugin on nodes during pod configuration. Such networks can
be referenced from pod using their names. If this object, we specify name of
the bridge and optionaly a VLAN tag.

Also note `resourceName` annotation. It is used to make sure that pod will
be scheduled on a node with bridge `br1` available.

First, let's create a simple OVS network that will connect pods in trunk mode.

```shell
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-net-1
  annotations:
    k8s.v1.cni.cncf.io/resourceName: ovs-cni.network.kubevirt.io/br1
spec:
  config: '{
      "cniVersion": "0.4.0",
      "type": "ovs",
      "bridge": "br1"
    }'
EOF
```

Now create another network connected to the same bridge, this time with VLAN
tag specified.

```shell
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-net-2-vlan
  annotations:
    k8s.v1.cni.cncf.io/resourceName: ovs-cni.network.kubevirt.io/br1
spec:
  config: '{
      "cniVersion": "0.4.0",
      "type": "ovs",
      "bridge": "br1",
      "vlan": 100
    }'
EOF
```

## Attach a Pod to the network

Now when everything is ready, we can create a Pod and connect it to OVS
networks.

```shell
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod-1
  annotations:
    k8s.v1.cni.cncf.io/networks: ovs-net-1,ovs-net-2-vlan
spec:
  containers:
  - name: samplepod
    command: ["sleep", "99999"]
    image: alpine
EOF
```

We can login into the Pod to verify that secondary interfaces were created.

```shell
kubectl exec samplepod-1 -- ip link show
```

## Configure IP Address

Open vSwitch CNI has support for IPAM. In order to test IP connectivity
between Pods, we first need to create `NetworkAttachmentDefinition` object
with required IPAM configuration.

```shell
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-ipam-net
  annotations:
    k8s.v1.cni.cncf.io/resourceName: ovs-cni.network.kubevirt.io/br1
spec:
  config: '{
      "cniVersion": "0.4.0",
      "type": "ovs",
      "bridge": "br1",
      "vlan": 100,
      "ipam": {
        "type": "static"
      },
      "mtu": 1450
    }'
EOF
```

The `ovs-ipam-net` nework uses `static` IPAM plugin but without any configured IP addresses.
Hence pod spec have to specify a static IP address through `runtimeConfig` parameter.

Now create a pod and connect to `ovs-ipam-net` network with `10.10.10.1` ip address.

```shell
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod-2
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
        {
          "name": "ovs-ipam-net",
          "ips": ["10.10.10.1/24"]
        }
]'
spec:
  containers:
  - name: samplepod
    command: ["sleep", "99999"]
    image: alpine
EOF
```

Create another Pod connected to same `ovs-ipam-net` network with `10.10.10.2` ip address,
so we have something to ping.

```shell
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod-3
  annotations:
    k8s.v1.cni.cncf.io/networks: '[
        {
          "name": "ovs-ipam-net",
          "ips": ["10.10.10.2/24"]
        }
]'
spec:
  containers:
  - name: samplepod
    command: ["sleep", "99999"]
    image: alpine
EOF
```

Once both Pods are up and running, we can try to ping from one to another.

```shell
kubectl exec -it samplepod-2 -- ping 10.10.10.2
```
