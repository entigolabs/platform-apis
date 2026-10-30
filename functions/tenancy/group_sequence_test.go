package main

import (
	"regexp"
	"testing"

	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSecretStoreKeysAreNotSequenced(t *testing.T) {
	zone := &v1alpha1.Zone{TypeMeta: metav1.TypeMeta{Kind: apis.XRKindZone}}
	sequence := (&GroupImpl{}).GetSequence(zone)
	if len(sequence.Steps) == 0 {
		t.Fatal("expected a Zone sequence")
	}
	keys := []string{
		service.GetSecretStoreRoleKey("teama"),
		service.GetSecretStoreRolePolicyKey("teama"),
		service.GetSecretStoreServiceAccountKey("teama"),
		service.GetSecretStoreKey("teama"),
	}
	for _, step := range sequence.Steps {
		for _, pattern := range step.Objects {
			regex := regexp.MustCompile("^" + pattern + "$")
			for _, key := range keys {
				if regex.MatchString(key) {
					t.Errorf("secret store key %s matches sequence pattern %s", key, pattern)
				}
			}
		}
	}
}
