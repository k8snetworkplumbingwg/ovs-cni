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

export KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME:-ovs-cni}
export KIND_NODE_IMAGE=${KIND_NODE_IMAGE:-localhost/kindest/node-ovs:latest}

OCI_BIN=${OCI_BIN:-$(if podman ps >/dev/null 2>&1; then echo podman; elif docker ps >/dev/null 2>&1; then echo docker; fi)}
export OCI_BIN

if [ -z "${OCI_BIN}" ]; then
    echo "ERROR: No container runtime found. Install docker or podman." >&2
    exit 1
fi

if [ "${OCI_BIN}" = "podman" ]; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
fi

function cluster::kubeconfig() {
    echo -n "${HOME}/.kube/kind-config-${KIND_CLUSTER_NAME}"
}
