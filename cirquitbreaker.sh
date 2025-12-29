#!/bin/bash
# Watch Crossplane composition resourceRefs for changes with diff output
# Usage: ./watch-composition-refs.sh <resource-type> <name> [namespace]

set -euo pipefail

RESOURCE_TYPE="${1:-}"
NAME="${2:-}"
NAMESPACE="${3:-}"

if [[ -z "$RESOURCE_TYPE" || -z "$NAME" ]]; then
    echo "Usage: $0 <resource-type> <name> [namespace]"
    echo "Example (namespaced):     $0 pginstances.database.entigo.com rig-bff rig-bff"
    echo "Example (cluster-scoped): $0 zones.tenancy.entigo.com myzone"
    exit 1
fi

TMPDIR=$(mktemp -d)
PIDS=()

cleanup() {
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    rm -rf "$TMPDIR"
}
trap cleanup EXIT

# Build API resources lookup table: "KIND.APIGROUP" -> "resourcename.apigroup"
declare -A API_LOOKUP
while IFS= read -r line; do
    res_name=$(echo "$line" | awk '{print $1}')
    api_ver=$(echo "$line" | awk '{print $(NF-2)}')
    kind=$(echo "$line" | awk '{print $NF}')

    if [[ "$api_ver" == *"/"* ]]; then
        api_group="${api_ver%/*}"
    else
        api_group=""
    fi

    if [[ -n "$api_group" ]]; then
        API_LOOKUP["${kind}.${api_group}"]="${res_name}.${api_group}"
    else
        API_LOOKUP["${kind}."]="${res_name}"
    fi
done < <(kubectl api-resources --no-headers 2>/dev/null)

# Function to resolve Kind + apiVersion to kubectl resource name
resolve_resource() {
    local api="$1"
    local kind="$2"

    local api_group="${api%/*}"
    [[ "$api_group" == "$api" ]] && api_group=""

    local lookup_key="${kind}.${api_group}"

    if [[ -n "${API_LOOKUP[$lookup_key]:-}" ]]; then
        echo "${API_LOOKUP[$lookup_key]}"
    else
        if [[ -n "$api_group" ]]; then
            echo "${kind,,}.${api_group}"
        else
            echo "${kind,,}"
        fi
    fi
}

# Function to watch a single resource
watch_resource_poll() {
    local full_resource="$1"
    local rname="$2"
    local ns_flag="$3"
    local resource_id="$4"
    local tmpdir="$5"

    local prev_file="${tmpdir}/$(echo "$resource_id" | tr '/' '_')_prev.yaml"
    local curr_file="${tmpdir}/$(echo "$resource_id" | tr '/' '_')_curr.yaml"

    # Initial fetch
    if ! kubectl get "$full_resource" $ns_flag "$rname" -o yaml > "$prev_file" 2>/dev/null; then
        echo "Warning: Could not fetch $resource_id"
        return
    fi

    while true; do
        sleep 5
        if kubectl get "$full_resource" $ns_flag "$rname" -o yaml > "$curr_file" 2>/dev/null; then
            if ! diff -q "$prev_file" "$curr_file" > /dev/null 2>&1; then
                echo ""
                echo "====== $(date '+%H:%M:%S') CHANGE: $resource_id ======"
                diff --color=auto -u "$prev_file" "$curr_file" | tail -n +3 || true
                echo "=============================================="
                cp "$curr_file" "$prev_file"
            fi
        fi
    done
}

# Determine parent resource namespace flag and fetch resourceRefs
PARENT_NS_FLAG=""
REFS=""
if [[ -n "$NAMESPACE" ]]; then
    if kubectl get "$RESOURCE_TYPE" -n "$NAMESPACE" "$NAME" > /dev/null 2>&1; then
        PARENT_NS_FLAG="-n $NAMESPACE"
        REFS=$(kubectl get "$RESOURCE_TYPE" -n "$NAMESPACE" "$NAME" -o jsonpath='{range .spec.crossplane.resourceRefs[*]}{.apiVersion} {.kind} {.name} {.namespace}{"\n"}{end}' 2>/dev/null)
    fi
fi
if [[ -z "$PARENT_NS_FLAG" ]]; then
    if kubectl get "$RESOURCE_TYPE" "$NAME" > /dev/null 2>&1; then
        PARENT_NS_FLAG=""
        REFS=$(kubectl get "$RESOURCE_TYPE" "$NAME" -o jsonpath='{range .spec.crossplane.resourceRefs[*]}{.apiVersion} {.kind} {.name} {.namespace}{"\n"}{end}' 2>/dev/null)
    fi
fi

REF_COUNT=$(echo "$REFS" | grep -c . || echo 0)

echo "=== Watching parent + $REF_COUNT child resources for changes ==="
echo "  * PARENT: $RESOURCE_TYPE/$NAME"
echo "$REFS" | while read -r api kind name ns; do
    [[ -z "$api" ]] && continue
    api_group="${api%/*}"
    [[ "$api_group" == "$api" ]] && api_group="core"
    if [[ -n "$ns" ]]; then
        echo "  - $kind.$api_group/$name (ns: $ns)"
    else
        echo "  - $kind.$api_group/$name (cluster-scoped)"
    fi
done
echo "=== Press Ctrl+C to stop ==="
echo ""

# Export function and lookup table for subshells
export -f resolve_resource watch_resource_poll
export API_LOOKUP NAMESPACE TMPDIR

# Watch parent resource
watch_resource_poll "$RESOURCE_TYPE" "$NAME" "$PARENT_NS_FLAG" "PARENT:$RESOURCE_TYPE/$NAME" "$TMPDIR" &
PIDS+=($!)

# Watch all child resources
while read -r api kind rname ref_ns; do
    [[ -z "$api" ]] && continue

    api_group="${api%/*}"
    [[ "$api_group" == "$api" ]] && api_group="core"
    resource_id="${kind}.${api_group}/${rname}"

    full_resource=$(resolve_resource "$api" "$kind")

    # Determine namespace flag
    ns_flag=""
    if [[ -n "$ref_ns" ]]; then
        ns_flag="-n $ref_ns"
    elif [[ -n "$NAMESPACE" ]]; then
        ns_flag="-n $NAMESPACE"
    fi

    watch_resource_poll "$full_resource" "$rname" "$ns_flag" "$resource_id" "$TMPDIR" &
    PIDS+=($!)
done <<< "$REFS"

wait
