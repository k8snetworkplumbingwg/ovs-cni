# Usage Example on Local Cluster

In this demo we will start local Kubernetes cluster with preinstalled Multus,
build and install OVS CNI plugin, create an OVS bridge on local cluster,
create NetworkAttachmentDefinition using the plugin and finally attach a Pod to
the bridge.

Start local Kubernetes cluster. This cluster has Open vSwitch installed and
running.

```shell
make cluster-up
```

Build OVS plugin, push its installator image to local cluster and install it.

```shell
make cluster-sync
```

Create OVS bridge on cluster Node, this bridge will be later used for
attachment.

```shell
./cluster/cli.sh sudo ovs-vsctl add-br br1
./cluster/cli.sh sudo ip link set br1 up
```

Wait until OVS Pods are Running.

```shell
./cluster/kubectl.sh get pods --namespace kube-system
```

Create new NetworkAttachmentDefiniton describing a network on top of br1 with
VLAN tag 100.

```shell
cat <<EOF | ./cluster/kubectl.sh create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-net
spec:
  config: '{
      "cniVersion": "0.3.1",
      "type": "ovs",
      "bridge": "br1",
      "vlan": 100
    }'
EOF
```

Create a Pod attached to the network.

```shell
cat <<EOF | ./cluster/kubectl.sh create -f -
apiVersion: v1
kind: Pod
metadata:
  name: samplepod
  annotations:
    k8s.v1.cni.cncf.io/networks: ovs-net
spec:
  containers:
  - name: samplepod
    command: ["sleep", "99999"]
    image: alpine
EOF
```

Verify that secondary interface `net1` was created in the Pod.

```shell
./cluster/kubectl.sh exec -it samplepod -- ip a
```

Verify that a tagged port was created on the bridge.

```shell
./cluster/cli.sh sudo ovs-vsctl show
```
