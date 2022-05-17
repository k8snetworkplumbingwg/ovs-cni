#!/usr/bin/env bash
#
# This file is part of the KubeVirt project
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
# Copyright 2017 Red Hat, Inc.
#

set -e
source $(dirname "$0")/common.sh

DOCKER_DIR=${KUBEVIRT_DIR}/hack/docker-builder
OCI_BIN=${OCI_BIN:-$(determine_cri_bin)}

SYNC_OUT=${SYNC_OUT:-true}

BUILDER=${job_prefix}

SYNC_VENDOR=${SYNC_VENDOR:-false}

TEMPFILE=".rsynctemp"

# Reduce verbosity if an automated build
BUILD_QUIET=
if [ -n "$JOB_NAME" -o -n "$TRAVIS_BUILD_ID" ]; then
    BUILD_QUIET="-q"
fi

# Build the build container
(cd ${DOCKER_DIR} && ${OCI_BIN} build . ${BUILD_QUIET} -t ${BUILDER})

${OCI_BIN} run --rm --privileged --network host -v /lib/modules:/lib/modules -v `pwd`:/root/go/src/github.com/k8snetworkplumbingwg/ovs-cni -w "/root/go/src/github.com/k8snetworkplumbingwg/ovs-cni" ${BUILDER} make test
