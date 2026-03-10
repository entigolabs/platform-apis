#!/bin/bash
# Watch a ValidatingWebhookConfiguration or MutatingWebhookConfiguration
# and show unified diff per webhook entry when the resource changes.
#
# Usage:
#   ./webhook-changes.sh <type> <name>
#
# Examples:
#   ./webhook-changes.sh validatingwebhookconfigurations.admissionregistration.k8s.io kyverno-resource-validating-webhook-cfg
#   ./webhook-changes.sh mutatingwebhookconfigurations.admissionregistration.k8s.io kyverno-resource-mutating-webhook-cfg
#
# Requires: kubectl, yq (Mike Farah's Go version >= v4)

set -euo pipefail

if [ $# -ne 2 ]; then
    echo "Usage: $0 <webhook-type> <webhook-name>"
    exit 1
fi

WEBHOOK_TYPE="$1"
WEBHOOK_NAME="$2"

for cmd in kubectl yq md5sum; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "Error: '$cmd' not found" >&2
        exit 1
    fi
done

WORK=$(mktemp -d "/tmp/webhook-watcher.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

PREV_DIR="$WORK/prev"
CURR_DIR="$WORK/curr"
DOC_FILE="$WORK/doc.yaml"
mkdir -p "$PREV_DIR" "$CURR_DIR"

PREV_RV=""
IS_FIRST=true

# --- Helper: run yq on first document only ---
yq1() {
    yq 'select(di == 0)' "$2" | yq "$1"
}

# --- Hash a webhook name into a safe filename ---
safe_name() {
    echo -n "$1" | md5sum | cut -d' ' -f1
}

# --- Split YAML into per-webhook files ---
split_webhooks() {
    local docfile="$1"
    local dest="$2"

    rm -f "$dest"/*.yaml "$dest"/order.txt "$dest"/rv.txt

    yq1 '.metadata.resourceVersion' "$docfile" > "$dest/rv.txt"

    local count
    count=$(yq1 '.webhooks | length' "$docfile")
    if ! [[ "$count" =~ ^[0-9]+$ ]] || [ "$count" -eq 0 ]; then
        return
    fi

    for ((i=0; i<count; i++)); do
        local wh_name hash
        wh_name=$(yq1 ".webhooks[$i].name" "$docfile")
        hash=$(safe_name "$wh_name")
        yq1 ".webhooks[$i]" "$docfile" > "$dest/${hash}.yaml"
        echo "${hash} ${wh_name}" >> "$dest/order.txt"
    done
}

# --- Show side-by-side webhook order ---
show_order() {
    echo ""
    echo "--- webhook order ---"

    local prev_names="" curr_names=""
    [ -f "$PREV_DIR/order.txt" ] && prev_names=$(awk '{print $2}' "$PREV_DIR/order.txt")
    [ -f "$CURR_DIR/order.txt" ] && curr_names=$(awk '{print $2}' "$CURR_DIR/order.txt")

    if [ "$prev_names" = "$curr_names" ]; then
        # No order change, just print current
        local i=1
        while IFS= read -r name; do
            printf "  %2d  %s\n" "$i" "$name"
            ((i++))
        done <<< "$curr_names"
        echo "(order unchanged)"
        return
    fi

    echo "ORDER CHANGED:"
    # Build side-by-side: prev | curr
    local prev_arr=() curr_arr=()
    while IFS= read -r name; do
        [ -n "$name" ] && prev_arr+=("$name")
    done <<< "$prev_names"
    while IFS= read -r name; do
        [ -n "$name" ] && curr_arr+=("$name")
    done <<< "$curr_names"

    local max=${#curr_arr[@]}
    [ ${#prev_arr[@]} -gt "$max" ] && max=${#prev_arr[@]}

    # Find longest name for column width
    local col_w=40
    printf "  %-4s %-${col_w}s  |  %-4s %s\n" "#" "PREVIOUS" "#" "CURRENT"
    printf "  %-4s %-${col_w}s  |  %-4s %s\n" "----" "$(printf '%0.s-' $(seq 1 $col_w))" "----" "$(printf '%0.s-' $(seq 1 $col_w))"

    for ((i=0; i<max; i++)); do
        local pn="${prev_arr[$i]:-}"
        local cn="${curr_arr[$i]:-}"
        local marker=" "
        [ "$pn" != "$cn" ] && marker="*"
        printf "%s %-4s %-${col_w}s  |  %-4s %s\n" "$marker" "$((i+1))" "$pn" "$((i+1))" "$cn"
    done
}

# --- Show diffs ---
show_diffs() {
    local prev_rv curr_rv
    prev_rv=$(cat "$PREV_DIR/rv.txt" 2>/dev/null || echo "none")
    curr_rv=$(cat "$CURR_DIR/rv.txt")

    echo ""
    echo "========================================================================"
    echo "  resourceVersion: $prev_rv -> $curr_rv  ($(date '+%Y-%m-%d %H:%M:%S'))"
    echo "========================================================================"

    # Merge order files, deduplicate by hash
    local all_entries=""
    [ -f "$PREV_DIR/order.txt" ] && all_entries=$(cat "$PREV_DIR/order.txt")
    if [ -f "$CURR_DIR/order.txt" ]; then
        if [ -n "$all_entries" ]; then
            all_entries=$(printf '%s\n%s' "$all_entries" "$(cat "$CURR_DIR/order.txt")")
        else
            all_entries=$(cat "$CURR_DIR/order.txt")
        fi
    fi
    all_entries=$(echo "$all_entries" | sort -u -k1,1)

    if [ -z "$all_entries" ]; then
        echo "(no webhooks found)"
        return
    fi

    while IFS=' ' read -r hash wh_name; do
        [ -z "$hash" ] && continue

        local left="$PREV_DIR/${hash}.yaml"
        local right="$CURR_DIR/${hash}.yaml"
        [ -f "$left" ] || left="/dev/null"
        [ -f "$right" ] || right="/dev/null"

        local diff_output
        diff_output=$(diff -u "$left" "$right" \
            --label "prev: $wh_name" \
            --label "curr: $wh_name" 2>/dev/null) || true

        if [ -n "$diff_output" ]; then
            echo ""
            echo "--- webhook: $wh_name ---"
            echo "$diff_output"
        else
            echo ""
            echo "--- webhook: $wh_name --- (no change)"
        fi
    done <<< "$all_entries"

    show_order
    echo ""
}

# --- Process a buffered YAML document ---
process_doc() {
    local doc="$1"

    # Write to file for yq
    printf '%s\n' "$doc" > "$DOC_FILE"

    # Swap prev <- curr
    rm -f "$PREV_DIR"/*.yaml "$PREV_DIR"/*.txt
    for f in "$CURR_DIR"/*.yaml "$CURR_DIR"/*.txt; do
        [ -f "$f" ] && cp "$f" "$PREV_DIR/"
    done

    # Split new doc
    split_webhooks "$DOC_FILE" "$CURR_DIR"

    local curr_rv
    curr_rv=$(cat "$CURR_DIR/rv.txt")

    if [ "$curr_rv" != "$PREV_RV" ]; then
        if $IS_FIRST; then
            echo ""
            echo "========================================================================"
            echo "  Initial state - resourceVersion: $curr_rv  ($(date '+%Y-%m-%d %H:%M:%S'))"
            echo "========================================================================"
            if [ -f "$CURR_DIR/order.txt" ]; then
                echo ""
                echo "Webhooks:"
                awk '{printf "  %2d  %s\n", NR, $2}' "$CURR_DIR/order.txt"
            fi
            echo ""
            echo "Waiting for changes..."
            IS_FIRST=false
        else
            show_diffs
        fi
        PREV_RV="$curr_rv"
    fi
}

# --- Main ---
echo "Watching $WEBHOOK_TYPE/$WEBHOOK_NAME ..."
echo "Press Ctrl+C to stop."

kubectl get "$WEBHOOK_TYPE" "$WEBHOOK_NAME" -o yaml -w | {
    doc=""
    while IFS= read -r line; do
        if [[ "$line" == "apiVersion:"* ]] && [ -n "$doc" ]; then
            process_doc "$doc"
            doc="$line"
        else
            if [ -z "$doc" ]; then
                doc="$line"
            else
                doc="$doc"$'\n'"$line"
            fi
        fi
    done

    if [ -n "$doc" ]; then
        process_doc "$doc"
    fi
}
