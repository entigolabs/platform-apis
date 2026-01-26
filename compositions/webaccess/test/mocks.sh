source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  mock_ready "VirtualService" "ServiceEntry" "DestinationRule"
}
