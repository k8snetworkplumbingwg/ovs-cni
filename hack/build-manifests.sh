#!/usr/bin/env bash
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

set -e

export NAMESPACE=${NAMESPACE:-kube-system}

export OVS_CNI_PLUGIN_IMAGE_REPO=${OVS_CNI_PLUGIN_IMAGE_REPO:-quay.io/kubevirt}
export OVS_CNI_PLUGIN_IMAGE_NAME=${OVS_CNI_PLUGIN_IMAGE_NAME:-ovs-cni-plugin}
export OVS_CNI_PLUGIN_IMAGE_VERSION=${OVS_CNI_PLUGIN_IMAGE_VERSION:-latest}
export OVS_CNI_PLUGIN_IMAGE_PULL_POLICY=${OVS_CNI_PLUGIN_IMAGE_PULL_POLICY:-IfNotPresent}
export CNI_MOUNT_PATH=${CNI_MOUNT_PATH:-/opt/cni/bin}
export OVS_CNI_MARKER_HEALTHCHECK_INTERVAL=${OVS_CNI_MARKER_HEALTHCHECK_INTERVAL:-60}

for template in manifests/*.in; do
    name=$(basename ${template%.in})
    envsubst < ${template} > examples/${name}
done
