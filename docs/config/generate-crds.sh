#!/bin/bash
set -e

rm -rf crds
mkdir crds

cp ../../compositions/postgresql/apis/instance-definition.yaml crds/

for i in `find crds/ -type f`; do 

# use lowercase names
API_KIND=`cat $i | yq '.spec.names.kind'`
API_GROUP=`cat $i | yq '.spec.group'`

yq e -i '.apiVersion = "apiextensions.k8s.io/v1"' $i
yq e -i '.kind = "CustomResourceDefinition"' $i

cat ../config/crdoc.yaml | \
    yq ".groups.[0].group = \"${API_GROUP}\""  | \
    yq ".groups.[0].kinds.[0].name = \"${API_KIND}\"" | \
    yq ".metadata.title = \"${API_KIND}\"" | \
    yq ".metadata.description = \"${API_KIND}\"" > crdoc.yaml

./crdoc  --resources $i --output ../api/${API_KIND}.md --toc crdoc.yaml -t ../config/api-reference.tmpl

  # Insert Examples into the existing Examples Tab (if examples exist)
  EXAMPLES_FILE="../../compositions/postgresql/examples/instance-definition.yaml"
  MD_FILE="../api/${API_KIND}.md"

  if [ -f "$EXAMPLES_FILE" ]; then
    echo "Adding examples for ${API_KIND} from ${EXAMPLES_FILE}"

    TMP_EXAMPLES=$(mktemp)
    trap 'rm -f "$TMP_EXAMPLES"' EXIT

    yq -r '.examples[] | "### \(.title) {#example-\(.title)}\n\(.description)\n```yaml\n\(.code)\n```"' "$EXAMPLES_FILE" > "$TMP_EXAMPLES"
    awk -v exfile="$TMP_EXAMPLES" '
    BEGIN { inserted = 0 }
    {
      print $0
      if (!inserted && $0 ~ /<TabItem[[:space:]]+value="examples"[[:space:]]+label="Examples">/) {
        while ((getline line < exfile) > 0) print line
        close(exfile)
        inserted = 1
      }
    }
    ' "$MD_FILE" > "$MD_FILE.tmp" && mv "$MD_FILE.tmp" "$MD_FILE"

    # cleanup handled by trap
    trap - EXIT
    rm -f "$TMP_EXAMPLES"
  else
    echo "No examples file found for ${API_KIND} (expected ${EXAMPLES_FILE}), skipping."
  fi

done

echo "✅ CRD Markdown generation complete"