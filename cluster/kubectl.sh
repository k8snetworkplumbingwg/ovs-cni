#!/bin/bash -e

source ./cluster/gocli.sh

k8s_port=$($gocli ports k8s | tr -d '\r')

kubectl --kubeconfig ./kubeconfig "$@"
