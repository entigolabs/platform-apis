

echo "TEST 5: rendering step 5 resources (BucketVersioning, BucketOwnershipControls)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "BucketVersioning" 1 "BucketOwnershipControls" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 6: rendering step 6 resources (BucketPublicAccessBlock, BucketServerSideEncryptionConfiguration)"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "BucketPublicAccessBlock" 1 "BucketServerSideEncryptionConfiguration" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 7: Checking S3Bucket Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "S3Bucket"
