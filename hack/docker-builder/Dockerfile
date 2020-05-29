FROM fedora:31

RUN dnf -y install make git sudo gcc rsync-daemon rsync openvswitch hostname && \
    dnf -y clean all

ENV GIMME_GO_VERSION=1.13

RUN mkdir -p /gimme && curl -sL https://raw.githubusercontent.com/travis-ci/gimme/master/gimme | HOME=/gimme bash >> /etc/profile.d/gimme.sh

ENV GOPATH="/go" GOBIN="/usr/bin"

ADD rsyncd.conf /etc/rsyncd.conf

RUN mkdir -p /opt/cni/bin

ENV PATH="/opt/cni/bin:$PATH"

ADD https://github.com/containernetworking/plugins/releases/download/v0.8.5/cni-plugins-linux-amd64-v0.8.5.tgz /opt/cni/bin

RUN \
    cd  /opt/cni/bin && \
    tar -xzf cni-plugins-linux-amd64-v0.8.5.tgz && \
    rm -f cni-plugins-linux-amd64-v0.8.5.tgz

RUN \
    mkdir -p /go && \
    source /etc/profile.d/gimme.sh && \
    go get -u github.com/onsi/ginkgo/ginkgo

ADD entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
