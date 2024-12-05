#!/bin/bash -xe

destination=$1
version=$(curl -s https://go.dev/dl/?mode=json | jq -r ".[0].version")
arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
tarball=$version.linux-$arch.tar.gz
url=https://dl.google.com/go/

mkdir -p $destination
curl -L $url/$tarball -o $destination/$tarball
tar -xf $destination/$tarball -C $destination
