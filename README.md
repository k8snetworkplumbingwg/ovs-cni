# Open vSwitch CNI plugin

This plugin allows user to define Kubernetes networks on top of Open vSwitch bridges available on nodes. Note that ovs-cni does not configure bridges, it's up to a user to create them and connect them to L2, L3 or an overlay network. This project also delivers OVS marker, which exposes available bridges as Node resources, that can be used to schedule pods on the right node via [intel/network-resources-injector](https://github.com/intel/network-resources-injector/). Finally please note that Open vSwitch must be installed and running on the host.

In order to use this plugin, Multus must be installed on all hosts and `NetworkAttachmentDefinition` CRD created.

## Overview

First create network attachment definition. This object specifies to which Open vSwitch bridge should the pod be attached and what VLAN ID should be set on the port. For more information, check [plugin documentation](docs/cni-plugin.md).

```shell
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-conf
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

Once the network definition is created and desired Open vSwitch bridges are available on nodes, a pod requesting the network can be created.

```shell
cat <<EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: ovs-conf
spec:
  containers:
  - name: samplepod
    command: ["/bin/sh", "-c", "sleep 99999"]
    image: alpine
    resources:  # this may be omitted if intel/network-resources-injector is present on the cluster
      limits:
        ovs-cni.network.kubevirt.io/br1: 1
EOF
```

Such pod should contain default `eth0` interface connected to default Kubernetes network and also `net1` connected to the bridge.

```shell
$ kubectl exec samplepod ip link
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
3: eth0@if11: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1450 qdisc noqueue state UP
    link/ether 0a:58:0a:f4:00:07 brd ff:ff:ff:ff:ff:ff
5: net1@if12: <BROADCAST,MULTICAST,UP,LOWER_UP,M-DOWN> mtu 1500 qdisc noqueue state UP
    link/ether e6:f4:2e:b4:4b:6e brd ff:ff:ff:ff:ff:ff
```

## Deployment and Usage

You can choose to deploy this plugin on [local virtualized cluster](docs/deployment-on-local-cluster.md) or on your [arbitrary cluster](docs/deployment-on-arbitrary-cluster.md). After that you can follow [demo](docs/demo.md) that will guide you through preparation of Open vSwitch bridges, defining networks on Kubernetes and attaching pods to them.

## Components

 * [CNI Plugin](docs/cni-plugin.md) - Documentation of standalone Open vSwitch CNI plugin.
 * [Port Mirroring](docs/traffic-mirroring.md) - Documentation of an OVS CNI extension, allowing for port mirroring.
 * [Hardware Offload](docs/ovs-offload.md) - Documentation of hardware offload functionality, using SR-IOV.
 * [Marker](docs/marker.md) - Documentation of daemon set exposing bridges as node resources.

## Development

[Development guide](docs/devel-guide.md) is a go-to reference point for development helper commands, building, testing, container images and local cluster.
