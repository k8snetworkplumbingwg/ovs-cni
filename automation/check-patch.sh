#!/bin/bash -xe

main() {
    echo "TODO: Add functional tests"

    echo $HOME

    env


    cd ..
    mkdir -p go/src/kubevirt.io
    mkdir -p go/pkg
    export GOPATH=$(pwd)/go

    ln -s $(pwd)/ovs-cni go/src/kubevirt.io/
    ls -la go/src/kubevirt.io/ovs-cni
    cd go/src/kubevirt.io/ovs-cni
    pwd
    ls -la

    go version

    GOOS=linux CGO_ENABLED=1 make build
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
