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

done
