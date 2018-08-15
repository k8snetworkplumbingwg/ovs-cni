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
deploy local cluster, use following commands to control it.

```shell
# Deploy local Kubernetes cluster with one node
make cluster-up

# SSH to the node
./cluster/cli.sh

# Communicate with the Kubernetes cluster using kubectl
./cluster/kubectl.sh

# Build project, build images and push them to cluster's registry
make build cluster-sync

# Destroy the cluster
make cluster-down
```
