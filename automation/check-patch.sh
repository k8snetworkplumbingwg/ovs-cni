#!/bin/bash -xe

main() {
    echo "TODO: Add functional tests"
    mkdir -p go/src/ovs-cni
    ls -la
    pwd
    ln -s / go/src/
    export GOPATH=$(pwd)/go
    export IMAGE_TAG=`./hack/get_tag.sh`
    make docker-build
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
