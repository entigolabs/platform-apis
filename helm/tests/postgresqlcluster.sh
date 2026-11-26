#!/bin/bash
# Checks how the chart renders PostgreSQLCluster: the availability domains infralib passes, the
# admission rules, and the install guards. Run from the chart directory.
set -euo pipefail

fail=0
render() {
    helm template t . --kube-version "${KUBE_VERSION:-1.36.0}" \
        --set postgresqlcluster.install=true \
        --set 'postgresqlcluster.backends[0]=PostgreSQLOCIConfig.oci.database.entigo.com' "$@"
}
expect() {
    local name=$1 pattern=$2 output=$3
    if [[ $output =~ $pattern ]]; then
        echo "ok: $name"
    else
        echo "FAIL: $name"
        fail=1
    fi
}

values=$(mktemp)
trap 'rm -f "$values"' EXIT

# infralib renders a Terraform list as one string with every element quoted.
printf 'database:\n  environmentConfig:\n    oci:\n      availabilityDomains: %s\n' \
    "'\"x:AD-1\",\"x:AD-2\"'" > "$values"
expect "quoted string becomes a list" \
    $'availabilityDomains:\n +- x:AD-1\n +- x:AD-2\n' "$(render -f "$values")"

printf 'database:\n  environmentConfig:\n    oci:\n      availabilityDomains: [x:AD-1]\n' > "$values"
expect "list passes through" $'availabilityDomains:\n +- x:AD-1\n' "$(render -f "$values")"

expect "unset becomes an empty list" 'availabilityDomains: \[\]' "$(render)"
expect "version changes refused" 'version cannot change yet' "$(render)"
expect "protected data deletion refused except by Crossplane and the garbage collector" \
    $'database.entigo.com/deletion-protection: "true"[^-]*(.*\n)+.*crossplane-system:(.*\n)+.*generic-garbage-collector' "$(render)"

if KUBE_VERSION=1.35.0 render >/dev/null 2>&1; then
    echo "FAIL: Kubernetes below 1.36 refused"
    fail=1
else
    echo "ok: Kubernetes below 1.36 refused"
fi
if helm template t . --kube-version 1.36.0 --set postgresqlcluster.install=true >/dev/null 2>&1; then
    echo "FAIL: no backend refused"
    fail=1
else
    echo "ok: no backend refused"
fi

exit "$fail"
