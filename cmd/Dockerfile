FROM --platform=$BUILDPLATFORM quay.io/centos/centos:stream9 AS builder

RUN mkdir /workdir
# Support overriding target GOARCH during `make docker-build`
ARG goarch=

# these variable are automatically set during the multiarch build by docker buildx
ARG TARGETOS
ARG TARGETARCH
ENV TARGETOS=${TARGETOS:-linux}
ENV TARGETARCH=${TARGETARCH:-amd64}

ARG BUILDOS
ARG BUILDARCH
ENV BUILDOS=${BUILDOS:-linux}
ENV BUILDARCH=${BUILDARCH:-amd64}

ENV GOOS=${TARGETOS}
ENV GOARCH=${goarch:-$TARGETARCH}
ENV CGO_ENABLED=0
ENV GOFLAGS=-mod=vendor

WORKDIR /workdir

RUN dnf install -y wget

COPY go.mod .
COPY go.sum .

RUN GO_VERSION=$(sed -En 's/^go +(.*)$/\1/p' go.mod) && \
    wget https://dl.google.com/go/go${GO_VERSION}.${BUILDOS}-${BUILDARCH}.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.${BUILDOS}-${BUILDARCH}.tar.gz && \
    rm go${GO_VERSION}.${BUILDOS}-${BUILDARCH}.tar.gz

ENV PATH=/usr/local/go/bin:$PATH

RUN go mod download

COPY . .

RUN mkdir /workdir/bin
RUN go build -tags no_openssl -o /workdir/bin/ovs ./cmd/plugin
RUN go build -tags no_openssl -o /workdir/bin/marker ./cmd/marker
RUN go build -tags no_openssl -o /workdir/bin/ovs-mirror-producer ./cmd/mirror-producer
RUN go build -tags no_openssl -o /workdir/bin/ovs-mirror-consumer ./cmd/mirror-consumer

FROM registry.access.redhat.com/ubi9/ubi-minimal

RUN microdnf install -y findutils

COPY --from=builder /workdir/.version /.version
COPY --from=builder /workdir/bin/* /
