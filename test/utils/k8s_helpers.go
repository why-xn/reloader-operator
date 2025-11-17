//go:build e2e
// +build e2e

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

package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testNamespace = "test-reloader"

// ============================================================================
// Namespace Management
// ============================================================================

// SetupTestNamespace creates and returns test namespace
func SetupTestNamespace() string {
	cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
	_, _ = Run(cmd)
	return testNamespace
}

// CleanupTestNamespace deletes test namespace
// Respects E2E_SKIP_CLEANUP environment variable - if set to "true", cleanup is skipped
func CleanupTestNamespace() {
	if os.Getenv("E2E_SKIP_CLEANUP") == "true" {
		// Skip cleanup to allow troubleshooting
		return
	}
	cmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--wait=false")
	_, _ = Run(cmd)
}

// CleanupResourcesOnSuccess deletes specific resources only if the current test passed
// This allows keeping resources for troubleshooting when tests fail
// resourceTypes: map of resource type to list of resource names, e.g., {"deployment": {"app1", "app2"}, "secret": {"secret1"}}
func CleanupResourcesOnSuccess(namespace string, resourceTypes map[string][]string) {
	// Check if current spec failed
	report := CurrentSpecReport()
	if report.Failed() {
		_, _ = fmt.Fprintf(GinkgoWriter, "⚠️  Test failed - keeping resources in namespace '%s' for troubleshooting\n", namespace)
		return
	}

	// Test passed, clean up resources
	for resourceType, resourceNames := range resourceTypes {
		for _, resourceName := range resourceNames {
			cmd := exec.Command("kubectl", "delete", resourceType, resourceName, "-n", namespace, "--ignore-not-found=true", "--wait=false")
			_, _ = Run(cmd)
		}
	}
}

// ============================================================================
// Pod Management
// ============================================================================

// GetPodUIDs returns UIDs of all pods matching a workload's selector
// It retries to ensure all pods are Ready, not just Running, to avoid race conditions
func GetPodUIDs(namespace, workloadType, workloadName string) ([]string, error) {
	// Get label selector based on workload type
	cmd := exec.Command("kubectl", "get", workloadType, workloadName,
		"-n", namespace,
		"-o", "jsonpath={.spec.selector.matchLabels}")
	output, err := Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get matchLabels: %w", err)
	}

	// Parse matchLabels JSON to create selector
	var labels map[string]string
	if err := json.Unmarshal([]byte(output), &labels); err != nil {
		return nil, fmt.Errorf("failed to parse matchLabels: %w", err)
	}

	// Build selector string
	var selectors []string
	for k, v := range labels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	labelSelector := strings.Join(selectors, ",")

	// Get expected replica count
	cmd = exec.Command("kubectl", "get", workloadType, workloadName,
		"-n", namespace,
		"-o", "jsonpath={.spec.replicas}")
	replicaOutput, err := Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get replica count: %w", err)
	}
	expectedCount := 1 // Default to 1 if replicas is not set
	if replicaOutput != "" && strings.TrimSpace(replicaOutput) != "" {
		fmt.Sscanf(strings.TrimSpace(replicaOutput), "%d", &expectedCount)
	}

	// Retry logic: wait for all pods to be Ready
	// This handles the race condition where pods are Running but not yet Ready
	maxRetries := 3
	retryDelay := 3 * time.Second

	var uids []string
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		// First, explicitly wait for pods to be ready using kubectl wait
		// This ensures we don't get pods that are Running but not Ready
		waitCmd := exec.Command("kubectl", "wait", "pods",
			"-n", namespace,
			"-l", labelSelector,
			"--for=condition=Ready",
			fmt.Sprintf("--timeout=%ds", int(retryDelay.Seconds())))
		_, _ = Run(waitCmd) // Ignore errors, we'll check UIDs anyway

		// Now get pod UIDs for Running pods
		cmd = exec.Command("kubectl", "get", "pods",
			"-n", namespace,
			"-l", labelSelector,
			"--field-selector=status.phase=Running",
			"-o", "jsonpath={.items[*].metadata.uid}")
		output, err = Run(cmd)
		if err != nil {
			if attempt == maxRetries-1 {
				return nil, fmt.Errorf("failed to get pod UIDs: %w", err)
			}
			continue
		}

		output = strings.TrimSpace(output)
		if output == "" {
			if attempt == maxRetries-1 {
				return []string{}, nil
			}
			continue
		}

		uids = strings.Fields(output)

		// If we got the expected count, return immediately
		if len(uids) == expectedCount {
			return uids, nil
		}

		// If this is not the last attempt and we didn't get the expected count, retry
		if attempt < maxRetries-1 {
			continue
		}
	}

	// Return what we have, even if it's not the expected count
	// The test will fail if the counts don't match
	return uids, nil
}

