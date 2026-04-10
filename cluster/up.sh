#!/bin/bash
#
# Copyright 2018-2019 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -ex

SCRIPTS_PATH="$(dirname "$(realpath "$0")")"
source ${SCRIPTS_PATH}/cluster.sh

echo 'Building custom kind node image with OVS'
${OCI_BIN} build -t ${KIND_NODE_IMAGE} ${SCRIPTS_PATH}/kind-node/

echo 'Creating kind cluster'
mkdir -p "$(dirname "$(cluster::kubeconfig)")"
kind create cluster \
    --name ${KIND_CLUSTER_NAME} \
    --image ${KIND_NODE_IMAGE} \
    --config ${SCRIPTS_PATH}/kind-config.yaml \
    --kubeconfig "$(cluster::kubeconfig)"

echo 'Waiting for nodes to be ready'
./cluster/kubectl.sh wait --for=condition=Ready nodes --all --timeout=300s

echo 'Starting Open vSwitch on nodes'
for n in $(./cluster/kubectl.sh get nodes --no-headers -o custom-columns=NAME:.metadata.name); do
    ${OCI_BIN} exec "${n}" systemctl start openvswitch-switch
done

echo 'Deploying Multus'
MULTUS_VERSION=v4.0.1
MULTUS_MANIFEST=https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/${MULTUS_VERSION}/deployments/multus-daemonset.yml
# update the tag until https://github.com/k8snetworkplumbingwg/multus-cni/issues/1170 is fixed
curl -L ${MULTUS_MANIFEST} | sed "s/:snapshot/:${MULTUS_VERSION}/g" | ./cluster/kubectl.sh apply -f -
./cluster/kubectl.sh -n kube-system rollout status daemonset kube-multus-ds --timeout 300s
