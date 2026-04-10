#!/bin/bash -xe

destination=$1
version=${KIND_VERSION:-v0.27.0}
arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

mkdir -p $destination
curl -Lo $destination/kind https://kind.sigs.k8s.io/dl/${version}/kind-linux-${arch}
chmod +x $destination/kind
