#!/bin/bash

set -e

node="$1"
shift

# Skip the "--" separator if present
if [ "$1" = "--" ]; then shift; fi

docker exec "$node" "$@"
