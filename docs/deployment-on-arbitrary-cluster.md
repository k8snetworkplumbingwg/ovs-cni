# Deployment on Arbitrary Cluster

In this guide we will cover installation of Open vSwitch, Multus and ovs-cni on
your arbitrary cluster.

## Requirements

This guide requires you to have your own Kubernetes/OpenShift cluster. If you
don't have one and just want to try ovs-cni out, please refer to [deployment on
local cluster](deployment-on-local-cluster.md) guide.

## Open vSwitch

First of all, Open vSwitch must be installed and running on all nodes (unless
you do [scheduling](scheduling.md)). OVS is available in repositories of all
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

If you use other platform, please check [Open vSwitch documentation](https://github.com/openvswitch/ovs).

You can verify that OVS is properly running.

```shell
ovs-vsctl add-br test-br
ovs-vsctl list-br
ovs-vsctl del-br test-br
```

## Multus

Installation of Multus is currently a little tricky. If you have not installed
any network plugin (Calico, Flannel,...) on your cluster, you can follow [quick
start guide](https://github.com/intel/multus-cni#quickstart-guide) of Multus.
However, if you already have a network plugin running, you need to do some
extra work. I would recommend you to study the quickstart guide and modify it
for your own case. Multus should soon include better documentation of the
installation process.

## Open vSwitch CNI plugin

Installation of ovs-cni can be done simply by deploying a daemon set that will
deploy ovs-cni binaries on your nodes. Following command will install the
latest image from this repository.

```shell
kubectl apply -f https://raw.githubusercontent.com/kubevirt/ovs-cni/master/examples/ovs-cni.yml
```

You can stop here and play with the cluster on your own or continue to
[demo](demo.md) that will guide you through definition of kubernetes networks,
configuration of Open vSwitch bridges and attachment of pods to them.
