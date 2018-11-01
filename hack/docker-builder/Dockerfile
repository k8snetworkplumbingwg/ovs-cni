FROM fedora:28

RUN dnf -y install make git sudo gcc gradle rsync-daemon rsync && \
    dnf -y clean all

ENV GIMME_GO_VERSION=1.9

RUN mkdir -p /gimme && curl -sL https://raw.githubusercontent.com/travis-ci/gimme/master/gimme | HOME=/gimme bash >> /etc/profile.d/gimme.sh

ENV GOPATH="/go" GOBIN="/usr/bin"

ADD rsyncd.conf /etc/rsyncd.conf

RUN \
    mkdir -p /go && \
    source /etc/profile.d/gimme.sh && \
    go get -u github.com/onsi/ginkgo/ginkgo

ADD entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
