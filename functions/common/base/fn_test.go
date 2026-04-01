package base

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockGroupService implements GroupService with configurable fields.
type mockGroupService struct {
	requiredResources    map[string]*fnv1.ResourceSelector
	requiredResourcesErr error
	sequence             Sequence
}

func (m *mockGroupService) SetLogger(_ logging.Logger) {}
func (m *mockGroupService) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}
func (m *mockGroupService) GetResourceHandlers() map[string]ResourceHandler {
	return nil
}
func (m *mockGroupService) GetSequence(_ client.Object) Sequence {
	return m.sequence
}
func (m *mockGroupService) GetReadyStatus(_ *composed.Unstructured) resource.Ready {
	return resource.ReadyTrue
}
func (m *mockGroupService) GetObservedStatus(_ *composed.Unstructured) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockGroupService) GetRequiredResources(_ *composite.Unstructured, _ map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	return m.requiredResources, m.requiredResourcesErr
}

// Ensure mockGroupService satisfies the interface at compile time.
var _ GroupService = (*mockGroupService)(nil)

// makeComposite builds a *resource.Composite with the given namespace set.
func makeComposite(namespace string) *resource.Composite {
	obj := unstructured.Unstructured{}
	obj.SetNamespace(namespace)
	return &resource.Composite{
		Resource: &composite.Unstructured{Unstructured: obj},
	}
}

// makeNamespaceRequired builds a required Namespace entry with an optional zone label.
func makeNamespaceRequired(zoneLabel string) resource.Required {
	labels := map[string]string{}
	if zoneLabel != "" {
		labels[TenancyZoneLabel] = zoneLabel
	}
	obj := unstructured.Unstructured{}
	obj.SetLabels(labels)
	return resource.Required{Resource: &obj}
}

func TestAddRequiredResources(t *testing.T) {
	log := logging.NewNopLogger()

	cases := map[string]struct {
		service       *mockGroupService
		composite     *resource.Composite
		required      map[string][]resource.Required
		wantErr       bool
		wantNil       bool // rsp.Requirements should remain nil
		wantResources map[string]*fnv1.ResourceSelector
	}{
		"ClusterScopedNoServiceRequirements": {
			service:   &mockGroupService{},
			composite: makeComposite(""),
			required:  nil,
			wantNil:   true,
		},
		"ClusterScopedWithServiceRequirements": {
			service: &mockGroupService{
				requiredResources: map[string]*fnv1.ResourceSelector{
					EnvironmentKey: RequiredEnvironmentConfig("my-env"),
				},
			},
			composite: makeComposite(""),
			required:  nil,
			wantResources: map[string]*fnv1.ResourceSelector{
				EnvironmentKey: RequiredEnvironmentConfig("my-env"),
			},
		},
		"NamespacedNamespaceNotYetFetched": {
			service:   &mockGroupService{},
			composite: makeComposite("my-ns"),
			required:  nil,
			wantResources: map[string]*fnv1.ResourceSelector{
				NamespaceKey: RequiredNamespace("my-ns"),
			},
		},
		"NamespacedNamespacePresentNoZone": {
			service:   &mockGroupService{},
			composite: makeComposite("my-ns"),
			required: map[string][]resource.Required{
				NamespaceKey: {makeNamespaceRequired("")},
			},
			wantResources: map[string]*fnv1.ResourceSelector{
				NamespaceKey: RequiredNamespace("my-ns"),
			},
		},
		"NamespacedNamespacePresentWithZone": {
			service:   &mockGroupService{},
			composite: makeComposite("my-ns"),
			required: map[string][]resource.Required{
				NamespaceKey: {makeNamespaceRequired("my-zone")},
			},
			wantResources: map[string]*fnv1.ResourceSelector{
				NamespaceKey: RequiredNamespace("my-ns"),
				ZoneKey:      RequiredZone("my-zone"),
				ZoneEnvKey:   RequiredEnvironmentConfig(ZoneEnvName),
			},
		},
		"NamespacedWithServiceResourcesAndZone": {
			service: &mockGroupService{
				requiredResources: map[string]*fnv1.ResourceSelector{
					EnvironmentKey: RequiredEnvironmentConfig("my-env"),
				},
			},
			composite: makeComposite("my-ns"),
			required: map[string][]resource.Required{
				NamespaceKey: {makeNamespaceRequired("my-zone")},
			},
			wantResources: map[string]*fnv1.ResourceSelector{
				EnvironmentKey: RequiredEnvironmentConfig("my-env"),
				NamespaceKey:   RequiredNamespace("my-ns"),
				ZoneKey:        RequiredZone("my-zone"),
				ZoneEnvKey:     RequiredEnvironmentConfig(ZoneEnvName),
			},
		},
		"ServiceReturnsError": {
			service: &mockGroupService{
				requiredResourcesErr: fmt.Errorf("service error"),
			},
			composite: makeComposite("my-ns"),
			required:  nil,
			wantErr:   true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: log, groupService: tc.service}
			rsp := &fnv1.RunFunctionResponse{}

			err := f.addRequiredResources(rsp, tc.composite, tc.required)

			if tc.wantErr {
				if err == nil {
					t.Errorf("addRequiredResources(): expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("addRequiredResources(): unexpected error: %v", err)
				return
			}

			if tc.wantNil {
				if rsp.Requirements != nil {
					t.Errorf("addRequiredResources(): expected nil requirements, got %v", rsp.Requirements)
				}
				return
			}

			if rsp.Requirements == nil {
				t.Fatalf("addRequiredResources(): expected requirements, got nil")
			}

			if diff := cmp.Diff(tc.wantResources, rsp.Requirements.Resources, protocmp.Transform()); diff != "" {
				t.Errorf("addRequiredResources(): -want +got:\n%s", diff)
			}
		})
	}
}

