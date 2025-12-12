#!/bin/bash

yq eval '.functions[] | .image + ":" + .tag' helm/values.yaml | while read SOURCE
do

    DEST=$(echo $SOURCE | sed 's|xpkg.upbound.io/crossplane-contrib/|entigolabs/|g')
    if docker manifest inspect $DEST > /dev/null 2>&1; then
        echo "Skipping - already exists at $DEST"
        continue
    fi
    echo "Copying $SOURCE to $DEST"
    docker buildx imagetools create --tag $DEST $SOURCE
    
    if [ $? -ne 0 ]; then
        echo "Failed to copy $SOURCE to $DEST"
        exit 2
    fi
done

echo "All functions copied successfully"


