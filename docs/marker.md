# OVS-CNI Resource Marker

## Overview

OVS-CNI resource marker is a Kubernetes daemon set which exposes available
Open vSwitch nodes as node resources.

For instance, when an OVS bridge `br10` is created on a node with following
command:

```shell
ovs-vsctl add-br br10
```

Marker will pick it up and update node state as following:

```yaml
...
status:
  allocatable:
    ovs-cni.network.kubevirt.io/br10: 1k
    ...
  capacity:
    ovs-cni.network.kubevirt.io/br10: 1k
    ...
  ...
...
```
