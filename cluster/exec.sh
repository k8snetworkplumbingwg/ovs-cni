#!/bin/bash

set -e

SCRIPTS_PATH="$(dirname "$(realpath "$0")")"
source ${SCRIPTS_PATH}/cluster.sh

node="$1"
shift

# Skip the "--" separator if present
if [ "$1" = "--" ]; then shift; fi

${OCI_BIN} exec "$node" "$@"
