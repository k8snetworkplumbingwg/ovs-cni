FROM quay.io/centos/centos:stream9 as builder

RUN mkdir /workdir
WORKDIR /workdir

COPY go.mod .
RUN dnf install -y golang-$(sed -En 's/^go +(.*+)$/\1/p' go.mod).*

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

FROM registry.access.redhat.com/ubi8/ubi-minimal
RUN microdnf install findutils
COPY --from=builder /workdir/.version /.version
COPY --from=builder /workdir/bin/* /
