FROM centos:centos7
RUN yum install -y openvswitch && yum clean all
COPY marker /marker
COPY .version /.version
ENTRYPOINT [ "./marker", "-v", "3", "-logtostderr"]