// WaitForPodsReady waits for the specified number of pods matching the label selector to be ready
func WaitForPodsReady(namespace, labelSelector string, count int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %d pods to be ready with selector %s", count, labelSelector)
		case <-ticker.C:
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", namespace,
				"-l", labelSelector,
				"-o", "jsonpath={.items[?(@.status.phase=='Running')].metadata.name}")
			output, err := Run(cmd)
			if err != nil {
				continue
			}

			// jsonpath returns space-separated pod names, so split by spaces
			output = strings.TrimSpace(output)
			if output == "" {
				continue
			}
			readyPods := strings.Fields(output)
			if len(readyPods) >= count {
				return nil
			}
		}
	}
}

// WaitForPodsDeletion waits for pods with specific UIDs to be deleted
func WaitForPodsDeletion(namespace string, uids []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pods to be deleted")
		case <-ticker.C:
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", namespace,
				"-o", "jsonpath={.items[*].metadata.uid}")
			output, err := Run(cmd)
			if err != nil {
				continue
			}

			currentUIDs := strings.Fields(output)
			allDeleted := true
			for _, uid := range uids {
				for _, currentUID := range currentUIDs {
					if uid == currentUID {
						allDeleted = false
						break
					}
				}
				if !allDeleted {
					break
				}
			}

			if allDeleted {
				return nil
			}
		}
	}
}

// ============================================================================
// Workload Management
// ============================================================================

// WaitForRolloutComplete waits for a workload rollout to complete
func WaitForRolloutComplete(namespace, workloadType, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resourceType := workloadType
	// kubectl rollout status expects specific resource types
	switch workloadType {
	case "deployment", "deployments":
		resourceType = "deployment"
	case "statefulset", "statefulsets":
		resourceType = "statefulset"
	case "daemonset", "daemonsets":
		resourceType = "daemonset"
	}

	cmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
		fmt.Sprintf("%s/%s", resourceType, name),
		"-n", namespace,
		"--timeout", timeout.String())

	_, err := Run(cmd)
	return err
}

// GetWorkloadGeneration retrieves the generation number of a workload
func GetWorkloadGeneration(namespace, workloadType, name string) (int64, error) {
	cmd := exec.Command("kubectl", "get", workloadType, name,
		"-n", namespace,
		"-o", "jsonpath={.metadata.generation}")
	output, err := Run(cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to get generation: %w", err)
	}

	var generation int64
	_, err = fmt.Sscanf(output, "%d", &generation)
	if err != nil {
		return 0, fmt.Errorf("failed to parse generation: %w", err)
	}

	return generation, nil
}

// WaitForGenerationChange waits for a workload's generation to change from the initial value
func WaitForGenerationChange(namespace, workloadType, name string, initialGeneration int64, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for generation change")
		case <-ticker.C:
			currentGen, err := GetWorkloadGeneration(namespace, workloadType, name)
			if err != nil {
				continue
			}
			if currentGen > initialGeneration {
				return nil
			}
		}
	}
}

// GetPodTemplateEnvVars retrieves environment variables from a workload's pod template
func GetPodTemplateEnvVars(namespace, workloadType, name string) (map[string]string, error) {
	cmd := exec.Command("kubectl", "get", workloadType, name,
		"-n", namespace,
		"-o", "jsonpath={.spec.template.spec.containers[0].env}")
	output, err := Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod template env vars: %w", err)
	}

	var envVars []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal([]byte(output), &envVars); err != nil {
		return nil, fmt.Errorf("failed to parse env vars: %w", err)
	}

	result := make(map[string]string)
	for _, env := range envVars {
		result[env.Name] = env.Value
	}
	return result, nil
}

