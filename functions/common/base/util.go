package base

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/go-logr/zapr"
	"github.com/mozillazg/go-unidecode"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func GenerateFNVHash(uid types.UID) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(uid))
	return strings.ToLower(fmt.Sprintf("%x", hasher.Sum32()))
}

func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	m, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: m}
	return u, nil
}

func ExtractRequiredResource(requiredResources map[string][]resource.Required, key string, target runtime.Object) error {
	if requiredResources == nil || len(requiredResources[key]) == 0 {
		return errors.Errorf("%s not found in required resources", key)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(requiredResources[key][0].Resource.Object, target); err != nil {
		return fmt.Errorf("cannot convert required resource %s: %w", key, err)
	}
	return nil
}

func SetString(ptr *string, field *string) {
	if ptr != nil {
		*field = *ptr
	}
}

func SetBool(ptr *bool, field *bool) {
	if ptr != nil {
		*field = *ptr
	}
}

func SetFloat64(ptr *float64, field *float64) {
	if ptr != nil {
		*field = *ptr
	}
}

func GenerateEligibleKubernetesFullName(original string) string {
	return GenerateEligibleKubernetesName(original, 253)
}

func GenerateEligibleKubernetesLabelName(original string) string {
	return GenerateEligibleKubernetesName(original, 58)
}

func GenerateEligibleKubernetesName(original string, limit int) string {
	processed := unidecode.Unidecode(original)
	processed = strings.ToLower(processed)
	reg, _ := regexp.Compile("[^a-z0-9-]+")
	processed = reg.ReplaceAllString(processed, "-")
	reg, _ = regexp.Compile("-+")
	processed = reg.ReplaceAllString(processed, "-")
	reg, _ = regexp.Compile("^([^a-z]+)")
	processed = reg.ReplaceAllString(processed, "")
	reg, _ = regexp.Compile("[^a-z0-9]+$")
	processed = reg.ReplaceAllString(processed, "")
	processed = strings.Trim(processed, " ")
	if len(processed) > limit {
		processed = processed[:limit]
	}
	reg, _ = regexp.Compile("[^a-z0-9]+$")
	processed = reg.ReplaceAllString(processed, "")
	return processed
}

func IsResourceReady(observed *composed.Unstructured) bool {
	return GetCrossplaneReadyStatus(observed) == resource.ReadyTrue
}

func GetCrossplaneReadyStatus(observed *composed.Unstructured) resource.Ready {
	conditions, found, err := unstructured.NestedSlice(observed.Object, "status", "conditions")
	if err != nil || !found {
		return defaultCrossplaneReadyStatus(observed)
	}

	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}
		if conditionMap["type"] == "Synced" && conditionMap["status"] == "False" {
			return resource.ReadyFalse
		}
		if conditionMap["type"] != "Ready" {
			continue
		}
		if conditionMap["status"] == "True" {
			return resource.ReadyTrue
		}
		return resource.ReadyFalse
	}
	return defaultCrossplaneReadyStatus(observed)
}

func defaultCrossplaneReadyStatus(observed *composed.Unstructured) resource.Ready {
	if strings.Contains(observed.GetAPIVersion(), "upbound.io") {
		return resource.ReadyFalse
	}
	return resource.ReadyTrue
}

// Copied from github.com/crossplane/function-sdk-go to make it compatible with crossplane-runtime v2
func SetConditions(xr *composite.Unstructured, conditions ...xpv1.Condition) {
	conditioned := xpv1.ConditionedStatus{}
	_ = fieldpath.Pave(xr.Object).GetValueInto("status", &conditioned)
	conditioned.SetConditions(conditions...)
	_ = fieldpath.Pave(xr.Object).SetValue("status.conditions", conditioned.Conditions)
}

// Copied from github.com/crossplane/function-sdk-go to make it compatible with crossplane-runtime v2
func NewLogger(debug bool) (logging.Logger, error) {
	o := []zap.Option{zap.AddCallerSkip(1)}
	if debug {
		zl, err := zap.NewDevelopment(o...)
		return logging.NewLogrLogger(zapr.NewLogger(zl)), errors.Wrap(err, "cannot create development zap logger")
	}
	zl, err := zap.NewProduction(o...)
	return logging.NewLogrLogger(zapr.NewLogger(zl)), errors.Wrap(err, "cannot create production zap logger")
}
