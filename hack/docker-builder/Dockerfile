FROM quay.io/fedora/fedora:32-x86_64

RUN dnf -y install make git sudo gcc rsync-daemon rsync openvswitch hostname && \
    dnf -y clean all

ENV GOPATH="/go"
RUN \
    DESTINATION=/opt && \
    VERSION=1.16.7 && \
    TARBALL=go${VERSION}.linux-amd64.tar.gz && \
    URL=https://dl.google.com/go && \
    mkdir -p ${DESTINATION} && \
    curl -L ${URL}/${TARBALL} -o ${DESTINATION}/${TARBALL} && \
    tar -xf ${DESTINATION}/${TARBALL} -C ${DESTINATION}
ENV PATH="/opt/go/bin:$PATH"

ADD rsyncd.conf /etc/rsyncd.conf

RUN mkdir -p /opt/cni/bin

ENV PATH="/opt/cni/bin:$PATH"

ADD https://github.com/containernetworking/plugins/releases/download/v0.8.5/cni-plugins-linux-amd64-v0.8.5.tgz /opt/cni/bin

RUN \
    cd  /opt/cni/bin && \
    tar -xzf cni-plugins-linux-amd64-v0.8.5.tgz && \
    rm -f cni-plugins-linux-amd64-v0.8.5.tgz

RUN go get -u github.com/onsi/ginkgo/ginkgo

ADD entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]
