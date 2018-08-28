# Deployment on Local Cluster

This project allows you to spin up virtualized Kubernets/OpenShift cluster. In
this guide we will create a local Kubernetes cluster with two nodes,
preinstalled Multus and Open vSwitch, and then install ovs-cni from local
sources.

If you want to deploy ovs-cni on your arbitrary cluster, go to [deployment on
arbitrary cluster guide](deployment-on-arbitrary-cluster.md).

Start local cluster. If you want to use OpenShift instead of Kubernetes or
different amount of nodes, check [development
guide](devel-guide.md#local-cluster).

```shell
KUBEVIRT_NUM_NODES=2 make cluster-up
```

Build ovs-cni from local sources and install it on the cluster.

```shell
make cluster-sync
```

You can ssh into created nodes using `cluster/cli.sh`.

```shell
cluster/cli.sh ssh node01
```

Finally if you want to use `kubectl` to access the cluster, start proxy.

```shell
./cluster/kubectl.sh proxy --port=8080 --disable-filter=true &
```

You can stop here and play with the cluster on your own or continue to
[demo](demo.md) that will guide you through definition of kubernetes networks,
configuration of Open vSwitch bridges and attachment of pods to them.
