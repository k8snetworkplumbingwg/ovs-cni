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
    ./cluster/cli.sh ssh ${node} -- sudo systemctl daemon-reload
    ./cluster/cli.sh ssh ${node} -- sudo systemctl enable openvswitch
    ./cluster/cli.sh ssh ${node} -- sudo systemctl restart openvswitch
    ./cluster/cli.sh ssh ${node} -- sudo systemctl restart NetworkManager
done

echo 'Deploying multus'
curl https://raw.githubusercontent.com/k8snetworkplumbingwg/multus-cni/v3.9.1/deployments/multus-daemonset.yml -o cluster/multus-daemonset.yml
MULTUS_IMAGE=ghcr.io/k8snetworkplumbingwg/multus-cni:v3.9
sed -i "s#ghcr.io/k8snetworkplumbingwg/multus-cni:stable\$#$MULTUS_IMAGE#" cluster/multus-daemonset.yml
./cluster/kubectl.sh create -f cluster/multus-daemonset.yml
./cluster/kubectl.sh rollout status daemonset -l name=multus -n kube-system --timeout=300s
