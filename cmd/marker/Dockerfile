FROM registry.access.redhat.com/ubi8/ubi-minimal
RUN microdnf install findutils
COPY marker /marker
COPY .version /.version
ENTRYPOINT [ "./marker", "-v", "3", "-logtostderr"]
