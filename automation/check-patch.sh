#!/bin/bash -xe

main() {
    echo "Start ovs process"
    /usr/share/openvswitch/scripts/ovs-ctl --system-id=random start

    cd ..
    mkdir -p go/src/kubevirt.io
    mkdir -p go/pkg

    ln -s $(pwd)/ovs-cni go/src/kubevirt.io/
    ls -la go/src/kubevirt.io/ovs-cni

    export GOPATH=$(pwd)/go
    cd go/src/kubevirt.io/ovs-cni

    echo "Run tests"
    make && make test
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
