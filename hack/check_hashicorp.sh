#!/bin/bash

allowed_hashicorp_modules=(
    "github.com/hashicorp/errwrap"
    "github.com/hashicorp/go-multierror"
    "github.com/hashicorp/hcl"
)

error_found=false
while read -r line; do
    if ! [[ " ${allowed_hashicorp_modules[*]} " == *" $line "* ]]; then
        echo "found non allowlisted hashicorp module: $line"
        error_found=true
    fi
done < <(grep -i hashicorp go.mod | grep -o 'github.com/[^ ]*')

if [[ $error_found == true ]]; then
    echo "Non allowlisted hashicorp modules found, exiting with an error."
    echo "HashiCorp adapted BSL, which we cant use on our projects."
    echo "Please review the licensing, and either add it to the list if it isn't BSL,"
    echo "or use a different library."
    exit 1
fi
echo "All included hashicorp modules are allowlisted"
