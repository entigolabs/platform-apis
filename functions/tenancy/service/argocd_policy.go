package service

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/entigolabs/platform-apis/apis/authz"
)

const (
	argoApplicationGroup = "argoproj.io"
	argoMetaGroup        = "argocd.meta.entigo.com"
	argoWildcard         = "*"
)

var (
	projectResources = map[string]map[string]string{
		argoApplicationGroup: {"applications": "applications", "applicationsets": "applicationsets"},
		argoMetaGroup:        {"logs": "logs", "exec": "exec", "repositories": "repositories", "clusters": "clusters"},
	}
	argoVerbActions = map[string]string{"get": "get", "list": "get", "watch": "get", "create": "create",
		"update": "update", "patch": "update", "delete": "delete", "sync": "sync", "override": "override",
		"invoke": "invoke", "action": "action/*", "*": "*"}
	casbinField = regexp.MustCompile(`^[A-Za-z0-9_.:/*@-]+$`)
)

func ProjectPolicies(zone, role string, rules []authz.Rule) ([]string, error) {
	subject := fmt.Sprintf("proj:%s:%s", zone, role)
	lines := make([]string, 0)
	for _, rule := range rules {
		if rule.Scope == authz.ScopeCluster {
			continue
		}
		for _, resource := range projectResourcesOf(rule) {
			for _, action := range argoActions(rule.Verbs) {
				line, err := casbinLine(subject, resource, action, fmt.Sprintf("%s/*", zone))
				if err != nil {
					return nil, fmt.Errorf("%w in role %s", err, role)
				}
				lines = append(lines, line)
			}
		}
	}
	slices.Sort(lines)
	return slices.Compact(lines), nil
}

func projectResourcesOf(rule authz.Rule) []string {
	groups := slices.DeleteFunc(slices.Clone(rule.APIGroups), func(group string) bool {
		_, known := projectResources[group]
		return !known
	})
	if slices.Contains(rule.APIGroups, argoWildcard) {
		groups = []string{argoApplicationGroup, argoMetaGroup}
	}
	names := make([]string, 0)
	for _, group := range groups {
		for resource, name := range projectResources[group] {
			if slices.Contains(rule.Resources, argoWildcard) || slices.Contains(rule.Resources, resource) {
				names = append(names, name)
			}
		}
	}
	return names
}

func argoActions(verbs []string) []string {
	mapped := make([]string, 0, len(verbs))
	for _, verb := range verbs {
		if action, found := argoVerbActions[verb]; found {
			mapped = append(mapped, action)
		}
	}
	return mapped
}

func casbinLine(subject, resource, action, object string) (string, error) {
	for _, field := range []string{subject, resource, action, object} {
		if !casbinField.MatchString(field) {
			return "", fmt.Errorf("invalid casbin field %q", field)
		}
	}
	return fmt.Sprintf("p, %s, %s, %s, %s, allow", subject, resource, action, object), nil
}
