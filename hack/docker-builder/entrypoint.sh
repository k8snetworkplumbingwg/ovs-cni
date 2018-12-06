#!/bin/bash
set -e
set -o pipefail

source /etc/profile.d/gimme.sh
export GOPATH="/root/go"

# launch OVS
function quit {
  /usr/share/openvswitch/scripts/ovs-ctl stop
  exit 0
}
trap quit SIGTERM
/usr/share/openvswitch/scripts/ovs-ctl start --system-id=random
eval "$@"
