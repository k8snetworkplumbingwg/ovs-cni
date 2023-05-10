#!/bin/bash -xe

# This script should be able to execute functional tests against Kubernetes
# cluster on any environment with basic dependencies listed in
# check-patch.packages installed and podman / docker running.
#
# yum -y install automation/check-patch.packages
# automation/check-patch.e2e.sh

teardown() {
    make cluster-down
}

main() {
    export KUBEVIRT_PROVIDER='k8s-1.26-centos9'

    source automation/setup.sh
    cd ${TMP_PROJECT_PATH}

    echo 'Run golint'
    make lint

    echo 'Run functional tests'
    make docker-test

    echo 'Run e2e tests'
    make cluster-down
    make cluster-up
    trap teardown EXIT SIGINT SIGTERM SIGSTOP
    make cluster-sync
    make E2E_TEST_ARGS="-ginkgo.v -test.v -ginkgo.noColor -test.timeout 20m --junit-output=$ARTIFACTS/junit.functest.xml" functest
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