// GetPodTemplateAnnotations retrieves annotations from a workload's pod template
func GetPodTemplateAnnotations(namespace, workloadType, name string) (map[string]string, error) {
	cmd := exec.Command("kubectl", "get", workloadType, name,
		"-n", namespace,
		"-o", "jsonpath={.spec.template.metadata.annotations}")
	output, err := Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod template annotations: %w", err)
	}

	var annotations map[string]string
	if output == "" || output == "{}" {
		return map[string]string{}, nil
	}

	if err := json.Unmarshal([]byte(output), &annotations); err != nil {
		return nil, fmt.Errorf("failed to parse annotations: %w", err)
	}

	return annotations, nil
}

// ============================================================================
// ReloaderConfig Management
// ============================================================================

// GetReloaderConfigStatus retrieves the status of a ReloaderConfig
func GetReloaderConfigStatus(namespace, name string) (*reloaderv1alpha1.ReloaderConfigStatus, error) {
	cmd := exec.Command("kubectl", "get", "reloaderconfig", name,
		"-n", namespace,
		"-o", "json")
	output, err := Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get ReloaderConfig: %w", err)
	}

	var rc struct {
		Status reloaderv1alpha1.ReloaderConfigStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &rc); err != nil {
		return nil, fmt.Errorf("failed to parse ReloaderConfig: %w", err)
	}

	return &rc.Status, nil
}

// WaitForStatusUpdate waits for a ReloaderConfig status to satisfy the check function
func WaitForStatusUpdate(namespace, name string, checkFunc func(*reloaderv1alpha1.ReloaderConfigStatus) bool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for status update")
		case <-ticker.C:
			status, err := GetReloaderConfigStatus(namespace, name)
			if err != nil {
				continue
			}
			if checkFunc(status) {
				return nil
			}
		}
	}
}

// GetCondition retrieves a specific condition from ReloaderConfig status
func GetCondition(status *reloaderv1alpha1.ReloaderConfigStatus, conditionType string) *metav1.Condition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}
	return nil
}

// ============================================================================
// YAML Resource Operations
// ============================================================================

// ApplyYAML applies YAML content to the cluster
func ApplyYAML(yamlContent string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(yamlContent)
	_, err := Run(cmd)
	return err
}

// DeleteYAML deletes resources defined in YAML content from the cluster
func DeleteYAML(yamlContent string) error {
	cmd := exec.Command("kubectl", "delete", "-f", "-", "--ignore-not-found=true")
	cmd.Stdin = strings.NewReader(yamlContent)
	_, err := Run(cmd)
	return err
}

// ApplyFile applies a YAML file to the cluster
func ApplyFile(filepath string) error {
	cmd := exec.Command("kubectl", "apply", "-f", filepath)
	_, err := Run(cmd)
	return err
}

// DeleteFile deletes resources defined in a YAML file from the cluster
func DeleteFile(filepath string) error {
	cmd := exec.Command("kubectl", "delete", "-f", filepath, "--ignore-not-found=true")
	_, err := Run(cmd)
	return err
}

// ============================================================================
// YAML Generation Helpers
// ============================================================================

// GenerateUniqueResourceName creates unique names for test resources
func GenerateUniqueResourceName(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

// GenerateSecret creates a Secret YAML string
func GenerateSecret(name, namespace string, data map[string]string) string {
	yaml := fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
stringData:
`, name, namespace)

	for k, v := range data {
		yaml += fmt.Sprintf("  %s: |\n    %s\n", k, v)
	}

	return yaml
}

// GenerateConfigMap creates a ConfigMap YAML string
func GenerateConfigMap(name, namespace string, data map[string]string) string {
	yaml := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
`, name, namespace)

	for k, v := range data {
		yaml += fmt.Sprintf("  %s: |\n    %s\n", k, v)
	}

	return yaml
}

// DeploymentOpts contains options for generating a Deployment
type DeploymentOpts struct {
	Replicas      int
	Image         string
	Labels        map[string]string
	Annotations   map[string]string
	SecretName    string
	SecretKey     string
	ConfigMapName string
	ConfigMapKey  string
	EnvVarName    string
	VolumeMount   bool
	AdditionalEnv []map[string]string // Additional env vars: [{"name": "VAR", "valueFrom": "secret-name", "key": "key"}]
}

