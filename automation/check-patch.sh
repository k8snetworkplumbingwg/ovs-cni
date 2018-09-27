#!/bin/bash -xe

main() {
    echo "TODO: Add functional tests"
    systemctl start openvswitch

    cd ..
    mkdir -p go/src/kubevirt.io
    mkdir -p go/pkg

    ln -s $(pwd)/ovs-cni go/src/kubevirt.io/
    ls -la go/src/kubevirt.io/ovs-cni

    export GOPATH=$(pwd)/go
    cd go/src/kubevirt.io/ovs-cni

    cd cmd/plugin/
    GOOS=linux go build ./

    make tests
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
