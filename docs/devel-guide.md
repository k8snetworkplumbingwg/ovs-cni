# Development

This document serves as a reference point to helper commands available under
this project.

## Building

```shell
# Run Go format and vet check
make format

# Build binaries of all components, make format is called as a part of it
make build

# Build binary of a specific component
make build-foo

# Refresh vendoring after dependencies changed
make dep
```

## Testing

```shell
# Run all unit tests
make test

# Run unit tests of a specific package
make test-cmd-foo
make test-pkg-bar

# Some tests might need root privileges, run them with sudo
sudo --preserve-env make test
```

## Containers

```shell
# Build containers of all components, make build is called as part of it
make docker-build

# Builder a container of a specific component
make docker-build-foo

# Push built containers to remote registry
make docker-push

# Push specific images to remote registry
make docker-push-foo

# All mentioned targets support REGISTRY variable (default is quay.io/kubevirt)
# Build images and push them to personal docker registry
REGISTRY=example.com/jdoe make docker-build docker-push
```

## Local Cluster

This project uses [kubevirtci](https://github.com/kubevirt/kubevirtci) to
deploy local cluster.

### Dockerized Kubernetes Provider

Refer to the [kubernetes 1.11.1 with multus document](../cluster/k8s-multus-1.11.1/README.md)

### Dockerized OCP Provider

Refer to the [OCP 3.10 with multus document](../cluster/os-3.10.0-multus/README.md)


### Usage

Use following commands to control it.

*note:* Default Provider is one node (master + worker) of kubernetes 1.11.1 with multus cni plugin.

```shell
# Deploy local Kubernetes cluster
export KUBEVIRT_PROVIDER=k8s-multus-1.11.1 # choose this provider
export KUBEVIRT_NUM_NODES=3 # master + two nodes
make cluster-up

# SSH to the node and open interactive shell
./cluster/cli.sh -i

# SSH to the node and run command
./cluster/cli.sh echo 'Hello World'

# Communicate with the Kubernetes cluster using kubectl
./cluster/kubectl.sh

# Build project, build images, push them to cluster's registry and install them
make build cluster-sync

# Destroy the cluster
make cluster-down
```
