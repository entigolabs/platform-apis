package base

import (
	"context"
	"fmt"
	"maps"
	"runtime/debug"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Function composes entigo resources.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log          logging.Logger
	groupService GroupService
}

func NewFunction(log logging.Logger, groupService GroupService) *Function {
	return &Function{
		log:          log,
		groupService: groupService,
	}
}

// RunFunction runs the Function.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (rsp *fnv1.RunFunctionResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			f.log.Info(fmt.Sprintf("Panic: %v\n%s", r, debug.Stack()))
			response.Fatal(rsp, fmt.Errorf("panic: %v", r))
		}
	}()
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())

	rsp = response.To(req, response.DefaultTTL)

	compositeResource, err := getObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource from %T", req))
		return rsp, nil
	}

	requiredResources, err := request.GetRequiredResources(req)
	if err != nil {
		response.Fatal(rsp, fmt.Errorf("could not fetch required resources: %w", err))
		return rsp, nil
	}

	err = f.addRequiredResources(rsp, compositeResource, requiredResources)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot add required resources to %T", rsp))
		return rsp, nil
	}
	if !requiredResourcesPresent(rsp.Requirements, req.RequiredResources) {
		return rsp, nil
	}

	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources from %T", req))
		return rsp, nil
	}

	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired resources from %T", req))
		return rsp, nil
	}

	err = f.addDesiredComposedResources(rsp, compositeResource, requiredResources, observed, desired)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot add desired composed resources to %T", rsp))
		return rsp, nil
	}

	err = f.addStatus(rsp, compositeResource, observed)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot add status to composite resource in %T", rsp))
		return rsp, nil
	}

	f.log.Info("Successfully composed resources", "kind", compositeResource.Resource.GetKind(), "count", len(desired))
	return rsp, nil
}

func getObservedCompositeResource(req *fnv1.RunFunctionRequest) (*resource.Composite, error) {
	compositeResource, err := request.GetObservedCompositeResource(req)
	if err != nil {
		return nil, err
	}
	compositeResource.Resource.SetManagedFields(nil)
	return compositeResource, nil
}

func requiredResourcesPresent(requirements *fnv1.Requirements, resources map[string]*fnv1.Resources) bool {
	if requirements == nil || len(requirements.Resources) == 0 {
		return true
	}
	if len(resources) == 0 {
		return false
	}
	for key := range requirements.Resources {
		if _, ok := resources[key]; !ok {
			return false
		}
	}
	return true
}

func (f *Function) addRequiredResources(rsp *fnv1.RunFunctionResponse, composite *resource.Composite, required map[string][]resource.Required) error {
	resources, err := f.groupService.GetRequiredResources(composite.Resource, required)
	if err != nil {
		return err
	}
	if len(resources) == 0 {
		return nil
	}
	rsp.Requirements = &fnv1.Requirements{
		Resources: resources,
	}
	return nil
}

func (f *Function) addDesiredComposedResources(
	rsp *fnv1.RunFunctionResponse,
	compositeResource *resource.Composite,
	requiredResources map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
	desired map[resource.Name]*resource.DesiredComposed,
) error {
	handler, ok := f.groupService.GetResourceHandlers()[compositeResource.Resource.GetKind()]
	if !ok {
		return fmt.Errorf("no resource handler found for kind %s", compositeResource.Resource.GetKind())
	}
	object := handler.Instantiate()
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(compositeResource.Resource.Object, object); err != nil {
		return err
	}
	allGeneratedObjects, err := handler.Generate(object, requiredResources, observed)
	if err != nil {
		return err
	}
	processedNames, err := f.addDesiredSequenceResources(compositeResource, observed, desired, allGeneratedObjects)
	if err != nil {
		return err
	}
	for name, obj := range allGeneratedObjects {
		if processedNames[name] {
			continue
		}
		if err := f.addDesiredResource(desired, name, obj, observed); err != nil {
			return fmt.Errorf("cannot add non-sequenced desired resource %s: %w", name, err)
		}
	}
	return response.SetDesiredComposedResources(rsp, desired)
}

func (f *Function) addDesiredSequenceResources(
	compositeResource *resource.Composite,
	observed map[resource.Name]resource.ObservedComposed,
	desired map[resource.Name]*resource.DesiredComposed,
	allGeneratedObjects map[string]runtime.Object,
) (Set[string], error) {
	processedNames := NewSet[string]()
	previousStepIsReady := true

	for _, step := range f.groupService.GetSequence(compositeResource.Resource) {
		currentStepAllReady := true
		for _, name := range step.Objects {
			obj, ok := allGeneratedObjects[name]
			if !ok {
				f.log.Info("Skipping sequence object not in generated objects", "name", name)
				continue
			}
			processedNames[name] = true
			_, existsInObserved := observed[resource.Name(name)]
			if !existsInObserved && !previousStepIsReady {
				currentStepAllReady = false
				continue
			}
			if err := f.addDesiredResource(desired, name, obj, observed); err != nil {
				return nil, fmt.Errorf("cannot add desired resource %s: %w", name, err)
			}
			if desired[resource.Name(name)].Ready != resource.ReadyTrue {
				currentStepAllReady = false
			}
		}
		previousStepIsReady = currentStepAllReady
	}
	return processedNames, nil
}

func (f *Function) addDesiredResource(desired map[resource.Name]*resource.DesiredComposed, name string, obj runtime.Object, observed map[resource.Name]resource.ObservedComposed) error {
	ready := resource.ReadyUnspecified
	if observedResource, ok := observed[resource.Name(name)]; ok {
		ready = f.getReadyStatus(observedResource.Resource)
	}

	unstructuredObject, err := ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("cannot convert object to unstructured: %w", err)
	}

	desired[resource.Name(name)] = &resource.DesiredComposed{
		Resource: &composed.Unstructured{
			Unstructured: *unstructuredObject,
		},
		Ready: ready,
	}
	return nil
}

func (f *Function) getReadyStatus(observed *composed.Unstructured) resource.Ready {
	ready := f.groupService.GetReadyStatus(observed)
	if ready != "" {
		return ready
	}
	return GetCrossplaneReadyStatus(observed)
}

func (f *Function) addStatus(rsp *fnv1.RunFunctionResponse, compositeResource *resource.Composite, observed map[resource.Name]resource.ObservedComposed) error {
	extraStatus, err := f.getCompositeResourceStatus(observed)
	if err != nil {
		return fmt.Errorf("cannot get composite resource status: %w", err)
	}

	if len(extraStatus) == 0 {
		return nil
	}
	status, found, err := unstructured.NestedMap(compositeResource.Resource.Object, "status")
	if err != nil || !found {
		return err
	}
	maps.Copy(status, extraStatus)
	err = unstructured.SetNestedField(compositeResource.Resource.Object, status, "status")
	if err != nil {
		return fmt.Errorf("cannot set status in composite resource: %w", err)
	}
	return response.SetDesiredCompositeResource(rsp, compositeResource)
}

func (f *Function) getCompositeResourceStatus(observedObjects map[resource.Name]resource.ObservedComposed) (map[string]interface{}, error) {
	status := make(map[string]interface{})
	for _, observedResource := range observedObjects {
		observed := observedResource.Resource
		observedStatus, err := f.groupService.GetObservedStatus(observed)
		if err != nil {
			return nil, err
		}
		if len(observedStatus) == 0 {
			continue
		}
		maps.Copy(status, observedStatus)
	}
	return status, nil
}
