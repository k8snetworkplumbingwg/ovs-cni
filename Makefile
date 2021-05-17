REGISTRY ?= quay.io/kubevirt
IMAGE_TAG ?= latest

COMPONENTS = $(sort \
			 $(subst /,-,\
			 $(patsubst cmd/%/,%,\
			 $(dir \
			 $(shell find cmd/ -type f -name '*.go')))))

BIN_DIR = $(CURDIR)/build/_output/bin/
export GOROOT=$(BIN_DIR)/go/
export GOBIN = $(GOROOT)/bin/
export PATH := $(GOBIN):$(PATH)
GOPATH = $(CURDIR)/.gopath
ORG_PATH = github.com/k8snetworkplumbingwg
PACKAGE = ovs-cni
REPO_PATH = $(ORG_PATH)/$(PACKAGE)
BASE = $(GOPATH)/src/$(REPO_PATH)
PKGS = $(or $(PKG),$(shell cd $(BASE) && env GOPATH=$(GOPATH) $(GO) list ./... | grep -v "$(PACKAGE)/vendor/" | grep -v "$(PACKAGE)/tests/cluster" | grep -v "$(PACKAGE)/tests/node"))
V = 0
Q = $(if $(filter 1,$V),,@)

all: lint build

GO := $(GOBIN)/go

$(GO):
	hack/install-go.sh $(BIN_DIR)

$(BASE): ; $(info  setting GOPATH...)
	@mkdir -p $(dir $@)
	@ln -sf $(CURDIR) $@

GOLINT = $(GOBIN)/golint
$(GOBIN)/golint: | $(BASE) ; $(info  building golint...)
	$Q go get -u golang.org/x/lint/golint

build: format $(patsubst %, build-%, $(COMPONENTS))

lint: | $(GO) $(BASE) $(GOLINT) ; $(info  running golint...) @ ## Run golint
	$Q cd $(BASE) && ret=0 && for pkg in $(PKGS); do \
		test -z "$$($(GOLINT) $$pkg | tee /dev/stderr)" || ret=1 ; \
	 done ; exit $$ret

build-%: $(GO)
	hack/version.sh > ./cmd/$(subst -,/,$*)/.version
	cd cmd/$(subst -,/,$*) && $(GO) fmt && $(GO) vet && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=on $(GO) build -tags no_openssl -mod vendor

format: $(GO)
	$(GO) fmt ./pkg/... ./cmd/...
	$(GO) vet ./pkg/... ./cmd/...

test: $(GO)
	$(GO) test ./cmd/... ./pkg/... -v --ginkgo.v

docker-test:
	hack/test-dockerized.sh

test-%: $(GO)
	$(GO) test ./$(subst -,/,$*)/... -v --ginkgo.v

functest: $(GO)
	GO=$(GO) hack/functests.sh

docker-build: $(patsubst %, docker-build-%, $(COMPONENTS))

docker-build-%: build-%
	docker build -t ${REGISTRY}/ovs-cni-$*:${IMAGE_TAG} ./cmd/$(subst -,/,$*)

docker-push: $(patsubst %, docker-push-%, $(COMPONENTS))

docker-push-%:
	docker push ${REGISTRY}/ovs-cni-$*:${IMAGE_TAG}

dep: $(GO)
	$(GO) mod tidy
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
