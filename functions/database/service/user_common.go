package service

import (
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
)

func GetGrantReadyStatus(observed *composed.Unstructured) resource.Ready {
	if isResourceReady(observed) {
		return resource.ReadyTrue
	}
	return resource.ReadyFalse
}
