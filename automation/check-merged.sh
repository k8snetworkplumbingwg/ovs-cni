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
    echo "TODO: Push image to a registry"
}

[[ "${BASH_SOURCE[0]}" == "$0" ]] && main "$@"
