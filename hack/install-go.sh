#!/bin/bash -xe

destination=$1
version=1.21.7
arch="$(arch | sed s'/aarch64/arm64/' | sed s'/x86_64/amd64/')"
tarball=go$version.linux-$arch.tar.gz
url=https://dl.google.com/go/

mkdir -p $destination
curl -L $url/$tarball -o $destination/$tarball
tar -xf $destination/$tarball -C $destination
