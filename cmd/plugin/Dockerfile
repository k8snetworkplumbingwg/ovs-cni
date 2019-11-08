FROM registry.access.redhat.com/ubi8/ubi-minimal
COPY plugin /ovs
COPY .version /.version
CMD ["sh", "-c", "cp /ovs /host/opt/cni/bin/ovs && sleep infinity"]
