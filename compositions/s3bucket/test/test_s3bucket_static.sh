#!/bin/bash

load_mocks
init_test "s3bucket" "/workspace/functions/storage"
setup_resources --env --required

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
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