REGISTRY ?= ghcr.io/k8snetworkplumbingwg
IMAGE_TAG ?= latest
IMAGE_GIT_TAG ?= $(shell git describe --abbrev=8 --tags)

COMPONENTS = $(sort \
			 $(subst /,-,\
			 $(patsubst cmd/%/,%,\
			 $(dir \
			 $(shell find cmd/ -type f -name '*.go')))))

BIN_DIR = $(CURDIR)/build/_output/bin/
export GOROOT=$(BIN_DIR)/go/
export GOBIN = $(GOROOT)/bin/
export PATH := $(GOBIN):$(PATH):$(BIN_DIR)
GOPATH = $(CURDIR)/.gopath
GOARCH ?= $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
ORG_PATH = github.com/k8snetworkplumbingwg
PACKAGE = ovs-cni
OCI_BIN ?= $(shell if podman ps >/dev/null 2>&1; then echo podman; elif docker ps >/dev/null 2>&1; then echo docker; fi)
REPO_PATH = $(ORG_PATH)/$(PACKAGE)
BASE = $(GOPATH)/src/$(REPO_PATH)
PKGS = $(or $(PKG),$(shell cd $(BASE) && env GOPATH=$(GOPATH) $(GO) list ./... | grep -v "$(PACKAGE)/vendor/" | grep -v "$(PACKAGE)/tests/cluster" | grep -v "$(PACKAGE)/tests/node"))
V = 0
Q = $(if $(filter 1,$V),,@)
TLS_SETTING := $(if $(filter $(OCI_BIN),podman),--tls-verify=false,)

# Go settings
GO_BUILD_OPTS ?= CGO_ENABLED=0 GO111MODULE=on
GO_TAGS ?= -tags no_openssl
GO_FLAGS ?= -mod vendor

all: lint build

GO := $(GOBIN)/go

$(GO):
	hack/install-go.sh $(BIN_DIR)

$(BASE): ; $(info  setting GOPATH...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

GOLANGCI = $(GOBIN)/golangci-lint
$(GOBIN)/golangci-lint: $(GO) | $(BASE) ; $(info  building golangci-lint...)
	$Q $(GO) install -mod=mod github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.6

build: format $(patsubst %, build-%, $(COMPONENTS))

lint: $(GO) $(GOLANGCI)
	$(GOLANGCI) run

build-%: $(GO)
	cd cmd/$* && $(GO) fmt && $(GO) vet && GOOS=linux GOARCH=$(GOARCH) $(GO_BUILD_OPTS) $(GO) build $(GO_TAGS)

format: $(GO)
	$(GO) fmt ./pkg/... ./cmd/...
	$(GO) vet ./pkg/... ./cmd/...

build-host-local-plugin:
	if [ ! -f $(BIN_DIR)/host-local -a `uname` = 'Linux' ]; then\
		cd $(BIN_DIR) && \
		git clone https://github.com/containernetworking/plugins && \
		cd plugins && git checkout v1.0.1 && cd .. && \
		./plugins/build_linux.sh && \
		cp ./plugins/bin/host-local . && \
		rm -rf plugins; \
	fi

test: $(GO) build-host-local-plugin
	$(GO) test -mod=readonly ./cmd/... ./pkg/... -v --ginkgo.v

docker-test:
	hack/test-dockerized.sh

test-%: $(GO) build-host-local-plugin
	$(GO) test ./$(subst -,/,$*)/... -v --ginkgo.v

functest: $(GO)
	GO=$(GO) hack/functests.sh

docker-build:
	hack/get_version.sh > .version
	$(OCI_BIN) build --build-arg goarch=${GOARCH} -t ${REGISTRY}/ovs-cni-plugin:${IMAGE_TAG} -f ./cmd/Dockerfile .

docker-push:
	$(OCI_BIN) push ${TLS_SETTING} ${REGISTRY}/ovs-cni-plugin:${IMAGE_TAG}
	$(OCI_BIN) tag ${REGISTRY}/ovs-cni-plugin:${IMAGE_TAG} ${REGISTRY}/ovs-cni-plugin:${IMAGE_GIT_TAG}
	$(OCI_BIN) push ${TLS_SETTING} ${REGISTRY}/ovs-cni-plugin:${IMAGE_GIT_TAG}

dep: $(GO)
	$(GO) mod vendor

manifests:
	./hack/build-manifests.sh

cluster-up:
	./cluster/up.sh

cluster-down:
	./cluster/down.sh

cluster-sync: build
	./cluster/sync.sh

.PHONY: build format test docker-build docker-push dep clean-dep manifests cluster-up cluster-down cluster-sync lint
