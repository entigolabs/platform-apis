#!/bin/bash

load_mocks

INPUT="../examples/s3bucket.yaml"
COMPOSITION="../apis/s3bucket-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"
mock_environment "$ENV_CONFIG"

TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "User" 1 \
                         "Secret" 1 \
                         "SecretVersion" 1 \
                         "UserPolicyAttachment" 1 \
                         "RolePolicyAttachment" 1 \
                         "Role" 1 \
                         "Policy" 1 \
                         "AccessKey" 1 \
                         "BucketVersioning" 1 \
                         "BucketServerSideEncryptionConfiguration" 1 \
                         "BucketPublicAccessBlock" 1 \
                         "BucketOwnershipControls" 1 \
                         "Bucket" 1