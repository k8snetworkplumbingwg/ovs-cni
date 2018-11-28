#!/bin/bash -xe

main() {
    TARGET="$0"
    TARGET="${TARGET#./}"
    TARGET="${TARGET%.*}"
    TARGET="${TARGET#*.}"
    echo "TARGET=$TARGET"
    export TARGET

    cd ..
    export GOROOT=/usr/local/go
    export GOPATH=$(pwd)/go
    export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
    mkdir -p $GOPATH

    echo "Install Go 1.10"
    mkdir -p /gimme
    curl -sL https://raw.githubusercontent.com/travis-ci/gimme/master/gimme | HOME=/gimme bash >> /etc/profile.d/gimme.sh
    GIMME_GO_VERSION=1.10 source /etc/profile.d/gimme.sh

    echo "Start ovs process"
    /usr/share/openvswitch/scripts/ovs-ctl --system-id=random start

    mkdir -p $GOPATH/src/kubevirt.io
    mkdir -p $GOPATH/pkg
    ln -s $(pwd)/ovs-cni $GOPATH/src/kubevirt.io/
    cd $GOPATH/src/kubevirt.io/ovs-cni

    echo "Run tests"
    make build test

    echo "Run functional tests"
    exec automation/test.sh
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
