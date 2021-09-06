#!/usr/bin/env bash
set -e
set -o pipefail

# launch OVS
function quit {
  /usr/share/openvswitch/scripts/ovs-ctl stop
  exit 0
}
trap quit SIGTERM
/usr/share/openvswitch/scripts/ovs-ctl start --system-id=random
eval "$@"
