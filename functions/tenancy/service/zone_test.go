package service

import (
	"testing"

	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/google/go-cmp/cmp"
)

func TestMergeRoleMappings(t *testing.T) {
	tests := map[string]struct {
		zone v1alpha1.Zone
		env  apis.Environment
		want map[string][]string
	}{
		"env only": {
			zone: v1alpha1.Zone{},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"env-contrib"}},
					{RoleRef: "maintainer", Groups: []string{"env-maintainer"}},
					{RoleRef: "observer", Groups: []string{"env-observer"}},
				},
			},
			want: map[string][]string{
				"contributor": {"env-contrib"},
				"maintainer":  {"env-maintainer"},
				"observer":    {"env-observer"},
			},
		},
		"zone only": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"zone-contrib"}},
						{RoleRef: "maintainer", Groups: []string{"zone-maintainer"}},
					},
				},
			},
			env: apis.Environment{},
			want: map[string][]string{
				"contributor": {"zone-contrib"},
				"maintainer":  {"zone-maintainer"},
			},
		},
		"both merged and deduplicated": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"group-a", "group-b"}},
						{RoleRef: "maintainer", Groups: []string{"group-m"}},
					},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"group-b", "group-c"}},
					{RoleRef: "maintainer", Groups: []string{"group-m", "group-n"}},
				},
			},
			want: map[string][]string{
				"contributor": {"group-a", "group-b", "group-c"},
				"maintainer":  {"group-m", "group-n"},
			},
		},
		"contributor includes AppProject groups": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					AppProject: &v1alpha1.AppProject{
						ContributorGroups: []string{"oidc-group"},
					},
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"zone-contrib"}},
					},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"env-contrib"}},
				},
			},
			want: map[string][]string{
				"contributor": {"env-contrib", "oidc-group", "zone-contrib"},
			},
		},
		"AppProject groups without env role mappings": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					AppProject: &v1alpha1.AppProject{
						ContributorGroups: []string{"oidc-group"},
					},
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"zone-contrib"}},
					},
				},
			},
			env: apis.Environment{},
			want: map[string][]string{
				"contributor": {"oidc-group", "zone-contrib"},
			},
		},
		"non-overlapping roles preserved": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "custom-role", Groups: []string{"custom-group"}},
					},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "maintainer", Groups: []string{"env-maintainer"}},
				},
			},
			want: map[string][]string{
				"custom-role": {"custom-group"},
				"maintainer":  {"env-maintainer"},
			},
		},
		"empty AppProject does not affect contributor": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					AppProject: &v1alpha1.AppProject{},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"env-contrib"}},
				},
			},
			want: map[string][]string{
				"contributor": {"env-contrib"},
			},
		},
		"no role mappings anywhere": {
			zone: v1alpha1.Zone{},
			env:  apis.Environment{},
			want: map[string][]string{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := mergeRoleMappings(tc.zone, tc.env)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mergeRoleMappings() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
