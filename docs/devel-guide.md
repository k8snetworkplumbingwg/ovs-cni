# Development

This document serves as a reference point to helper commands available under
this project.

## Building

**Note:** OVS CNI plugin must be compiled with Go 1.10 or newer.

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

# All mentioned targets support IMAGE_TAG variable (default is latest)
# Build images and push them with a custom image tag
IMAGE_TAG=test make docker-build docker-push
```

## Manifests

Manifests in `examples/` folder are built from templates kept in `manifests/`.

```shell
# build manifests
make manifests
```

Manifest templates contain following variables. It it possible to adjust them
my setting environment variables before calling `make manifests`.

```
NAMESPACE # default kube-system

OVS_CNI_IMAGE_REPO # default quay.io/kubevirt
OVS_CNI_IMAGE_NAME # default ovs-cni-plugin
OVS_CNI_IMAGE_VERSION # default latest

OVS_CNI_PLUGIN_IMAGE_REPO # default quay.io/kubevirt
OVS_CNI_PLUGIN_IMAGE_NAME # default ovs-cni-plugin
OVS_CNI_PLUGIN_IMAGE_VERSION # default latest

OVS_CNI_MARKER_IMAGE_REPO # default quay.io/kubevirt
OVS_CNI_MARKER_IMAGE_NAME # default ovs-cni-marker
OVS_CNI_MARKER_IMAGE_VERSION # default latest
```

## Local Cluster

This project uses [kubevirtci](https://github.com/kubevirt/kubevirtci) to
deploy local cluster.

### Usage

Use following commands to control it.

*note:* Default Provider is one node (master + worker) of kubernetes 1.11.1
with multus cni plugin.

```shell
# Deploy local Kubernetes cluster
make cluster-up

# SSH to node01 and open interactive shell
./cluster/cli.sh ssh node01

# SSH to node01 and run command
./cluster/cli.sh ssh node01 echo 'Hello World'

# Communicate with the Kubernetes cluster using kubectl
./cluster/kubectl.sh

# Build project, build images, push them to cluster's registry and install them
make build cluster-sync

# Destroy the cluster
make cluster-down
```
