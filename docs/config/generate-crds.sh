#!/bin/bash
set -e

rm -rf crds
mkdir crds

cp ../../compositions/cronjob/apis/cronjob-definition.yaml crds/
cp ../../compositions/postgresql/apis/instance-definition.yaml crds/
cp ../../compositions/repository/apis/repository-definition.yaml crds/
cp ../../compositions/webaccess/apis/webaccess-definition.yaml crds/
cp ../../compositions/webapp/apis/webapp-definition.yaml crds/
cp ../../compositions/zone/apis/zone-definition.yaml crds/
cp ../../compositions/zone/apis/s3bucket-definition.yaml crds/

for i in `find crds/ -type f`; do 

# use lowercase names
API_KIND=`cat $i | yq '.spec.names.kind'`
API_GROUP=`cat $i | yq '.spec.group'`
TIERS=`cat ../config/tiers.yaml | yq ".${API_KIND} | join(\", \")"`

yq e -i '.apiVersion = "apiextensions.k8s.io/v1"' $i
yq e -i '.kind = "CustomResourceDefinition"' $i

cat ../config/crdoc.yaml | \
    yq ".groups.[0].group = \"${API_GROUP}\""  | \
    yq ".groups.[0].kinds.[0].name = \"${API_KIND}\"" | \
    yq ".metadata.title = \"${API_KIND}\"" | \
    yq ".metadata.description = \"${API_KIND}\"" > crdoc.yaml

MD_FILE="../api/${API_KIND}.md"
./crdoc --resources $i --output $MD_FILE --toc crdoc.yaml -t ../config/api-reference.tmpl

# insert tiers section right after the title
awk -v tiers="$TIERS" '
  /^# / && !inserted {
    print $0
    print ""
    print ":::note Tiers"
    print "This feature is available for the following tiers: **" tiers "**."
    print ":::"
    print ""
    inserted=1
    next
  }
  { print }
' "$MD_FILE" > "$MD_FILE.tmp" && mv "$MD_FILE.tmp" "$MD_FILE"

  # Insert Examples into the existing Examples Tab (if examples exist)
  EXAMPLES_FILE="../examples/${API_KIND}.md"

  if [ -f "$EXAMPLES_FILE" ]; then
    echo "Adding examples for ${API_KIND} from ${EXAMPLES_FILE}"

    awk -v exfile="$EXAMPLES_FILE" '
      function slugify(s) {
        gsub(/[^A-Za-z0-9]+/, "-", s)
        gsub(/^-+|-+$/, "", s)
        return tolower(s)
      }
      BEGIN { inserted = 0 }
      {
        print $0
        if (!inserted && $0 ~ /<TabItem[[:space:]]+value="examples"[[:space:]]+label="Examples">/) {
          while ((getline line < exfile) > 0) {
            if (line ~ /^### /) {
              title = substr(line,5)         # strip leading "### "
              print "### " title " {#example-" slugify(title) "}"
            } else {
              print line
            }
          }
          close(exfile)
          inserted = 1
        }
      }
    ' "$MD_FILE" > "$MD_FILE.tmp" && mv "$MD_FILE.tmp" "$MD_FILE"

  else
    echo "No examples file found for ${API_KIND} (expected ${EXAMPLES_FILE}), removing Examples tab."

    awk '
      BEGIN { skip=0 }
      /<TabItem[[:space:]]+value="examples"[[:space:]]+label="Examples">/ { skip=1; next }
      /<\/TabItem>/ { if (skip) { skip=0; next } }
      { if (!skip) print }
    ' "$MD_FILE" > "$MD_FILE.tmp" && mv "$MD_FILE.tmp" "$MD_FILE"
  fi

done

echo "✅ CRD Markdown generation complete"