ARG KIND_NODE_IMAGE=docker.io/kindest/node:v1.34.0
FROM ${KIND_NODE_IMAGE}

RUN apt-get update && \
    apt-get install -y --no-install-recommends openvswitch-switch && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
