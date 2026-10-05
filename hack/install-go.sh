#!/bin/bash -xe

destination=$1
version="go$(awk '$1 == "go" { print $2; exit }' go.mod)"
if [ "$version" = "go" ]; then
    echo "could not determine Go version from go.mod" >&2
    exit 1
fi
arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
tarball=$version.linux-$arch.tar.gz
url=https://dl.google.com/go/

mkdir -p $destination
curl -L $url/$tarball -o $destination/$tarball
tar -xf $destination/$tarball -C $destination
