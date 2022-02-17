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

function check_deleted() {
    OUTPUT=$(./cluster/kubectl.sh get --ignore-not-found $1)
    if [ $? -eq 0 ]; then
        echo $(echo "$OUTPUT" | wc -l)
    else
        echo 100
    fi
}

registry_port=$(./cluster/cli.sh ports registry | tr -d '\r')
registry=localhost:$registry_port

REGISTRY=$registry make docker-build
REGISTRY=$registry make docker-push

ovs_cni_manifest="./examples/ovs-cni.yml"

sed 's/quay.io\/kubevirt/registry:5000/g' examples/ovs-cni.yml | ./cluster/kubectl.sh delete --ignore-not-found -f -

# Delete daemon sets that were deprecated/renamed
./cluster/kubectl.sh -n kube-system delete --ignore-not-found ds ovs-cni-plugin-amd64
./cluster/kubectl.sh -n kube-system delete --ignore-not-found ds ovs-vsctl-amd64
for i in $(seq 1 ${KUBEVIRT_NUM_NODES}); do
    ./cluster/cli.sh ssh "node$(printf "%02d" ${i})" -- rm -rf /opt/cni/bin/ovs-cni
done

# Wait until all objects are deleted
until [ $(check_deleted "-f $ovs_cni_manifest") -eq 1 ]; do sleep 1; done
until [ $(check_deleted "ds ovs-cni-plugin-amd64") -eq 1 ]; do sleep 1; done
until [ $(check_deleted "ds ovs-vsctl-amd64") -eq 1 ]; do sleep 1; done

sed 's/quay.io\/kubevirt/registry:5000/g' examples/ovs-cni.yml | ./cluster/kubectl.sh apply -f -