func TestRunFunctionSkipsWhenSkipGeneration(t *testing.T) {
	log := logging.NewNopLogger()
	svc := &skipGroupService{}
	f := NewFunction(log, svc, "")

	resourceStruct, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "example.com/v1",
		"kind":       "Test",
		"metadata":   map[string]interface{}{"name": "test"},
	})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}

	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{
				Resource: resourceStruct,
			},
		},
	}

	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("RunFunction: unexpected error: %v", err)
	}
	if rsp.Requirements != nil {
		t.Errorf("RunFunction with SkipGeneration: expected nil requirements, got %v", rsp.Requirements)
	}
	if len(rsp.GetDesired().GetResources()) != 0 {
		t.Errorf("RunFunction with SkipGeneration: expected empty desired resources")
	}
}

type skipGroupService struct {
	mockGroupService
}

func (s *skipGroupService) SkipGeneration(_ *composite.Unstructured) bool { return true }

func TestInjectZone(t *testing.T) {
	// makeUnstructuredWithTags builds an unstructured object that has spec.forProvider.tags,
	// seeded with the given existing tags. A unique GVK is set so supportsField cache does
	// not interfere between cases.
	makeUnstructuredWithTags := func(gvkSuffix string, existingTags map[string]interface{}, labels map[string]string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{Object: map[string]interface{}{}}
		u.SetAPIVersion("test.example.com/v1")
		u.SetKind("InjectZone" + gvkSuffix)
		u.SetLabels(labels)
		if existingTags != nil {
			_ = unstructured.SetNestedMap(u.Object, existingTags, "spec", "forProvider", "tags")
		} else {
			// Set an empty tags field so supportsField returns true.
			_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{}, "spec", "forProvider", "tags")
		}
		return u
	}

	// makeUnstructuredNoTags builds an unstructured object without spec.forProvider.tags.
	makeUnstructuredNoTags := func(gvkSuffix string) *unstructured.Unstructured {
		u := &unstructured.Unstructured{Object: map[string]interface{}{}}
		u.SetAPIVersion("test.example.com/v1")
		u.SetKind("InjectZoneNoTags" + gvkSuffix)
		return u
	}

	// makeManyTags builds a tags map with n entries.
	makeManyTags := func(n int) map[string]interface{} {
		m := make(map[string]interface{}, n)
		for i := range n {
			m[fmt.Sprintf("key%d", i)] = fmt.Sprintf("val%d", i)
		}
		return m
	}

	log := logging.NewNopLogger()

	cases := map[string]struct {
		workspace    string
		resourceTags ResourceTags
		buildObj     func() (*unstructured.Unstructured, client.Object)
		wantErr      bool
		wantLabels   map[string]string
		wantTags     map[string]interface{} // nil means tags field should be absent / untouched
	}{
		"NoTagsFieldOnlyLabelsUpdated": {
			resourceTags: ResourceTags{
				Zone:   "prod",
				Tags:   map[string]string{"Cost": "high"},
				Labels: map[string]string{TagsPrefix + "Team": "platform"},
			},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				u := makeUnstructuredNoTags("OnlyLabels")
				return u, u
			},
			wantLabels: map[string]string{
				TenancyZoneLabel:    "prod",
				TagsPrefix + "Team": "platform",
			},
			wantTags: nil,
		},
		"ZoneWrittenToLabelsAndTags": {
			resourceTags: ResourceTags{Zone: "prod"},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				u := makeUnstructuredWithTags("ZoneLabel", nil, nil)
				return u, u
			},
			wantLabels: map[string]string{TenancyZoneLabel: "prod"},
			wantTags:   map[string]interface{}{TenancyZoneAWSTag: "prod"},
		},
		"WorkspaceWrittenToLabelsAndTags": {
			workspace:    "dev",
			resourceTags: ResourceTags{},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				u := makeUnstructuredWithTags("workspace", nil, nil)
				return u, u
			},
			wantLabels: map[string]string{TenancyWorkspaceLabel: "dev"},
			wantTags:   map[string]interface{}{TenancyWorkspaceAWSTag: "dev"},
		},
		"ResourceTagLabelsAndTagsMerged": {
			resourceTags: ResourceTags{
				Zone:   "staging",
				Tags:   map[string]string{"Cost": "low", "Team": "ops"},
				Labels: map[string]string{TagsPrefix + "Cost": "low"},
			},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				u := makeUnstructuredWithTags("Merge", map[string]interface{}{"existing": "val"}, nil)
				return u, u
			},
			wantLabels: map[string]string{
				TenancyZoneLabel:    "staging",
				TagsPrefix + "Cost": "low",
			},
			wantTags: map[string]interface{}{
				"existing":        "val",
				"Cost":            "low",
				"Team":            "ops",
				TenancyZoneAWSTag: "staging",
			},
		},
		"ExistingObjectLabelsPreserved": {
			resourceTags: ResourceTags{Zone: "qa"},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				u := makeUnstructuredWithTags("ExistingLabels", nil, map[string]string{"app": "myapp"})
				return u, u
			},
			wantLabels: map[string]string{
				"app":            "myapp",
				TenancyZoneLabel: "qa",
			},
			wantTags: map[string]interface{}{TenancyZoneAWSTag: "qa"},
		},
		"NoZoneNoWorkspaceTagsFieldLeftEmpty": {
			resourceTags: ResourceTags{},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				u := makeUnstructuredWithTags("Empty", nil, nil)
				return u, u
			},
			wantLabels: map[string]string{},
			wantTags:   map[string]interface{}{},
		},
		"TagsLimitExceededReturnsError": {
			resourceTags: ResourceTags{Zone: "prod"},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				// AWSTagsLimit is 44; pre-fill with 44 tags so adding Zone exceeds it.
				u := makeUnstructuredWithTags("Limit", makeManyTags(AWSTagsLimit), nil)
				return u, u
			},
			wantErr: true,
		},
		"TypedObjWithTagsFieldUpdated": {
			resourceTags: ResourceTags{Zone: "prod"},
			buildObj: func() (*unstructured.Unstructured, client.Object) {
				typed := &tagTestObjWithTags{}
				typed.APIVersion = "test.example.com/v1"
				typed.Kind = "InjectZoneTyped"
				u, err := ToUnstructured(typed)
				if err != nil {
					panic(err)
				}
				// Ensure tags field is present so supportsField picks it up.
				_ = unstructured.SetNestedMap(u.Object, map[string]interface{}{}, "spec", "forProvider", "tags")
				return u, typed
			},
			wantLabels: map[string]string{TenancyZoneLabel: "prod"},
			wantTags:   map[string]interface{}{TenancyZoneAWSTag: "prod"},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: log, workspace: tc.workspace}
			u, obj := tc.buildObj()

			err := f.injectZone(obj, u, tc.resourceTags)

			if tc.wantErr {
				if err == nil {
					t.Errorf("injectZone(): expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("injectZone(): unexpected error: %v", err)
				return
			}

			gotLabels := u.GetLabels()
			if gotLabels == nil {
				gotLabels = map[string]string{}
			}
			if diff := cmp.Diff(tc.wantLabels, gotLabels); diff != "" {
				t.Errorf("injectZone() labels mismatch (-want +got):\n%s", diff)
			}

			if tc.wantTags == nil {
				// Expect no tags field in the object.
				tags, found, _ := unstructured.NestedMap(u.Object, "spec", "forProvider", "tags")
				if found && len(tags) > 0 {
					t.Errorf("injectZone(): expected no tags, got %v", tags)
				}
				return
			}

			gotTags, _, _ := unstructured.NestedMap(u.Object, "spec", "forProvider", "tags")
			if gotTags == nil {
				gotTags = map[string]interface{}{}
			}
			if diff := cmp.Diff(tc.wantTags, gotTags); diff != "" {
				t.Errorf("injectZone() tags mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestAddDesiredSequenceResources(t *testing.T) {
	log := logging.NewNopLogger()

	// makeObj builds a minimal unstructured client.Object for use in allGeneratedObjects.
	makeObj := func(name string) client.Object {
		u := &unstructured.Unstructured{}
		u.SetName(name)
		u.SetAPIVersion("test.example.com/v1")
		u.SetKind("SeqResource")
		return u
	}

	// makeObserved builds a minimal observed composed resource (marks resource as existing).
	makeObserved := func(name string) resource.ObservedComposed {
		u := composed.New()
		u.SetName(name)
		return resource.ObservedComposed{Resource: u}
	}

	cases := map[string]struct {
		sequence            Sequence
		allGeneratedObjects map[string]client.Object
		observed            map[resource.Name]resource.ObservedComposed
		wantProcessedNames  Set[string]
		wantDesiredNames    []string // resources expected in desired map
		wantNotDesiredNames []string // resources expected NOT in desired (blocked by sequence)
		wantErr             bool
	}{
		"EmptySequence": {
			sequence:            Sequence{},
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a")},
			observed:            nil,
			wantProcessedNames:  NewSet[string](),
			wantDesiredNames:    nil,
		},
		"SingleStep_AllObserved_AllReady": {
			sequence:            NewSequence(false, []string{"a", "b"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a"), "b": makeObj("b")},
			observed: map[resource.Name]resource.ObservedComposed{
				"a": makeObserved("a"),
				"b": makeObserved("b"),
			},
			wantProcessedNames: NewSet("a", "b"),
			wantDesiredNames:   []string{"a", "b"},
		},
		"SingleStep_NoneObserved_FirstStepAlwaysAdded": {
			// previousStepIsReady starts true, so unobserved resources in step 1 are still added.
			sequence:            NewSequence(false, []string{"a"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a")},
			observed:            nil,
			wantProcessedNames:  NewSet("a"),
			wantDesiredNames:    []string{"a"},
		},
		"TwoSteps_Step1NotReady_BlocksUnobservedStep2": {
			// Step1: "a" not observed → added but ReadyUnspecified → step1 not all ready.
			// Step2: "b" not observed → blocked by previousStepIsReady=false → not added to desired.
			sequence:            NewSequence(false, []string{"a"}, []string{"b"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a"), "b": makeObj("b")},
			observed:            nil,
			wantProcessedNames:  NewSet("a", "b"),
			wantDesiredNames:    []string{"a"},
			wantNotDesiredNames: []string{"b"},
		},
		"TwoSteps_Step1Ready_UnblocksStep2": {
			// Step1: "a" observed → ReadyTrue → step1 all ready.
			// Step2: "b" not observed but previousStepIsReady=true → added.
			sequence:            NewSequence(false, []string{"a"}, []string{"b"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a"), "b": makeObj("b")},
			observed: map[resource.Name]resource.ObservedComposed{
				"a": makeObserved("a"),
			},
			wantProcessedNames: NewSet("a", "b"),
			wantDesiredNames:   []string{"a", "b"},
		},
		"TwoSteps_Step2ObservedAdded_EvenWhenStep1NotReady": {
			// Step1: "a" not observed → ReadyUnspecified → step1 not all ready.
			// Step2: "b" IS observed → condition !existsInObserved=false → always added.
			sequence:            NewSequence(false, []string{"a"}, []string{"b"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a"), "b": makeObj("b")},
			observed: map[resource.Name]resource.ObservedComposed{
				"b": makeObserved("b"),
			},
			wantProcessedNames: NewSet("a", "b"),
			wantDesiredNames:   []string{"a", "b"},
		},
		"NameNotInGeneratedObjects_NotInProcessedNames": {
			// "missing" is listed in the step but absent from allGeneratedObjects → skipped,
			// not added to processedNames (caller will attempt it as a non-sequenced resource).
			sequence:            NewSequence(false, []string{"a", "missing"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a")},
			observed:            nil,
			wantProcessedNames:  NewSet("a"), // "missing" NOT included
			wantDesiredNames:    []string{"a"},
		},
		"RegexSequence_MatchesMultiple": {
			sequence:            NewSequence(true, []string{"prefix-.*"}, []string{"other"}),
			allGeneratedObjects: map[string]client.Object{"prefix-x": makeObj("prefix-x"), "prefix-y": makeObj("prefix-y"), "other": makeObj("other")},
			observed: map[resource.Name]resource.ObservedComposed{
				"prefix-x": makeObserved("prefix-x"),
				"prefix-y": makeObserved("prefix-y"),
			},
			wantProcessedNames: NewSet("prefix-x", "prefix-y", "other"),
			wantDesiredNames:   []string{"prefix-x", "prefix-y", "other"},
		},
		"RegexSequence_Step1NotReady_BlocksStep2": {
			// Step1 regex matches "prefix-x" which is NOT observed → not ready.
			// Step2 "other" is NOT observed → blocked.
			sequence:            NewSequence(true, []string{"prefix-.*"}, []string{"other"}),
			allGeneratedObjects: map[string]client.Object{"prefix-x": makeObj("prefix-x"), "other": makeObj("other")},
			observed:            nil,
			wantProcessedNames:  NewSet("prefix-x", "other"),
			wantDesiredNames:    []string{"prefix-x"},
			wantNotDesiredNames: []string{"other"},
		},
		"InvalidRegex_ReturnsError": {
			sequence:            NewSequence(true, []string{"[invalid"}),
			allGeneratedObjects: map[string]client.Object{"a": makeObj("a")},
			observed:            nil,
			wantErr:             true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &mockGroupService{sequence: tc.sequence}
			f := &Function{log: log, groupService: svc}

			obj := &unstructured.Unstructured{}
			desired := make(map[resource.Name]*resource.DesiredComposed)

			processedNames, err := f.addDesiredSequenceResources(obj, tc.observed, desired, tc.allGeneratedObjects, ResourceTags{})

			if tc.wantErr {
				if err == nil {
					t.Errorf("addDesiredSequenceResources(): expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("addDesiredSequenceResources(): unexpected error: %v", err)
				return
			}

			if diff := cmp.Diff(tc.wantProcessedNames, processedNames); diff != "" {
				t.Errorf("addDesiredSequenceResources() processedNames mismatch (-want +got):\n%s", diff)
			}

			for _, wantName := range tc.wantDesiredNames {
				if _, ok := desired[resource.Name(wantName)]; !ok {
					t.Errorf("addDesiredSequenceResources(): expected %q in desired, but it was absent", wantName)
				}
			}
			for _, notWantName := range tc.wantNotDesiredNames {
				if _, ok := desired[resource.Name(notWantName)]; ok {
					t.Errorf("addDesiredSequenceResources(): expected %q to be absent from desired, but it was present", notWantName)
				}
			}
		})
	}
}
