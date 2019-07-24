FROM fedora:29
RUN dnf install -y openvswitch && dnf clean all
COPY marker /marker
COPY .version /.version
ENTRYPOINT [ "./marker", "-v", "3", "-logtostderr"]
