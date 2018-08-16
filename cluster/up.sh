#!/bin/bash
#
# Copyright 2018 Red Hat, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

set -e

source ./cluster/gocli.sh

$gocli run --random-ports --nodes 1 --background kubevirtci/k8s-1.10.3

k8s_port=$($gocli ports k8s | tr -d '\r')

$gocli scp /etc/kubernetes/admin.conf - > ./kubeconfig
kubectl --kubeconfig=./kubeconfig config set-cluster kubernetes --server=https://127.0.0.1:$k8s_port
kubectl --kubeconfig=./kubeconfig config set-cluster kubernetes --insecure-skip-tls-verify=true

echo 'Wait until all nodes are ready'
until [[ $(./cluster/kubectl.sh get nodes --no-headers | wc -l) -eq $(./cluster/kubectl.sh get nodes --no-headers | grep Ready | wc -l) ]]; do
    sleep 1
done

echo 'Wait until all pods are running'
until [[ $(./cluster/kubectl.sh get pods --all-namespaces --no-headers | wc -l) -eq $(./cluster/kubectl.sh get pods --all-namespaces --no-headers | grep Running | wc -l) ]]; do
    sleep 1
done
