# Deployment on Arbitrary Cluster

In this guide we will cover installation of Open vSwitch (OVS), Multus and
ovs-cni on your arbitrary cluster.

# Requirements

## A Cluster

This guide requires you to have your own Kubernetes/OpenShift cluster. If you
don't have one and just want to try ovs-cni out, please refer to the [deployment
on local cluster](deployment-on-local-cluster.md) guide.

## Open vSwitch

### Kubernetes

Open vSwitch must be installed and running on all nodes (or on a subset of nodes
if you do [scheduling](scheduling.md)). OVS is available in repositories of all
major distributions.

On CentOS:

```shell
yum install openvswitch
systemctl start openvswitch
```

On Fedora:

```shell
dnf install openvswitch
systemctl start openvswitch
```

If you use other platform, please check [Open vSwitch
documentation](https://github.com/openvswitch/ovs).

You can verify that OVS is properly running.

```shell
ovs-vsctl add-br test-br
ovs-vsctl list-br
ovs-vsctl del-br test-br
```

### OpenShift

For OpenShift, Open vSwitch is already installed on the cluster. Please note
that this Open vSwitch instance is meant only for OpenShiftSDN and using it
for ovs-cni may lead to unexpected behavior.

# Open vSwitch CNI plugin

Finally, we can install ovs-cni on our cluster. In order to do that,
please use [Cluster Network Addons Operator Project](https://github.com/kubevirt/cluster-network-addons-operator).
