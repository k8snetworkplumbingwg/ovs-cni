#!/bin/bash -xe

# This script should be able to execute functional tests against Kubernetes
# cluster on any environment with basic dependencies listed in
# check-patch.packages installed and docker running.
#
# yum -y install automation/check-patch.packages
# automation/check-patch.e2e.sh

teardown() {
    make cluster-down
}

main() {
    export KUBEVIRT_PROVIDER='k8s-1.17.0'

    source automation/setup.sh
    cd ${TMP_PROJECT_PATH}

    echo 'Run functional tests'
    make docker-test

    echo 'Run e2e tests'
    make cluster-down
    make cluster-up
    trap teardown EXIT SIGINT SIGTERM SIGSTOP
    make cluster-sync
    ginko_params="-ginkgo.noColor --junit-output=$ARTIFACTS_PATH/tests.junit.xml"
    FUNC_TEST_ARGS=$ginko_params make functest
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