// GenerateDeployment creates a Deployment YAML string
func GenerateDeployment(name, namespace string, opts DeploymentOpts) string {
	if opts.Replicas == 0 {
		opts.Replicas = 2
	}
	if opts.Image == "" {
		opts.Image = "nginxinc/nginx-unprivileged:alpine"
	}
	if opts.Labels == nil {
		opts.Labels = map[string]string{"app": name}
	}

	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	// Add annotations if provided
	if len(opts.Annotations) > 0 {
		yaml += "  annotations:\n"
		for k, v := range opts.Annotations {
			yaml += fmt.Sprintf("    %s: \"%s\"\n", k, v)
		}
	}

	yaml += fmt.Sprintf(`spec:
  replicas: %d
  selector:
    matchLabels:
`, opts.Replicas)

	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("      %s: %s\n", k, v)
	}

	yaml += "  template:\n    metadata:\n      labels:\n"
	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("        %s: %s\n", k, v)
	}

	yaml += "    spec:\n      containers:\n"
	yaml += fmt.Sprintf("      - name: %s\n", name)
	yaml += fmt.Sprintf("        image: %s\n", opts.Image)
	yaml += `        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`

	// Add environment variables if secret or configmap is specified
	// Skip env vars if VolumeMount is true (volumes are used instead)
	if !opts.VolumeMount && (opts.SecretName != "" || opts.ConfigMapName != "" || len(opts.AdditionalEnv) > 0) {
		yaml += "        env:\n"

		if opts.SecretName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "SECRET_VALUE"
			}
			secretKey := opts.SecretKey
			if secretKey == "" {
				secretKey = "password"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            secretKeyRef:
              name: %s
              key: %s
`, envName, opts.SecretName, secretKey)
		}

		if opts.ConfigMapName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "CONFIG_VALUE"
			}
			configMapKey := opts.ConfigMapKey
			if configMapKey == "" {
				configMapKey = "config"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            configMapKeyRef:
              name: %s
              key: %s
`, envName, opts.ConfigMapName, configMapKey)
		}

		// Add additional environment variables
		for _, env := range opts.AdditionalEnv {
			envName := env["name"]
			valueFrom := env["valueFrom"]
			valueFromRef := env["valueFromRef"]
			key := env["key"]

			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            %s:
              name: %s
              key: %s
`, envName, valueFromRef, valueFrom, key)
		}
	}

	// Add volume mount if requested
	if opts.VolumeMount {
		hasVolumes := false

		if opts.SecretName != "" || opts.ConfigMapName != "" {
			yaml += "        volumeMounts:\n"

			if opts.SecretName != "" {
				yaml += "        - name: secret-volume\n"
				yaml += "          mountPath: /etc/secrets\n"
				hasVolumes = true
			}

			if opts.ConfigMapName != "" {
				yaml += "        - name: config-volume\n"
				yaml += "          mountPath: /etc/config\n"
				hasVolumes = true
			}
		}

		if hasVolumes {
			yaml += "      volumes:\n"

			if opts.SecretName != "" {
				yaml += "      - name: secret-volume\n"
				yaml += "        secret:\n"
				yaml += fmt.Sprintf("          secretName: %s\n", opts.SecretName)
			}

			if opts.ConfigMapName != "" {
				yaml += "      - name: config-volume\n"
				yaml += "        configMap:\n"
				yaml += fmt.Sprintf("          name: %s\n", opts.ConfigMapName)
			}
		}
	}

	return yaml
}

// StatefulSetOpts contains options for generating a StatefulSet
type StatefulSetOpts struct {
	Replicas      int
	Image         string
	Labels        map[string]string
	Annotations   map[string]string
	SecretName    string
	SecretKey     string
	ConfigMapName string
	ConfigMapKey  string
	EnvVarName    string
}

// GenerateStatefulSet creates a StatefulSet YAML string
func GenerateStatefulSet(name, namespace string, opts StatefulSetOpts) string {
	if opts.Replicas == 0 {
		opts.Replicas = 2
	}
	if opts.Image == "" {
		opts.Image = "nginxinc/nginx-unprivileged:alpine"
	}
	if opts.Labels == nil {
		opts.Labels = map[string]string{"app": name}
	}

	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	if len(opts.Annotations) > 0 {
		yaml += "  annotations:\n"
		for k, v := range opts.Annotations {
			yaml += fmt.Sprintf("    %s: \"%s\"\n", k, v)
		}
	}

	yaml += fmt.Sprintf(`spec:
  serviceName: %s
  replicas: %d
  selector:
    matchLabels:
`, name, opts.Replicas)

	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("      %s: %s\n", k, v)
	}

	yaml += "  template:\n    metadata:\n      labels:\n"
	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("        %s: %s\n", k, v)
	}

	yaml += "    spec:\n      containers:\n"
	yaml += fmt.Sprintf("      - name: %s\n", name)
	yaml += fmt.Sprintf("        image: %s\n", opts.Image)
	yaml += `        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`

	if opts.SecretName != "" || opts.ConfigMapName != "" {
		yaml += "        env:\n"

		if opts.SecretName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "SECRET_VALUE"
			}
			secretKey := opts.SecretKey
			if secretKey == "" {
				secretKey = "password"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            secretKeyRef:
              name: %s
              key: %s
`, envName, opts.SecretName, secretKey)
		}

		if opts.ConfigMapName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "CONFIG_VALUE"
			}
			configMapKey := opts.ConfigMapKey
			if configMapKey == "" {
				configMapKey = "config"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            configMapKeyRef:
              name: %s
              key: %s
`, envName, opts.ConfigMapName, configMapKey)
		}
	}

	return yaml
}

// DaemonSetOpts contains options for generating a DaemonSet
type DaemonSetOpts struct {
	Image         string
	Labels        map[string]string
	Annotations   map[string]string
	SecretName    string
	SecretKey     string
	ConfigMapName string
	ConfigMapKey  string
	EnvVarName    string
}

// GenerateDaemonSet creates a DaemonSet YAML string
func GenerateDaemonSet(name, namespace string, opts DaemonSetOpts) string {
	if opts.Image == "" {
		opts.Image = "nginxinc/nginx-unprivileged:alpine"
	}
	if opts.Labels == nil {
		opts.Labels = map[string]string{"app": name}
	}

	yaml := fmt.Sprintf(`apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: %s
  namespace: %s
`, name, namespace)

	if len(opts.Annotations) > 0 {
		yaml += "  annotations:\n"
		for k, v := range opts.Annotations {
			yaml += fmt.Sprintf("    %s: \"%s\"\n", k, v)
		}
	}

	yaml += `spec:
  selector:
    matchLabels:
`

	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("      %s: %s\n", k, v)
	}

	yaml += "  template:\n    metadata:\n      labels:\n"
	for k, v := range opts.Labels {
		yaml += fmt.Sprintf("        %s: %s\n", k, v)
	}

	yaml += "    spec:\n      containers:\n"
	yaml += fmt.Sprintf("      - name: %s\n", name)
	yaml += fmt.Sprintf("        image: %s\n", opts.Image)
	yaml += `        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
`

	if opts.SecretName != "" || opts.ConfigMapName != "" {
		yaml += "        env:\n"

		if opts.SecretName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "SECRET_VALUE"
			}
			secretKey := opts.SecretKey
			if secretKey == "" {
				secretKey = "password"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            secretKeyRef:
              name: %s
              key: %s
`, envName, opts.SecretName, secretKey)
		}

		if opts.ConfigMapName != "" {
			envName := opts.EnvVarName
			if envName == "" {
				envName = "CONFIG_VALUE"
			}
			configMapKey := opts.ConfigMapKey
			if configMapKey == "" {
				configMapKey = "config"
			}
			yaml += fmt.Sprintf(`        - name: %s
          valueFrom:
            configMapKeyRef:
              name: %s
              key: %s
`, envName, opts.ConfigMapName, configMapKey)
		}
	}

	return yaml
}

