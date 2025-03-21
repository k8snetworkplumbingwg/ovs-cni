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

export KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER:-'k8s-1.26-centos9'}
export KUBEVIRTCI_TAG=2305081329-48e913c

KUBEVIRTCI_REPO='https://github.com/kubevirt/kubevirtci.git'
# The CLUSTER_PATH var is used in cluster folder and points to the _kubevirtci where the cluster is deployed from.
CLUSTER_PATH=${CLUSTER_PATH:-"${PWD}/_kubevirtci/"}

function cluster::_get_repo() {
    git --git-dir ${CLUSTER_PATH}/.git remote get-url origin
}

function cluster::_get_tag() {
    git -C ${CLUSTER_PATH} describe --tags
}

function cluster::install() {
    # Remove cloned kubevirtci repository if it does not match the requested one
    if [ -d ${CLUSTER_PATH} ]; then
        if [ $(cluster::_get_repo) != ${KUBEVIRTCI_REPO} -o $(cluster::_get_tag) != ${KUBEVIRTCI_TAG} ]; then
            rm -rf ${CLUSTER_PATH}
        fi
    fi

    if [ ! -d ${CLUSTER_PATH} ]; then
        git clone https://github.com/kubevirt/kubevirtci.git ${CLUSTER_PATH}
        (
            cd ${CLUSTER_PATH}
            git checkout ${KUBEVIRTCI_TAG}
        )
    fi
}

function cluster::path() {
    echo -n ${CLUSTER_PATH}
}

function cluster::kubeconfig() {
    if [ ${KUBEVIRT_PROVIDER} != "external" ]; then
        echo -n ${CLUSTER_PATH}/_ci-configs/${KUBEVIRT_PROVIDER}/.kubeconfig
    else
        [[ -n $KUBECONFIG ]] || (echo "missing KUBECONFIG"; exit 1)
        echo -n ${KUBECONFIG}
    fi
}
