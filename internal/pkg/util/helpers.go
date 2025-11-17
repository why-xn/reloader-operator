/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	// Annotations used by Reloader
	AnnotationLastHash         = "reloader.stakater.com/last-hash"
	AnnotationAuto             = "reloader.stakater.com/auto"
	AnnotationSearch           = "reloader.stakater.com/search"
	AnnotationMatch            = "reloader.stakater.com/match"
	AnnotationIgnore           = "reloader.stakater.com/ignore"
	AnnotationRolloutStrategy  = "reloader.stakater.com/rollout-strategy"
	AnnotationLastReload       = "reloader.stakater.com/last-reload"
	AnnotationLastReloadedFrom = "reloader.stakater.com/last-reloaded-from"

	// Type-specific annotations
	AnnotationSecretReload    = "secret.reloader.stakater.com/reload"
	AnnotationSecretAuto      = "secret.reloader.stakater.com/auto"
	AnnotationConfigMapReload = "configmap.reloader.stakater.com/reload"
	AnnotationConfigMapAuto   = "configmap.reloader.stakater.com/auto"

	// Workload-specific annotations
	AnnotationDeploymentPausePeriod  = "deployment.reloader.stakater.com/pause-period"
	AnnotationStatefulSetPausePeriod = "statefulset.reloader.stakater.com/pause-period"
	AnnotationDaemonSetPausePeriod   = "daemonset.reloader.stakater.com/pause-period"

	// Environment variable name for reload trigger
	EnvReloaderTriggeredAt = "RELOADER_TRIGGERED_AT"
)

// Resource kinds supported by Reloader
const (
	KindSecret           = "Secret"
	KindConfigMap        = "ConfigMap"
	KindDeployment       = "Deployment"
	KindStatefulSet      = "StatefulSet"
	KindDaemonSet        = "DaemonSet"
	KindDeploymentConfig = "DeploymentConfig"
	KindRollout          = "Rollout"
	KindCronJob          = "CronJob"
)

// Rollout strategies (how to deploy the change)
const (
	RolloutStrategyRestart = "restart" // Delete pods directly (no template modification)
	RolloutStrategyRollout = "rollout" // Modify template and trigger rolling update
)

// Reload strategies (how to modify template when rollout strategy is "rollout")
const (
	ReloadStrategyEnvVars     = "env-vars"    // Update RELOADER_TRIGGERED_AT environment variable
	ReloadStrategyAnnotations = "annotations" // Update pod template annotations
)

// GetDefaultNamespace returns the namespace from target or falls back to default
func GetDefaultNamespace(targetNamespace, defaultNamespace string) string {
	if targetNamespace != "" {
		return targetNamespace
	}
	return defaultNamespace
}

// GetDefaultRolloutStrategy returns the rollout strategy from target or falls back to default
func GetDefaultRolloutStrategy(targetStrategy, defaultStrategy string) string {
	if targetStrategy != "" {
		return targetStrategy
	}
	if defaultStrategy != "" {
		return defaultStrategy
	}
	return RolloutStrategyRollout // ultimate default
}

// GetDefaultReloadStrategy returns the reload strategy from target or falls back to default
func GetDefaultReloadStrategy(targetStrategy, defaultStrategy string) string {
	if targetStrategy != "" {
		return targetStrategy
	}
	if defaultStrategy != "" {
		return defaultStrategy
	}
	return ReloadStrategyEnvVars // ultimate default
}

// ReloadSource represents the source resource that triggered a reload
type ReloadSource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Hash      string `json:"hash"`
}

// CreateReloadSourceAnnotation creates a JSON annotation value for the last reloaded resource
// This matches the original Reloader's behavior of storing metadata about the trigger resource
func CreateReloadSourceAnnotation(kind, name, namespace, hash string) string {
	source := ReloadSource{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
		Hash:      hash,
	}

	// Marshal to JSON - ignore errors and return empty string if it fails
	// (this is unlikely to fail with simple string fields)
	jsonData, err := json.Marshal(source)
	if err != nil {
		return ""
	}

	return string(jsonData)
}

// ParseCommaSeparatedList parses a comma-separated string into a list of trimmed strings
func ParseCommaSeparatedList(input string) []string {
	if input == "" {
		return []string{}
	}

	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// ParseDuration safely parses a duration string, returning zero duration on error
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// MakeResourceKey creates a unique key for a resource in format "namespace/kind/name"
func MakeResourceKey(namespace, kind, name string) string {
	return fmt.Sprintf("%s/%s/%s", namespace, kind, name)
}

// ParseResourceKey parses a resource key back into components
func ParseResourceKey(key string) (namespace, kind, name string, err error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid resource key format: %s", key)
	}
	return parts[0], parts[1], parts[2], nil
}

// ContainsString checks if a string slice contains a specific string
func ContainsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// IsSupportedWorkloadKind checks if the given kind is supported
func IsSupportedWorkloadKind(kind string) bool {
	supportedKinds := []string{
		KindDeployment,
		KindStatefulSet,
		KindDaemonSet,
		KindDeploymentConfig,
		KindRollout,
		KindCronJob,
	}
	return ContainsString(supportedKinds, kind)
}

// IsSupportedResourceKind checks if the given kind is a supported data resource
func IsSupportedResourceKind(kind string) bool {
	return kind == KindSecret || kind == KindConfigMap
}

// ShouldReloadOnCreate checks if reload should be triggered on resource creation
func ShouldReloadOnCreate(reloadOnCreate bool, annotations map[string]string) bool {
	if reloadOnCreate {
		return true
	}
	// Check annotation for backward compatibility
	if annotations != nil && annotations["reloader.stakater.com/reload-on-create"] == "true" {
		return true
	}
	return false
}

// ShouldReloadOnDelete checks if reload should be triggered on resource deletion
func ShouldReloadOnDelete(reloadOnDelete bool, annotations map[string]string) bool {
	if reloadOnDelete {
		return true
	}
	// Check annotation for backward compatibility
	if annotations != nil && annotations["reloader.stakater.com/reload-on-delete"] == "true" {
		return true
	}
	return false
}
