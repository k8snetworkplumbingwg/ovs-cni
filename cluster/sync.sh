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

REGISTRY=${REGISTRY:-ghcr.io/k8snetworkplumbingwg}
IMAGE_TAG=${IMAGE_TAG:-latest}

make docker-build

if [ "${OCI_BIN}" = "podman" ]; then
    ${OCI_BIN} save ${REGISTRY}/ovs-cni-plugin:${IMAGE_TAG} -o /tmp/ovs-cni-plugin.tar
    kind load image-archive --name ${KIND_CLUSTER_NAME} /tmp/ovs-cni-plugin.tar
    rm -f /tmp/ovs-cni-plugin.tar
else
    kind load docker-image --name ${KIND_CLUSTER_NAME} ${REGISTRY}/ovs-cni-plugin:${IMAGE_TAG}
fi

./cluster/kubectl.sh delete --ignore-not-found -f examples/ovs-cni.yml
./cluster/kubectl.sh apply -f examples/ovs-cni.yml
