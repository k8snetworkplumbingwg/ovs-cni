#!/bin/bash -xe

quay_login() {
    (
        set +x
        docker login \
            -u "$QUAY_REGISTRY_USERNAME" \
            -p "$QUAY_REGISTRY_PASSWORD" \
            quay.io
    )
}

main() {
    quay_login
    cd ..
    mkdir -p go/src/kubevirt.io
    mkdir -p go/pkg
    export GOPATH=$(pwd)/go
    ln -s $(pwd)/ovs-cni go/src/kubevirt.io/
    cd go/src/kubevirt.io/ovs-cni

    if [ "$(git describe --tags --abbrev=0  --match 'v[0-9].[0-9].[0-9]' 2> /dev/null | wc -l)" == "0" ]
    then
        export TAG="master"
    fi
     if [ "$(git describe --tags --abbrev=0  --match 'v[0-9].[0-9].[0-9]' 2> /dev/null | wc -l)" == "1" ]
    then
        export TAG=`git describe --tags --abbrev=0  --match 'v[0-9].[0-9].[0-9]'`
    fi

    make docker-build IMAGE_TAG=$TAG

}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
