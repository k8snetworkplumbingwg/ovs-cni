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
#

KUBEVIRT_DIR="$(
    cd "$(dirname "$BASH_SOURCE[0]")/../"
    pwd
)"

VENDOR_DIR=$KUBEVIRT_DIR/vendor


KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER:-k8s-multus-1.12.2}
KUBEVIRT_NUM_NODES=${KUBEVIRT_NUM_NODES:-1}
KUBEVIRT_MEMORY_SIZE=${KUBEVIRT_MEMORY_SIZE:-5120M}

OUT_DIR=$KUBEVIRT_DIR/_out
TESTS_OUT_DIR=$OUT_DIR/tests

function build_func_tests() {
    mkdir -p ${TESTS_OUT_DIR}/
    ginkgo build ${KUBEVIRT_DIR}/tests
    mv ${KUBEVIRT_DIR}/tests/tests.test ${TESTS_OUT_DIR}/
}

# Use this environment variable to set a custom pkgdir path
# Useful for cross-compilation where the default -pkdir for cross-builds may not be writable
#KUBEVIRT_GO_BASE_PKGDIR="${GOPATH}/crossbuild-cache-root/"

#If run on jenkins, let us create isolated environments based on the job and
# the executor number
provider_prefix=${JOB_NAME:-${KUBEVIRT_PROVIDER}}${EXECUTOR_NUMBER}
job_prefix=${JOB_NAME:-kubevirt}${EXECUTOR_NUMBER}

# Populate an environment variable with the version info needed.
# It should be used for everything which needs a version when building (not generating)
# IMPORTANT:
# RIGHT NOW ONLY RELEVANT FOR BUILDING, GENERATING CODE OUTSIDE OF GIT
# IS NOT NEEDED NOR RECOMMENDED AT THIS STAGE.

function kubevirt_version() {
    if [ -n "${KUBEVIRT_VERSION}" ]; then
        echo ${KUBEVIRT_VERSION}
    elif [ -d ${KUBEVIRT_DIR}/.git ]; then
        echo "$(git describe --always --tags)"
    else
        echo "undefined"
    fi
}
KUBEVIRT_VERSION="$(kubevirt_version)"

determine_cri_bin() {
    if podman ps >/dev/null 2>&1; then
        echo podman
    elif docker ps >/dev/null 2>&1; then
        echo docker
    else
        echo ""
    fi
}
