REGISTRY ?= quay.io/kubevirt
IMAGE_TAG ?= latest

COMPONENTS = $(sort \
			 $(subst /,-,\
			 $(patsubst cmd/%/,%,\
			 $(dir \
			 $(shell find cmd/ -type f -name '*.go')))))

all: build

build: format $(patsubst %, build-%, $(COMPONENTS))

build-%:
	hack/version.sh > ./cmd/$(subst -,/,$*)/.version
	cd cmd/$(subst -,/,$*) && go fmt && go vet && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=on go build -tags no_openssl -mod vendor

format:
	go fmt ./pkg/... ./cmd/...
	go vet ./pkg/... ./cmd/...

test:
	go test ./cmd/... ./pkg/... -v --ginkgo.v

docker-test:
	hack/test-dockerized.sh

test-%:
	go test ./$(subst -,/,$*)/... -v --ginkgo.v

functest:
	hack/functests.sh

docker-build: $(patsubst %, docker-build-%, $(COMPONENTS))

docker-build-%: build-%
	docker build -t ${REGISTRY}/ovs-cni-$*:${IMAGE_TAG} ./cmd/$(subst -,/,$*)

docker-push: $(patsubst %, docker-push-%, $(COMPONENTS))

docker-push-%:
	docker push ${REGISTRY}/ovs-cni-$*:${IMAGE_TAG}

dep:
	go get -t -u ./...
	go mod tidy
	go mod vendor

manifests:
	./hack/build-manifests.sh

cluster-up:
	./cluster/up.sh

cluster-down:
	./cluster/down.sh

cluster-sync: build
	./cluster/sync.sh

.PHONY: build format test docker-build docker-push dep clean-dep manifests cluster-up cluster-down cluster-sync
