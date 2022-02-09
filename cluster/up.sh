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

source ./cluster/cluster.sh
cluster::install

$(cluster::path)/cluster-up/up.sh

echo 'Installing Open vSwitch on nodes'
for node in $(./cluster/kubectl.sh get nodes --no-headers | awk '{print $1}'); do
    ./cluster/cli.sh ssh ${node} -- sudo dnf install -y centos-release-nfv-openvswitch
    ./cluster/cli.sh ssh ${node} -- sudo dnf install -y openvswitch2.16 dpdk
    ./cluster/cli.sh ssh ${node} -- sudo systemctl daemon-reload
    ./cluster/cli.sh ssh ${node} -- sudo systemctl restart openvswitch
done

echo 'Deploying multus'
MULTUS_IMAGE=quay.io/kubevirt/cluster-network-addon-multus@sha256:b7487e14aa0e4f4d0b8f6a626af7d420b4cd0d8bda2fda1eb652c310526db1f8
cp cluster/multus-daemonset.do-not-change.yml cluster/multus-daemonset.yml
sed -i "s#ghcr.io/k8snetworkplumbingwg/multus-cni:stable\$#$MULTUS_IMAGE#" cluster/multus-daemonset.yml
./cluster/kubectl.sh create -f cluster/multus-daemonset.yml
./cluster/kubectl.sh -n kube-system wait --for=condition=ready -l name=multus pod --timeout=300s
