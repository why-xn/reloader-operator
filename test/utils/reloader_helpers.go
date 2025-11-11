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
	"os/exec"
	"strings"
	"time"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetPodUIDs returns UIDs of all pods matching a workload's selector
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

	// Get pod UIDs (only Running pods, which typically excludes terminating ones)
	cmd = exec.Command("kubectl", "get", "pods",
		"-n", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={.items[?(@.status.phase=='Running')].metadata.uid}")
	output, err = Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod UIDs: %w", err)
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return []string{}, nil
	}

	uids := strings.Fields(output)
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

// GetCondition retrieves a specific condition from ReloaderConfig status
func GetCondition(status *reloaderv1alpha1.ReloaderConfigStatus, conditionType string) *metav1.Condition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}
	return nil
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
