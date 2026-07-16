package service

import (
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
)

const CrossplaneProtectionApi = "protection.crossplane.io/v1beta1"

func GetDatabaseReadyStatus(observed *composed.Unstructured) resource.Ready {
	if isResourceReady(observed) {
		return resource.ReadyTrue
	}
	return resource.ReadyFalse
}
