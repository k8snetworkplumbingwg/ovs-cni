#!/bin/bash -xe

destination=$1
version=$(curl -s https://go.dev/dl/?mode=json | jq -r ".[0].version")
tarball=$version.linux-amd64.tar.gz
url=https://dl.google.com/go/

mkdir -p $destination
curl -L $url/$tarball -o $destination/$tarball
tar -xf $destination/$tarball -C $destination
