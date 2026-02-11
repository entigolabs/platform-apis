#!/bin/bash

load_mocks
init_test "s3bucket" "/workspace/functions/storage"
setup_resources --env --required

echo "TEST 1: rendering step 1 resources (Bucket, User, Role)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Bucket" 1 "User" 1 "Role" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 2: rendering step 2 resources (BucketVersioning, AccessKey, Policy)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "BucketVersioning" 1 "AccessKey" 1 "Policy" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 3: rendering step 3 resources (BucketOwnershipControls, UserPolicyAttachment, RolePolicyAttachment)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "BucketOwnershipControls" 1 "UserPolicyAttachment" 1 "RolePolicyAttachment" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 4: rendering step 4 resources (BucketPublicAccessBlock, Secret)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "BucketPublicAccessBlock" 1 "Secret" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 5: rendering step 5 resources (BucketServerSideEncryptionConfiguration, SecretVersion)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "BucketServerSideEncryptionConfiguration" 1 "SecretVersion" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 6: Checking S3Bucket Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "S3Bucket"
