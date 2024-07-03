FROM quay.io/centos/centos:stream9 as builder

RUN mkdir /workdir
WORKDIR /workdir

COPY go.mod .

RUN dnf install -y wget

RUN GO_VERSION=$(sed -En 's/^go +(.*)$/\1/p' go.mod) && \
    wget https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && \
    rm go${GO_VERSION}.linux-amd64.tar.gz

ENV PATH /usr/local/go/bin:$PATH

COPY . .

ENV GOOS linux
# Support overriding target GOARCH during `make docker-build`
ARG goarch=amd64
ENV GOARCH=$goarch
ENV CGO_ENABLED 0
ENV GOFLAGS -mod=vendor

RUN mkdir /workdir/bin
RUN go build -tags no_openssl -o /workdir/bin/ovs ./cmd/plugin
RUN go build -tags no_openssl -o /workdir/bin/marker ./cmd/marker
RUN go build -tags no_openssl -o /workdir/bin/ovs-mirror-producer ./cmd/mirror-producer
RUN go build -tags no_openssl -o /workdir/bin/ovs-mirror-consumer ./cmd/mirror-consumer

FROM registry.access.redhat.com/ubi9/ubi-minimal
RUN microdnf install -y findutils
COPY --from=builder /workdir/.version /.version
COPY --from=builder /workdir/bin/* /
