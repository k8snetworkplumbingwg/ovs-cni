#!/bin/bash
#
# Copyright 2018 Red Hat, Inc.
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
#

set -e

namespace=${NAMESPACE:-kube-system}

ovs_cni_image_repo=${OVS_CNI_IMAGE_REPO:-quay.io/kubevirt}
ovs_cni_image_name=${OVS_CNI_IMAGE_NAME:-ovs-cni-plugin}
ovs_cni_image_version=${OVS_CNI_IMAGE_VERSION:-latest}

multus_image_repo=${MULTUS_IMAGE_REPO:-docker.io/nfvpe}
multus_image_name=${MULTUS_IMAGE_NAME:-multus}
multus_image_version=${MULTUS_IMAGE_VERSION:-latest}

openshift_node_image_repo=${OPENSHIFT_NODE_IMAGE_REPO:-docker.io/openshift}
openshift_node_image_name=${OPENSHIFT_NODE_IMAGE_NAME:-origin-node}
openshift_node_image_version=${OPENSHIFT_NODE_IMAGE_VERSION:-v3.10.0-rc.0}

for template in manifests/*.in; do
    name=$(basename ${template%.in})
    sed \
        -e "s#\${NAMESPACE}#${namespace}#g" \
        -e "s#\${OVS_CNI_IMAGE_REPO}#${ovs_cni_image_repo}#g" \
        -e "s#\${OVS_CNI_IMAGE_NAME}#${ovs_cni_image_name}#g" \
        -e "s#\${OVS_CNI_IMAGE_VERSION}#${ovs_cni_image_version}#g" \
        -e "s#\${MULTUS_IMAGE_REPO}#${multus_image_repo}#g" \
        -e "s#\${MULTUS_IMAGE_NAME}#${multus_image_name}#g" \
        -e "s#\${MULTUS_IMAGE_VERSION}#${multus_image_version}#g" \
        -e "s#\${OPENSHIFT_NODE_IMAGE_REPO}#${openshift_node_image_repo}#g" \
        -e "s#\${OPENSHIFT_NODE_IMAGE_NAME}#${openshift_node_image_name}#g" \
        -e "s#\${OPENSHIFT_NODE_IMAGE_VERSION}#${openshift_node_image_version}#g" \
        ${template} > examples/${name}
done
