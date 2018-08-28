# Scheduling

In some cases, you want to configure Open vSwitch bridges only on certain
nodes, in that case, we need to make sure that pods requesting such
bridges/networks will be scheduled only on such nodes. This should be
eventually solved automatically by ovs-cni scheduler, until then it is possible
to use Kubernetes node labeling as a workaround.

## Label nodes

First, let's say that we created following network attachment definition.

```yaml
cat <<EOF | kubectl create -f -
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: ovs-net
spec:
  config: '{
      "cniVersion": "0.3.1",
      "type": "ovs",
      "bridge": "br1"
    }'
EOF
```

This network requires OVS bridge `br1`. We know that this bridge is available
only on node `node01`, so we label the node accordingly.

```shell
kubectl label nodes node01 ovs-cni/ovs-net=true
```

## Create pods

Then when we want to use this network from a pod, we don't only list it as a
requested network, we also add a node selector for nodes that have it
available.


```shell
cat <<EOF | kubectl create -f -
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
  nodeSelector:
    ovs-cni/ovs-net: "true"
EOF
```
