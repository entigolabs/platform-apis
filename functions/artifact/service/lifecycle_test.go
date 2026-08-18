package service

import (
	"testing"

	"github.com/entigolabs/platform-apis/apis/v1alpha1"
)

func TestBuildLifecyclePolicy(t *testing.T) {
	cases := map[string]struct {
		rules []v1alpha1.LifecycleRule
		want  string
	}{
		"KeepCountByTagPrefix": {
			rules: []v1alpha1.LifecycleRule{{TagPrefixes: []string{"develop"}, KeepCount: ptr(10)}},
			want:  `{"rules":[{"rulePriority":1,"description":"Keep 10 latest images tagged with prefix develop","selection":{"tagStatus":"tagged","tagPrefixList":["develop"],"countType":"imageCountMoreThan","countNumber":10},"action":{"type":"expire"}}]}`,
		},
		"ExpireUntagged": {
			rules: []v1alpha1.LifecycleRule{{Untagged: true, ExpireAfterDays: ptr(7)}},
			want:  `{"rules":[{"rulePriority":1,"description":"Expire untagged images older than 7 days","selection":{"tagStatus":"untagged","countType":"sinceImagePushed","countUnit":"days","countNumber":7},"action":{"type":"expire"}}]}`,
		},
		// The third sample in lifecycle.md, so the retention the ticket asks for stays pinned.
		"TicketSample": {
			rules: []v1alpha1.LifecycleRule{
				{TagPrefixes: []string{"develop"}, KeepCount: ptr(10)},
				{TagPatterns: []string{"*.*.*.*"}, KeepCount: ptr(100)},
				{TagPatterns: []string{"*.*.*"}, KeepCount: ptr(100)},
				{KeepCount: ptr(100)},
			},
			want: `{"rules":[{"rulePriority":1,"description":"Keep 10 latest images tagged with prefix develop","selection":{"tagStatus":"tagged","tagPrefixList":["develop"],"countType":"imageCountMoreThan","countNumber":10},"action":{"type":"expire"}},{"rulePriority":2,"description":"Keep 100 latest images matching tag pattern *.*.*.*","selection":{"tagStatus":"tagged","tagPatternList":["*.*.*.*"],"countType":"imageCountMoreThan","countNumber":100},"action":{"type":"expire"}},{"rulePriority":3,"description":"Keep 100 latest images matching tag pattern *.*.*","selection":{"tagStatus":"tagged","tagPatternList":["*.*.*"],"countType":"imageCountMoreThan","countNumber":100},"action":{"type":"expire"}},{"rulePriority":4,"description":"Keep 100 latest images","selection":{"tagStatus":"any","countType":"imageCountMoreThan","countNumber":100},"action":{"type":"expire"}}]}`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := BuildLifecyclePolicy(tc.rules)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("BuildLifecyclePolicy():\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestBuildLifecyclePolicyRejectsInvalidRules(t *testing.T) {
	cases := map[string][]v1alpha1.LifecycleRule{
		"NoBound":            {{Untagged: true}},
		"BothBounds":         {{Untagged: true, KeepCount: ptr(10), ExpireAfterDays: ptr(7)}},
		"TwoSelectors":       {{TagPrefixes: []string{"develop"}, TagPatterns: []string{"*"}, KeepCount: ptr(10)}},
		"UntaggedAndPattern": {{Untagged: true, TagPatterns: []string{"*"}, KeepCount: ptr(10)}},
		"EmptyPattern":       {{TagPatterns: []string{""}, KeepCount: ptr(10)}},
		"AnyRuleNotLast":     {{KeepCount: ptr(100)}, {Untagged: true, ExpireAfterDays: ptr(7)}},
		"TwoAnyRules":        {{KeepCount: ptr(100)}, {ExpireAfterDays: ptr(7)}},
		"ZeroKeepCount":      {{Untagged: true, KeepCount: ptr(0)}},
		"DaysOutOfRange":     {{Untagged: true, ExpireAfterDays: ptr(366)}},
		"TooManyRules":       tooManyRules(),
	}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := BuildLifecyclePolicy(rules); err == nil {
				t.Error("BuildLifecyclePolicy(): expected an error, got nil")
			}
		})
	}
}

func tooManyRules() []v1alpha1.LifecycleRule {
	rules := make([]v1alpha1.LifecycleRule, v1alpha1.MaxLifecycleRules+1)
	for i := range rules {
		rules[i] = v1alpha1.LifecycleRule{TagPrefixes: []string{"tag"}, KeepCount: ptr(1)}
	}
	return rules
}

func ptr[T any](value T) *T {
	return &value
}
