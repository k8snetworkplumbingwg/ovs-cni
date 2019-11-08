FROM fedora:31
RUN dnf install -y openvswitch && dnf clean all
COPY plugin /ovs
COPY .version /.version
CMD ["sh", "-c", "cp /ovs /host/opt/cni/bin/ovs && sleep infinity"]
