package service

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/argocd"
	"github.com/entigolabs/platform-apis/apis/authz"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	RoleDefinitionKey = "RoleDefinitions"
	RoleGrantKey      = "RoleGrants"

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

func AuthzSelectors(settings apis.Authz) map[string]*fnv1.ResourceSelector {
	if !settings.Enabled() {
		return nil
	}
	return map[string]*fnv1.ResourceSelector{
		RoleDefinitionKey: {
			Kind:       "RoleDefinition",
			ApiVersion: authz.APIVersion,
			Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
		},
		RoleGrantKey: {
			Kind:       "RoleGrant",
			ApiVersion: authz.APIVersion,
			Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
		},
	}
}

func projectRolesFrom(zone string, settings apis.Authz, required map[string][]resource.Required,
	log logging.Logger) ([]argocd.ProjectRole, error) {
	if !settings.Enabled() {
		return nil, nil
	}
	definitions, err := extractRoleDefinitions(required)
	if err != nil {
		return nil, err
	}
	grants, err := extractRoleGrants(required)
	if err != nil {
		return nil, err
	}
	return ProjectRoles(zone, settings, definitions, grants, log), nil
}

func extractRoleDefinitions(required map[string][]resource.Required) ([]authz.RoleDefinition, error) {
	definitions := make([]authz.RoleDefinition, 0, len(required[RoleDefinitionKey]))
	for _, item := range required[RoleDefinitionKey] {
		var definition authz.RoleDefinition
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Resource.Object, &definition); err != nil {
			return nil, fmt.Errorf("cannot convert required resource %s: %w", RoleDefinitionKey, err)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func extractRoleGrants(required map[string][]resource.Required) ([]authz.RoleGrant, error) {
	grants := make([]authz.RoleGrant, 0, len(required[RoleGrantKey]))
	for _, item := range required[RoleGrantKey] {
		var grant authz.RoleGrant
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Resource.Object, &grant); err != nil {
			return nil, fmt.Errorf("cannot convert required resource %s: %w", RoleGrantKey, err)
		}
		grants = append(grants, grant)
	}
	return grants, nil
}

func ProjectRoles(zone string, settings apis.Authz, definitions []authz.RoleDefinition, grants []authz.RoleGrant,
	log logging.Logger) []argocd.ProjectRole {
	byName := make(map[string]authz.RoleDefinition, len(definitions))
	for _, definition := range definitions {
		byName[definition.Name] = definition
	}
	groups := make(map[string][]string)
	for _, grant := range grants {
		if _, defined := byName[grant.Spec.RoleRef]; !defined || grant.DeletionTimestamp != nil || !reachesZone(grant, zone) {
			continue
		}
		groups[grant.Spec.RoleRef] = append(groups[grant.Spec.RoleRef], grantEntry(grant, settings, zone))
	}
	roles := make([]argocd.ProjectRole, 0, len(groups))
	for _, role := range slices.Sorted(maps.Keys(groups)) {
		policies, err := ProjectPolicies(zone, role, byName[role].Spec.Rules)
		if err != nil {
			log.Info("Skipping an AppProject role with an invalid policy", "zone", zone, "role", role, "error", err)
			continue
		}
		if len(policies) == 0 {
			continue
		}
		entries := slices.Clone(groups[role])
		slices.Sort(entries)
		roles = append(roles, argocd.ProjectRole{
			Name:        role,
			Description: fmt.Sprintf("RoleDefinition %s", role),
			Groups:      slices.Compact(entries),
			Policies:    policies,
		})
	}
	return roles
}

func reachesZone(grant authz.RoleGrant, zone string) bool {
	zones := targetNames(grant, authz.ZoneGroup, authz.ZoneKind)
	return len(zones) == 0 || slices.Contains(zones, zone)
}

func grantEntry(grant authz.RoleGrant, settings apis.Authz, zone string) string {
	role := grant.Spec.RoleRef
	switch {
	case len(targetNames(grant, authz.ZoneGroup, authz.ZoneKind)) > 0:
		return strings.ToLower(fmt.Sprintf("%s@%s/%s/%s", role, settings.Organization, settings.Workspace, zone))
	case grant.Labels[authz.ReplicatedFromLabel] != "" &&
		len(targetNames(grant, authz.WorkspaceGroup, authz.WorkspaceKind)) == 0:
		return strings.ToLower(fmt.Sprintf("%s@%s", role, settings.Organization))
	}
	return strings.ToLower(fmt.Sprintf("%s@%s/%s", role, settings.Organization, settings.Workspace))
}

func targetNames(grant authz.RoleGrant, group, kind string) []string {
	names := make([]string, 0)
	for _, target := range grant.Spec.Resources {
		if target.Group == group && target.Kind == kind {
			names = append(names, target.Name)
		}
	}
	return names
}

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
