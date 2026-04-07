package base

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type tagTestForProvider struct {
	Tags map[string]string `json:"tags"`
}

type tagTestSpec struct {
	ForProvider tagTestForProvider `json:"forProvider"`
}

type tagTestObjWithTags struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec tagTestSpec `json:"spec"`
}

func (o *tagTestObjWithTags) DeepCopyObject() runtime.Object {
	c := *o
	return &c
}

type tagTestObjNoTags struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (o *tagTestObjNoTags) DeepCopyObject() runtime.Object {
	c := *o
	return &c
}

type tagTestPtrForProvider struct {
	ForProvider *tagTestForProvider `json:"forProvider"`
}

type tagTestObjPtrFields struct {
	metav1.TypeMeta
	metav1.ObjectMeta
	Spec *tagTestPtrForProvider `json:"spec"`
}

func (o *tagTestObjPtrFields) DeepCopyObject() runtime.Object {
	c := *o
	return &c
}

func TestExtractTags(t *testing.T) {
	cases := map[string]struct {
		src  map[string]string
		want map[string]string
	}{
		"NilInput": {
			src:  nil,
			want: map[string]string{},
		},
		"EmptyInput": {
			src:  map[string]string{},
			want: map[string]string{},
		},
		"NoPrefixedKeys": {
			src:  map[string]string{"app": "myapp", "env": "prod"},
			want: map[string]string{},
		},
		"AllPrefixedKeys": {
			src:  map[string]string{TagsPrefix + "Env": "prod", TagsPrefix + "Team": "platform"},
			want: map[string]string{"Env": "prod", "Team": "platform"},
		},
		"MixedKeys": {
			src:  map[string]string{"app": "myapp", TagsPrefix + "Env": "prod"},
			want: map[string]string{"Env": "prod"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := extractTags(tc.src)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("extractTags() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExtractTagsLabels(t *testing.T) {
	cases := map[string]struct {
		src        map[string]string
		wantTags   map[string]string
		wantLabels map[string]string
	}{
		"NilInput": {
			src:        nil,
			wantTags:   map[string]string{},
			wantLabels: map[string]string{},
		},
		"NoPrefixedKeys": {
			src:        map[string]string{"app": "myapp"},
			wantTags:   map[string]string{},
			wantLabels: map[string]string{},
		},
		"PrefixedKeys": {
			src:        map[string]string{TagsPrefix + "Env": "prod", TagsPrefix + "Team": "platform"},
			wantTags:   map[string]string{"Env": "prod", "Team": "platform"},
			wantLabels: map[string]string{TagsPrefix + "Env": "prod", TagsPrefix + "Team": "platform"},
		},
		"MixedKeys": {
			src:        map[string]string{"app": "myapp", TagsPrefix + "Env": "prod"},
			wantTags:   map[string]string{"Env": "prod"},
			wantLabels: map[string]string{TagsPrefix + "Env": "prod"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotTags, gotLabels := extractTagsLabels(tc.src)
			if diff := cmp.Diff(tc.wantTags, gotTags, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("extractTagsLabels() tags mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantLabels, gotLabels, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("extractTagsLabels() labels mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetObjectTagsLabels(t *testing.T) {
	cases := map[string]struct {
		labels      map[string]string
		annotations map[string]string
		wantTags    map[string]string
		wantLabels  map[string]string
	}{
		"NoLabelsNoAnnotations": {
			wantTags:   map[string]string{},
			wantLabels: map[string]string{},
		},
		"PrefixedLabelOnly": {
			labels:     map[string]string{TagsPrefix + "Env": "prod"},
			wantTags:   map[string]string{"Env": "prod"},
			wantLabels: map[string]string{TagsPrefix + "Env": "prod"},
		},
		"PrefixedAnnotationOnly": {
			annotations: map[string]string{TagsPrefix + "Env": "staging"},
			wantTags:    map[string]string{"Env": "staging"},
			wantLabels:  map[string]string{},
		},
		"AnnotationOverridesLabelInTags": {
			labels:      map[string]string{TagsPrefix + "Env": "prod"},
			annotations: map[string]string{TagsPrefix + "Env": "staging"},
			wantTags:    map[string]string{"Env": "staging"},
			wantLabels:  map[string]string{TagsPrefix + "Env": "prod"},
		},
		"DisjointLabelAndAnnotationKeys": {
			labels:      map[string]string{TagsPrefix + "Env": "prod"},
			annotations: map[string]string{TagsPrefix + "Team": "platform"},
			wantTags:    map[string]string{"Env": "prod", "Team": "platform"},
			wantLabels:  map[string]string{TagsPrefix + "Env": "prod"},
		},
		"NonPrefixedKeysIgnored": {
			labels:      map[string]string{"app": "myapp", TagsPrefix + "Env": "prod"},
			annotations: map[string]string{"other": "value"},
			wantTags:    map[string]string{"Env": "prod"},
			wantLabels:  map[string]string{TagsPrefix + "Env": "prod"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			obj := unstructured.Unstructured{}
			obj.SetLabels(tc.labels)
			obj.SetAnnotations(tc.annotations)
			gotTags, gotLabels := getObjectTagsLabels(obj)
			if diff := cmp.Diff(tc.wantTags, gotTags, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("getObjectTagsLabels() tags mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantLabels, gotLabels, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("getObjectTagsLabels() labels mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveFieldPath(t *testing.T) {
	cases := map[string]struct {
		obj       client.Object
		fieldPath []string
		want      bool
	}{
		"UnstructuredFieldPresent": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"forProvider": map[string]interface{}{
						"tags": map[string]interface{}{},
					},
				},
			}},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      true,
		},
		"UnstructuredFieldAbsent": {
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{},
			}},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      false,
		},
		"TypedFieldPresent": {
			obj:       &tagTestObjWithTags{},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      true,
		},
		"TypedFieldAbsent": {
			obj:       &tagTestObjNoTags{},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      false,
		},
		"TypedIntermediateFieldAbsent": {
			obj:       &tagTestObjWithTags{},
			fieldPath: []string{"spec", "missing", "tags"},
			want:      false,
		},
		"TypedPointerFields": {
			obj:       &tagTestObjPtrFields{},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := resolveFieldPath(tc.obj, tc.fieldPath)
			if got != tc.want {
				t.Errorf("resolveFieldPath() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSupportsField(t *testing.T) {
	cases := map[string]struct {
		obj       client.Object
		fieldPath []string
		want      bool
	}{
		"UnstructuredFieldPresent": {
			obj: unstructuredWithGVK("test.example.com/v1", "SupportsFieldWithTags", map[string]interface{}{
				"spec": map[string]interface{}{
					"forProvider": map[string]interface{}{
						"tags": map[string]interface{}{},
					},
				},
			}),
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      true,
		},
		"UnstructuredFieldAbsent": {
			obj:       unstructuredWithGVK("test.example.com/v1", "SupportsFieldNoTags", nil),
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      false,
		},
		"TypedFieldPresent": {
			obj: &tagTestObjWithTags{
				TypeMeta: metav1.TypeMeta{APIVersion: "test.example.com/v1", Kind: "TypedSupportsWithTags"},
			},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      true,
		},
		"TypedFieldAbsent": {
			obj: &tagTestObjNoTags{
				TypeMeta: metav1.TypeMeta{APIVersion: "test.example.com/v1", Kind: "TypedSupportsNoTags"},
			},
			fieldPath: []string{"spec", "forProvider", "tags"},
			want:      false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := supportsField(tc.obj, tc.fieldPath...)
			if got != tc.want {
				t.Errorf("supportsField() = %v, want %v", got, tc.want)
			}
			// Second call must return the same result via cache.
			if got2 := supportsField(tc.obj, tc.fieldPath...); got2 != got {
				t.Errorf("supportsField() cached result = %v, want %v", got2, got)
			}
		})
	}
}

func unstructuredWithGVK(apiVersion, kind string, extra map[string]interface{}) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]interface{}{}}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	for k, v := range extra {
		u.Object[k] = v
	}
	return u
}
