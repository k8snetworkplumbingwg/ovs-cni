FROM centos:centos7
COPY plugin /ovs
COPY .version /.version
CMD ["sh", "-c", "cp /ovs /host/opt/cni/bin/ovs && sleep infinity"]
