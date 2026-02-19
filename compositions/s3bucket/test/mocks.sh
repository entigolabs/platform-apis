source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  mock_ready "Bucket" "BucketVersioning" "BucketServerSideEncryptionConfiguration" "BucketPublicAccessBlock" "BucketOwnershipControls" "User" "Policy" "UserPolicyAttachment" "AccessKey" "Role" "RolePolicyAttachment" "Secret" "SecretVersion" "ServiceAccount"
}
