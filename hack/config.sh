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

unset docker_tag master_ip network_provider kubeconfig manifest_docker_prefix namespace image_pull_policy

KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER:-${PROVIDER}}

source ${KUBEVIRT_PATH}hack/config-default.sh

# Allow different providers to override default config values
test -f "hack/config-provider-${KUBEVIRT_PROVIDER}.sh" && source hack/config-provider-${KUBEVIRT_PROVIDER}.sh

# Let devs override any default variables, to avoid needing
# to change the version controlled config-default.sh file
test -f "hack/config-local.sh" && source hack/config-local.sh

export docker_tag master_ip network_provider kubeconfig namespace image_pull_policy
