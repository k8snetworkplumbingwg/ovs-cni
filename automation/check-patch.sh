#!/bin/bash -xe

main() {
    TARGET="$0"
    TARGET="${TARGET#./}"
    TARGET="${TARGET%.*}"
    TARGET="${TARGET#*.}"
    echo "TARGET=$TARGET"
    export TARGET

    cd ..
    mkdir -p go/src/kubevirt.io
    mkdir -p go/pkg
    export GOPATH=$(pwd)/go
    ln -s $(pwd)/ovs-cni go/src/kubevirt.io/
    cd go/src/kubevirt.io/ovs-cni

    echo "Run functional tests"
    exec automation/test.sh
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
