#!/bin/bash
# Watch Crossplane composition resourceRefs for changes with diff output
# Usage: ./watch-composition-refs.sh <resource-type> <n> [namespace]

set -euo pipefail

RESOURCE_TYPE="${1:-}"
NAME="${2:-}"
NAMESPACE="${3:-}"

if [[ -z "$RESOURCE_TYPE" || -z "$NAME" ]]; then
    echo "Usage: $0 <resource-type> <n> [namespace]"
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
    # Parse: NAME SHORTNAMES APIVERSION NAMESPACED KIND
    res_name=$(echo "$line" | awk '{print $1}')
    api_ver=$(echo "$line" | awk '{print $(NF-2)}')
    kind=$(echo "$line" | awk '{print $NF}')

    # Extract API group from version (e.g., "rbac.authorization.k8s.io/v1" -> "rbac.authorization.k8s.io")
    if [[ "$api_ver" == *"/"* ]]; then
        api_group="${api_ver%/*}"
    else
        api_group=""  # Core API (v1)
    fi

    # Build lookup key and value
    if [[ -n "$api_group" ]]; then
        API_LOOKUP["${kind}.${api_group}"]="${res_name}.${api_group}"
    else
        API_LOOKUP["${kind}."]="${res_name}"
    fi
done < <(kubectl api-resources --no-headers 2>/dev/null)

# Function to resolve Kind + apiVersion to kubectl resource name
resolve_resource() {
    local api="$1"  # e.g., "rbac.authorization.k8s.io/v1"
    local kind="$2" # e.g., "Role"

    local api_group="${api%/*}"
    [[ "$api_group" == "$api" ]] && api_group=""  # Core API

    local lookup_key="${kind}.${api_group}"

    if [[ -n "${API_LOOKUP[$lookup_key]:-}" ]]; then
        echo "${API_LOOKUP[$lookup_key]}"
    else
        # Fallback: lowercase kind with api group
        if [[ -n "$api_group" ]]; then
            echo "${kind,,}.${api_group}"
        else
            echo "${kind,,}"
        fi
    fi
}

# Fetch resourceRefs from the composition (try namespaced, then cluster-scoped)
# Format: apiVersion kind name namespace (namespace may be empty)
REFS=""
if [[ -n "$NAMESPACE" ]]; then
    REFS=$(kubectl get "$RESOURCE_TYPE" -n "$NAMESPACE" "$NAME" -o jsonpath='{range .spec.crossplane.resourceRefs[*]}{.apiVersion} {.kind} {.name} {.namespace}{"\n"}{end}' 2>/dev/null)
fi
if [[ -z "$REFS" ]]; then
    REFS=$(kubectl get "$RESOURCE_TYPE" "$NAME" -o jsonpath='{range .spec.crossplane.resourceRefs[*]}{.apiVersion} {.kind} {.name} {.namespace}{"\n"}{end}' 2>/dev/null)
fi

if [[ -z "$REFS" ]]; then
    echo "No resourceRefs found in spec.crossplane.resourceRefs"
    exit 1
fi

echo "=== Watching $(echo "$REFS" | wc -l) resources for changes ==="
echo "$REFS" | while read -r api kind name ns; do
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

# Function to watch a single resource
watch_resource_poll() {
    local api="$1"
    local kind="$2"
    local rname="$3"
    local ref_ns="$4"
    local fallback_ns="$5"
    local tmpdir="$6"

    # Extract API group for resource identification
    local api_group="${api%/*}"
    [[ "$api_group" == "$api" ]] && api_group="core"
    local resource_id="${kind}.${api_group}/${rname}"
    local prev_file="${tmpdir}/${kind}_${api_group}_${rname}_prev.yaml"
    local curr_file="${tmpdir}/${kind}_${api_group}_${rname}_curr.yaml"

    # Resolve correct kubectl resource name
    local full_resource
    full_resource=$(resolve_resource "$api" "$kind")

    # Determine namespace: prefer resourceRef namespace, then fallback, then cluster-scoped
    local ns_flag=""
    if [[ -n "$ref_ns" ]] && kubectl get "$full_resource" -n "$ref_ns" "$rname" -o yaml > "$prev_file" 2>/dev/null; then
        ns_flag="-n $ref_ns"
    elif [[ -n "$fallback_ns" ]] && kubectl get "$full_resource" -n "$fallback_ns" "$rname" -o yaml > "$prev_file" 2>/dev/null; then
        ns_flag="-n $fallback_ns"
    elif kubectl get "$full_resource" "$rname" -o yaml > "$prev_file" 2>/dev/null; then
        ns_flag=""
    else
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

# Export function and lookup table for subshells
export -f resolve_resource watch_resource_poll
export API_LOOKUP NAMESPACE TMPDIR

# Start watching all resources in background
while read -r api kind rname ref_ns; do
    [[ -z "$api" ]] && continue
    watch_resource_poll "$api" "$kind" "$rname" "$ref_ns" "$NAMESPACE" "$TMPDIR" &
    PIDS+=($!)
done <<< "$REFS"

# Wait for Ctrl+C
wait
