# AGENTS.md - ovs-cni

Kubernetes CNI plugin that attaches pods to Open vSwitch bridges. Works with Multus and NetworkAttachmentDefinition CRD.

## Project structure

```
cmd/                    # 4 binary entry points
  plugin/               #   CNI plugin (ovs) - attach pods to OVS bridges
  marker/               #   Daemon exposing OVS bridges as node resources
  mirror-producer/      #   CNI plugin for traffic mirroring (producer)
  mirror-consumer/      #   CNI plugin for traffic mirroring (consumer)
pkg/                    # Core libraries
  plugin/               #   Main CNI logic (CmdAdd/CmdCheck/CmdDel)
  config/               #   Config loading with merge support
  types/                #   NetConf, MirrorNetConf structs
  ovsdb/                #   OVS database client wrapper
  sriov/                #   SR-IOV hardware offload
  marker/               #   Node resource marker logic
  mirror-producer/      #   Mirror producer implementation
  mirror-consumer/      #   Mirror consumer implementation
  utils/                #   Utilities and cache helpers
  testhelpers/          #   Shared test utilities
tests/                  # E2E/functional tests (require running cluster)
cluster/                # Local K8s cluster management (kubevirtci)
hack/                   # Build scripts, CI helpers
manifests/              # K8s manifest templates
examples/               # Example deployments and NetworkAttachmentDefinitions
docs/                   # User and developer documentation
vendor/                 # Vendored Go dependencies
```

## Build and test

Go toolchain is auto-installed to `build/_output/bin/go/` by `hack/install-go.sh`. No system Go required.

```bash
make build              # Format, vet, build all 4 binaries
make lint               # golangci-lint v2 (config in .golangci.yml)
make test               # Unit tests (Ginkgo/Gomega) - may need sudo for netns tests
make functest           # E2E tests against a running cluster
make docker-build       # Container image with all binaries
make docker-test        # Run tests inside Docker
```

Build individual components: `make build-plugin`, `make build-marker`, etc.
Test individual packages: `make test-pkg-plugin`, `make test-cmd-marker`, etc.

Binaries output to `cmd/<component>/` (built in-place by `go build`).

## Go conventions

- **Module:** `github.com/k8snetworkplumbingwg/ovs-cni`
- **Go version:** 1.25.5
- **Build flags:** `CGO_ENABLED=0 GO111MODULE=on -tags no_openssl -mod vendor`
- **Dependencies are vendored.** After modifying `go.mod`, run `make dep` to update vendor/.
- **Logging:** Use `logutils.Log` only. Logrus is banned via depguard.
- **Imports:** Local prefix `github.com/k8snetworkplumbingwg/ovs-cni` (enforced by goimports).
- **Tests:** Ginkgo v2 + Gomega. Dot-imports allowed for `ginkgo/v2`, `gomega`, `gomega/gstruct`.
- **Linting:** 20+ linters enabled. Relaxed rules in `_test.go` files. Run `make lint` before submitting.

## Local development cluster

Uses kubevirtci to spin up a local Kubernetes cluster with Multus pre-installed.

```bash
make cluster-up         # Deploy local K8s cluster
make cluster-sync       # Build and deploy ovs-cni to the cluster
make cluster-down       # Tear down cluster
./cluster/kubectl.sh    # kubectl with correct kubeconfig
./cluster/cli.sh ssh node01  # SSH into cluster node
```

## Container image

- **Dockerfile:** `cmd/Dockerfile`
- **Registry:** `ghcr.io/k8snetworkplumbingwg/ovs-cni-plugin`
- **Multi-arch:** amd64, arm64, ppc64le, s390x
- **Base:** UBI 9 minimal (runtime), CentOS Stream 9 (builder)

## Git

- **Always sign off commits with `git commit -s`.** This project requires DCO (Developer Certificate of Origin) sign-off.

## CI

GitHub Actions workflows in `.github/workflows/`:
- `image-build-test.yaml` - Build validation on PRs (multi-arch)
- `image-push-main.yaml` - Push `:latest` on merge to main
- `image-push-release.yaml` - Push release tags

Dependabot manages Docker, GitHub Actions, and Go module updates.
