package base

import (
	"reflect"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	tagSupportCache  sync.Map
	tagSupportFlight singleflight.Group
)

func supportsField(obj client.Object, fieldPath ...string) bool {
	key := obj.GetObjectKind().GroupVersionKind()
	if cached, ok := tagSupportCache.Load(key); ok {
		return cached.(bool)
	}

	result, _, _ := tagSupportFlight.Do(key.String(), func() (interface{}, error) {
		if cached, ok := tagSupportCache.Load(key); ok {
			return cached.(bool), nil
		}
		r := resolveFieldPath(obj, fieldPath)
		tagSupportCache.Store(key, r)
		return r, nil
	})

	return result.(bool)
}

func resolveFieldPath(obj client.Object, fieldPath []string) bool {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		_, found, _ := unstructured.NestedFieldNoCopy(u.Object, fieldPath...)
		return found
	}
	t := reflect.TypeOf(obj)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for _, name := range fieldPath {
		if t.Kind() != reflect.Struct {
			return false
		}
		found := false
		numFields := t.NumField()
		for i := 0; i < numFields; i++ {
			field := t.Field(i)
			tag := field.Tag.Get("json")
			if idx := strings.IndexByte(tag, ','); idx >= 0 {
				tag = tag[:idx]
			}
			if tag == name {
				t = field.Type
				for t.Kind() == reflect.Ptr {
					t = t.Elem()
				}
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
