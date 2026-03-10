#!/bin/bash
# cleanup-compositionrevisions.sh
set -euo pipefail

# Get compositionRevisionRef from both possible paths
IN_USE=$(
  {
    # Standard path: spec.compositionRevisionRef.name
    kubectl get composite --all-namespaces -o jsonpath='{range .items[*]}{.spec.compositionRevisionRef.name}{"\n"}{end}' 2>/dev/null
    # Nested path: spec.crossplane.compositionRevisionRef.name
    kubectl get composite --all-namespaces -o jsonpath='{range .items[*]}{.spec.crossplane.compositionRevisionRef.name}{"\n"}{end}' 2>/dev/null
  } | grep -v '^$' | sort -u
)

# Get latest revision for each composition (highest spec.revision number)
LATEST=$(kubectl get compositionrevision -o json | jq -r '
  .items
  | group_by(.metadata.labels["crossplane.io/composition-name"])
  | map(sort_by(.spec.revision) | last | .metadata.name)
  | .[]
')

# Combine in-use and latest
KEEP=$(echo -e "${IN_USE}\n${LATEST}" | grep -v '^$' | sort -u)

echo "=== Revisions in use ==="
echo "$IN_USE"
echo ""
echo "=== Latest revisions (per composition) ==="
echo "$LATEST"
echo ""

ALL_REVS=$(kubectl get compositionrevision -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | grep -v '^$')

DELETED=0
SKIPPED=0

for rev in $ALL_REVS; do
  if echo "$KEEP" | grep -Fxq "$rev"; then
    echo "KEEP: $rev"
    SKIPPED=$((SKIPPED + 1))
  else
    echo "DELETE: $rev"
    kubectl delete compositionrevision "$rev"
    DELETED=$((DELETED + 1))
  fi
done

echo ""
echo "Summary: $DELETED deleted, $SKIPPED kept"
