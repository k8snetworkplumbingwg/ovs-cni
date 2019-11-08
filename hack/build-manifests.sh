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

namespace=${NAMESPACE:-kube-system}

ovs_cni_plugin_image_repo=${OVS_CNI_PLUGIN_IMAGE_REPO:-quay.io/kubevirt}
ovs_cni_plugin_image_name=${OVS_CNI_PLUGIN_IMAGE_NAME:-ovs-cni-plugin}
ovs_cni_plugin_image_version=${OVS_CNI_PLUGIN_IMAGE_VERSION:-latest}
ovs_cni_plugin_image_pull_policy=${OVS_CNI_PLUGIN_IMAGE_PULL_POLICY:-IfNotPresent}

ovs_cni_marker_image_repo=${OVS_CNI_MARKER_IMAGE_REPO:-quay.io/kubevirt}
ovs_cni_marker_image_name=${OVS_CNI_MARKER_IMAGE_NAME:-ovs-cni-marker}
ovs_cni_marker_image_version=${OVS_CNI_MARKER_IMAGE_VERSION:-latest}
ovs_cni_marker_image_pull_policy=${OVS_CNI_MARKER_IMAGE_PULL_POLICY:-IfNotPresent}

for template in manifests/*.in; do
    name=$(basename ${template%.in})
    sed \
        -e "s#\${NAMESPACE}#${namespace}#g" \
        -e "s#\${OVS_CNI_PLUGIN_IMAGE_REPO}#${ovs_cni_plugin_image_repo}#g" \
        -e "s#\${OVS_CNI_PLUGIN_IMAGE_NAME}#${ovs_cni_plugin_image_name}#g" \
        -e "s#\${OVS_CNI_PLUGIN_IMAGE_VERSION}#${ovs_cni_plugin_image_version}#g" \
        -e "s#\${OVS_CNI_PLUGIN_IMAGE_PULL_POLICY}#${ovs_cni_plugin_image_pull_policy}#g" \
        -e "s#\${OVS_CNI_MARKER_IMAGE_REPO}#${ovs_cni_marker_image_repo}#g" \
        -e "s#\${OVS_CNI_MARKER_IMAGE_NAME}#${ovs_cni_marker_image_name}#g" \
        -e "s#\${OVS_CNI_MARKER_IMAGE_VERSION}#${ovs_cni_marker_image_version}#g" \
        -e "s#\${OVS_CNI_MARKER_IMAGE_PULL_POLICY}#${ovs_cni_marker_image_pull_policy}#g" \
        ${template} > examples/${name}
done
