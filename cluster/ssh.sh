#!/bin/bash

set -e

SCRIPTS_PATH="$(dirname "$(realpath "$0")")"
source ${SCRIPTS_PATH}/cluster.sh
cluster::install

$(cluster::path)/cluster-up/cli.sh ssh "$@"
