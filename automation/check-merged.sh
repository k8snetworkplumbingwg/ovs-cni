#!/bin/bash -xe

quay_login() {
    (
        set +x
        docker login \
            -u "$QUAY_REGISTRY_USERNAME" \
            -p "$QUAY_REGISTRY_PASSWORD" \
            quay.io
    )
}

main() {
    quay_login
    export IMAGE_TAG=`./hack/get_tag.sh`
    make docker-build
    make docker-push
}

# We use Travis to deploy built images
# [[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
