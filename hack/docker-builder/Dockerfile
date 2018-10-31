FROM fedora:28

RUN dnf -y install make git sudo gcc gradle rsync-daemon rsync protobuf-compiler && \
    dnf -y clean all

ENV GIMME_GO_VERSION=1.9

RUN mkdir -p /gimme && curl -sL https://raw.githubusercontent.com/travis-ci/gimme/master/gimme | HOME=/gimme bash >> /etc/profile.d/gimme.sh

ENV GOPATH="/go" GOBIN="/usr/bin"

ADD rsyncd.conf /etc/rsyncd.conf

RUN \
    mkdir -p /go && \
    source /etc/profile.d/gimme.sh && \
    go get -u github.com/onsi/ginkgo/ginkgo && \
    go get -u -d github.com/golang/protobuf/protoc-gen-go && \
    cd /go/src/github.com/golang/protobuf/protoc-gen-go && \
    git checkout 1643683e1b54a9e88ad26d98f81400c8c9d9f4f9 && \
    go install

ADD entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
