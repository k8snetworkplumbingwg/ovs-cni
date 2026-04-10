# Deployment on Local Cluster

This project allows you to spin up a local Kubernetes cluster using kind. In
this guide we will create a local Kubernetes cluster with a control-plane and
a worker node, preinstalled Multus and Open vSwitch, and then install ovs-cni
from local sources.

If you want to deploy ovs-cni on your arbitrary cluster, go to [deployment on
arbitrary cluster guide](deployment-on-arbitrary-cluster.md).

Start local cluster.

```shell
make cluster-up
```

Build ovs-cni from local sources and install it on the cluster.

```shell
make cluster-sync
```

You can run commands on cluster nodes using `cluster/exec.sh`.

```shell
cluster/exec.sh ovs-cni-worker -- ovs-vsctl show
```

Finally if you want to use `kubectl` to access the cluster, start proxy.

```shell
./cluster/kubectl.sh proxy --port=8080 --disable-filter=true &
```

You can stop here and play with the cluster on your own or continue to
[demo](demo.md) that will guide you through definition of kubernetes networks,
configuration of Open vSwitch bridges and attachment of pods to them.
