FROM registry.access.redhat.com/ubi8/ubi-minimal
RUN microdnf install findutils
COPY marker/marker /marker
COPY plugin/plugin /ovs
COPY mirror-consumer/mirror-consumer /ovs-mirror-consumer
COPY mirror-producer/mirror-producer /ovs-mirror-producer
COPY .version /.version