// ReloaderConfigSpec is a simplified spec for generating ReloaderConfig
type ReloaderConfigSpec struct {
	WatchedSecrets       []string
	WatchedConfigMaps    []string
	EnableTargetedReload bool
	Targets              []Target
	RolloutStrategy      string // How to deploy: "rollout" or "restart"
	ReloadStrategy       string // How to modify template: "env-vars" or "annotations" (only when rollout)
	AutoReloadAll        bool
}

// Target represents a workload target
type Target struct {
	Kind             string
	Name             string
	PausePeriod      string
	RequireReference bool
	RolloutStrategy  string // Override config-level rollout strategy
	ReloadStrategy   string // Override config-level reload strategy
}

// GenerateReloaderConfig creates a ReloaderConfig YAML string
func GenerateReloaderConfig(name, namespace string, spec ReloaderConfigSpec) string {
	yaml := fmt.Sprintf(`apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: %s
  namespace: %s
spec:
`, name, namespace)

	if len(spec.WatchedSecrets) > 0 || len(spec.WatchedConfigMaps) > 0 || spec.EnableTargetedReload {
		yaml += "  watchedResources:\n"
		if len(spec.WatchedSecrets) > 0 {
			yaml += "    secrets:\n"
			for _, secret := range spec.WatchedSecrets {
				yaml += fmt.Sprintf("    - %s\n", secret)
			}
		}
		if len(spec.WatchedConfigMaps) > 0 {
			yaml += "    configMaps:\n"
			for _, cm := range spec.WatchedConfigMaps {
				yaml += fmt.Sprintf("    - %s\n", cm)
			}
		}
		if spec.EnableTargetedReload {
			yaml += "    enableTargetedReload: true\n"
		}
	}

	if len(spec.Targets) > 0 {
		yaml += "  targets:\n"
		for _, target := range spec.Targets {
			yaml += fmt.Sprintf("  - kind: %s\n", target.Kind)
			yaml += fmt.Sprintf("    name: %s\n", target.Name)
			if target.PausePeriod != "" {
				yaml += fmt.Sprintf("    pausePeriod: %s\n", target.PausePeriod)
			}
			if target.RequireReference {
				yaml += "    requireReference: true\n"
			}
			if target.RolloutStrategy != "" {
				yaml += fmt.Sprintf("    rolloutStrategy: %s\n", target.RolloutStrategy)
			}
			if target.ReloadStrategy != "" {
				yaml += fmt.Sprintf("    reloadStrategy: %s\n", target.ReloadStrategy)
			}
		}
	}

	if spec.RolloutStrategy != "" {
		yaml += fmt.Sprintf("  rolloutStrategy: %s\n", spec.RolloutStrategy)
	}

	if spec.ReloadStrategy != "" {
		yaml += fmt.Sprintf("  reloadStrategy: %s\n", spec.ReloadStrategy)
	}

	if spec.AutoReloadAll {
		yaml += "  autoReloadAll: true\n"
	}

	return yaml
}

