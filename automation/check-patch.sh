#!/bin/bash -xe

main() {
    echo "Start ovs process"
    /usr/share/openvswitch/scripts/ovs-ctl --system-id=random start

    cd ..
    mkdir -p go/src/kubevirt.io
    mkdir -p go/pkg
    export GOPATH=$(pwd)/go
    ln -s $(pwd)/ovs-cni go/src/kubevirt.io/
    cd go/src/kubevirt.io/ovs-cni

    echo "Run tests"

    git describe --tags

    if [ "$(git describe --tags --abbrev=0  --match 'v[0-9].[0-9].[0-9]' 2> /dev/null | wc -l)" == "0" ]
    then
        export IMAGE_TAG="master"
    fi

    if [ "$(git describe --tags --abbrev=0  --match 'v[0-9].[0-9].[0-9]' 2> /dev/null | wc -l)" == "1" ]
    then
        export IMAGE_TAG=`git describe --tags --abbrev=0  --match 'v[0-9].[0-9].[0-9]'`
    fi

    #make docker-build
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
