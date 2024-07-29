#!/bin/bash -e

KUBEVIRTCI_TAG=$(curl -L -Ss https://storage.googleapis.com/kubevirt-prow/release/kubevirt/kubevirtci/latest)
[[ ${#KUBEVIRTCI_TAG} != "18" ]] && echo "error getting KUBEVIRTCI_TAG" && exit 1

sed -i "s/\(KUBEVIRTCI_TAG:-\)[^}]*/\1${KUBEVIRTCI_TAG}/" cluster/cluster.sh

git --no-pager diff cluster/cluster.sh | grep KUBEVIRTCI_TAG || true