// AddAnnotation adds an annotation to a Kubernetes resource YAML
func AddAnnotation(yaml, key, value string) string {
	// Find the metadata section
	metadataIndex := strings.Index(yaml, "metadata:")
	if metadataIndex == -1 {
		return yaml
	}

	// Find the end of the metadata name line
	nameIndex := strings.Index(yaml[metadataIndex:], "  name:")
	if nameIndex == -1 {
		return yaml
	}

	// Find the end of that line
	nameLineEnd := strings.Index(yaml[metadataIndex+nameIndex:], "\n")
	if nameLineEnd == -1 {
		return yaml
	}

	insertPos := metadataIndex + nameIndex + nameLineEnd + 1

	// Check if annotations already exist
	annotationsIndex := strings.Index(yaml[metadataIndex:], "  annotations:")
	if annotationsIndex != -1 && annotationsIndex < nameIndex+nameLineEnd+100 {
		// Annotations section exists, find where to insert
		annotationsPos := metadataIndex + annotationsIndex
		annotationsLineEnd := strings.Index(yaml[annotationsPos:], "\n")
		insertPos = annotationsPos + annotationsLineEnd + 1
		annotation := fmt.Sprintf("    %s: \"%s\"\n", key, value)
		return yaml[:insertPos] + annotation + yaml[insertPos:]
	}

	// Annotations section doesn't exist, create it
	annotations := fmt.Sprintf("  annotations:\n    %s: \"%s\"\n", key, value)
	return yaml[:insertPos] + annotations + yaml[insertPos:]
}
