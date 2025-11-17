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

package workload

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/stakater/Reloader/internal/pkg/util"
)

// Updater handles workload updates (rolling restarts)
type Updater struct {
	client.Client
}

// NewUpdater creates a new workload updater
func NewUpdater(c client.Client) *Updater {
	return &Updater{Client: c}
}

// TriggerReload triggers a rolling update of a workload
func (u *Updater) TriggerReload(ctx context.Context, target Target, resourceKind, resourceName, resourceNamespace, resourceHash string) error {
	logger := log.FromContext(ctx)

	// Default rollout strategy to "rollout" if not specified
	rolloutStrategy := target.RolloutStrategy
	if rolloutStrategy == "" {
		rolloutStrategy = util.RolloutStrategyRollout
	}

	logger.Info("Triggering reload",
		"kind", target.Kind,
		"name", target.Name,
		"namespace", target.Namespace,
		"rolloutStrategy", rolloutStrategy,
		"reloadStrategy", target.ReloadStrategy,
		"triggerResource", fmt.Sprintf("%s/%s", resourceKind, resourceName))

	// Check rollout strategy first
	if rolloutStrategy == util.RolloutStrategyRestart {
		// Restart strategy: Delete pods directly without modifying template
		return u.triggerRestartRollout(ctx, target)
	}

	// Rollout strategy: Modify template based on reload strategy
	// Default reload strategy to "env-vars" if not specified
	reloadStrategy := target.ReloadStrategy
	if reloadStrategy == "" {
		reloadStrategy = util.ReloadStrategyEnvVars
	}

	// Create reload source JSON annotation
	reloadSourceJSON := util.CreateReloadSourceAnnotation(resourceKind, resourceName, resourceNamespace, resourceHash)

	var err error
	switch target.Kind {
	case util.KindDeployment:
		err = u.reloadDeployment(ctx, target.Name, target.Namespace, reloadStrategy, reloadSourceJSON, resourceKind, resourceName, resourceHash)

	case util.KindStatefulSet:
		err = u.reloadStatefulSet(ctx, target.Name, target.Namespace, reloadStrategy, reloadSourceJSON, resourceKind, resourceName, resourceHash)

	case util.KindDaemonSet:
		err = u.reloadDaemonSet(ctx, target.Name, target.Namespace, reloadStrategy, reloadSourceJSON, resourceKind, resourceName, resourceHash)

	default:
		return fmt.Errorf("unsupported workload kind: %s", target.Kind)
	}

	// If reload succeeded and this is an annotation-based workload, set last reload timestamp
	if err == nil && target.Config == nil && target.PausePeriod != "" {
		if annotErr := u.setLastReloadAnnotation(ctx, target); annotErr != nil {
			logger.Error(annotErr, "Failed to set last reload annotation",
				"kind", target.Kind,
				"name", target.Name)
			// Don't fail the reload if annotation update fails
		}
	}

	return err
}

// triggerRestartRollout handles the restart rollout strategy by deleting pods directly
func (u *Updater) triggerRestartRollout(ctx context.Context, target Target) error {
	logger := log.FromContext(ctx)

	// Get the workload to access its selector
	obj, err := u.getWorkload(ctx, target)
	if err != nil {
		return err
	}

	var selector *metav1.LabelSelector
	switch target.Kind {
	case util.KindDeployment:
		deployment := obj.(*appsv1.Deployment)
		selector = deployment.Spec.Selector
	case util.KindStatefulSet:
		statefulSet := obj.(*appsv1.StatefulSet)
		selector = statefulSet.Spec.Selector
	case util.KindDaemonSet:
		daemonSet := obj.(*appsv1.DaemonSet)
		selector = daemonSet.Spec.Selector
	default:
		return fmt.Errorf("unsupported workload kind: %s", target.Kind)
	}

	logger.Info("Using restart rollout strategy - deleting pods",
		"kind", target.Kind,
		"name", target.Name,
		"namespace", target.Namespace)

	return u.restartWorkloadPods(ctx, selector, target.Namespace, target.Kind, target.Name)
}

// TriggerDeleteReload handles workload reload when a Secret/ConfigMap is deleted
//
// Business Logic:
// When a Secret/ConfigMap is deleted:
// - If rollout strategy is "restart": Delete pods directly
// - If rollout strategy is "rollout": Modify template to trigger reload
//   - env-vars strategy: Update resource-specific environment variable (e.g., STAKATER_DB_CREDENTIALS_SECRET)
//   - annotations strategy: Update pod template annotation
func (u *Updater) TriggerDeleteReload(ctx context.Context, target Target, resourceKind, resourceName string) error {
	logger := log.FromContext(ctx)

	// Default rollout strategy to "rollout" if not specified
	rolloutStrategy := target.RolloutStrategy
	if rolloutStrategy == "" {
		rolloutStrategy = util.RolloutStrategyRollout
	}

	logger.Info("Triggering delete reload",
		"kind", target.Kind,
		"name", target.Name,
		"namespace", target.Namespace,
		"rolloutStrategy", rolloutStrategy,
		"deletedResource", fmt.Sprintf("%s/%s", resourceKind, resourceName))

	// Check rollout strategy first
	if rolloutStrategy == util.RolloutStrategyRestart {
		// Restart strategy: Delete pods directly without modifying template
		return u.triggerRestartRollout(ctx, target)
	}

	// Rollout strategy: Modify template based on reload strategy
	reloadStrategy := target.ReloadStrategy
	if reloadStrategy == "" {
		reloadStrategy = util.ReloadStrategyEnvVars
	}

	var err error
	switch target.Kind {
	case util.KindDeployment:
		err = u.reloadDeleteDeployment(ctx, target.Name, target.Namespace, reloadStrategy, resourceKind, resourceName)

	case util.KindStatefulSet:
		err = u.reloadDeleteStatefulSet(ctx, target.Name, target.Namespace, reloadStrategy, resourceKind, resourceName)

	case util.KindDaemonSet:
		err = u.reloadDeleteDaemonSet(ctx, target.Name, target.Namespace, reloadStrategy, resourceKind, resourceName)

	default:
		return fmt.Errorf("unsupported workload kind: %s", target.Kind)
	}

	// If reload succeeded and this is an annotation-based workload, set last reload timestamp
	if err == nil && target.Config == nil && target.PausePeriod != "" {
		if annotErr := u.setLastReloadAnnotation(ctx, target); annotErr != nil {
			logger.Error(annotErr, "Failed to set last reload annotation",
				"kind", target.Kind,
				"name", target.Name)
			// Don't fail the reload if annotation update fails
		}
	}

	return err
}

// reloadDeployment triggers a rolling update of a Deployment
func (u *Updater) reloadDeployment(
	ctx context.Context,
	name, namespace, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash string,
) error {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
	}

	// Apply the reload strategy (env-vars or annotations)
	if err := applyReloadStrategy(&deployment.Spec.Template, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash); err != nil {
		return err
	}

	// Update the Deployment
	if err := u.Update(ctx, deployment); err != nil {
		return fmt.Errorf("failed to update Deployment: %w", err)
	}

	logger.Info("Successfully triggered Deployment reload",
		"deployment", name,
		"strategy", strategy)

	return nil
}

// reloadStatefulSet triggers a rolling update of a StatefulSet
func (u *Updater) reloadStatefulSet(
	ctx context.Context,
	name, namespace, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash string,
) error {
	logger := log.FromContext(ctx)

	statefulSet := &appsv1.StatefulSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, statefulSet); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Apply the reload strategy (env-vars or annotations)
	if err := applyReloadStrategy(&statefulSet.Spec.Template, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash); err != nil {
		return err
	}

	// Update the StatefulSet
	if err := u.Update(ctx, statefulSet); err != nil {
		return fmt.Errorf("failed to update StatefulSet: %w", err)
	}

	logger.Info("Successfully triggered StatefulSet reload",
		"statefulset", name,
		"strategy", strategy)

	return nil
}

// reloadDaemonSet triggers a rolling update of a DaemonSet
func (u *Updater) reloadDaemonSet(
	ctx context.Context,
	name, namespace, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash string,
) error {
	logger := log.FromContext(ctx)

	daemonSet := &appsv1.DaemonSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, daemonSet); err != nil {
		return fmt.Errorf("failed to get DaemonSet: %w", err)
	}

	// Apply the reload strategy (env-vars or annotations)
	if err := applyReloadStrategy(&daemonSet.Spec.Template, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash); err != nil {
		return err
	}

	// Update the DaemonSet
	if err := u.Update(ctx, daemonSet); err != nil {
		return fmt.Errorf("failed to update DaemonSet: %w", err)
	}

	logger.Info("Successfully triggered DaemonSet reload",
		"daemonset", name,
		"strategy", strategy)

	return nil
}

// reloadDeleteDeployment triggers a rolling update of a Deployment using delete strategy
func (u *Updater) reloadDeleteDeployment(
	ctx context.Context,
	name, namespace, strategy string,
	resourceKind, resourceName string,
) error {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
	}

	// Apply the delete strategy (remove env var or set empty hash annotation)
	if err := applyDeleteStrategy(&deployment.Spec.Template, strategy, resourceKind, resourceName); err != nil {
		return err
	}

	// Update the Deployment
	if err := u.Update(ctx, deployment); err != nil {
		return fmt.Errorf("failed to update Deployment: %w", err)
	}

	logger.Info("Successfully triggered Deployment delete reload",
		"deployment", name,
		"strategy", strategy)

	return nil
}

// reloadDeleteStatefulSet triggers a rolling update of a StatefulSet using delete strategy
func (u *Updater) reloadDeleteStatefulSet(
	ctx context.Context,
	name, namespace, strategy string,
	resourceKind, resourceName string,
) error {
	logger := log.FromContext(ctx)

	statefulSet := &appsv1.StatefulSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, statefulSet); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Apply the delete strategy
	if err := applyDeleteStrategy(&statefulSet.Spec.Template, strategy, resourceKind, resourceName); err != nil {
		return err
	}

	// Update the StatefulSet
	if err := u.Update(ctx, statefulSet); err != nil {
		return fmt.Errorf("failed to update StatefulSet: %w", err)
	}

	logger.Info("Successfully triggered StatefulSet delete reload",
		"statefulset", name,
		"strategy", strategy)

	return nil
}

// reloadDeleteDaemonSet triggers a rolling update of a DaemonSet using delete strategy
func (u *Updater) reloadDeleteDaemonSet(
	ctx context.Context,
	name, namespace, strategy string,
	resourceKind, resourceName string,
) error {
	logger := log.FromContext(ctx)

	daemonSet := &appsv1.DaemonSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, daemonSet); err != nil {
		return fmt.Errorf("failed to get DaemonSet: %w", err)
	}

	// Apply the delete strategy
	if err := applyDeleteStrategy(&daemonSet.Spec.Template, strategy, resourceKind, resourceName); err != nil {
		return err
	}

	// Update the DaemonSet
	if err := u.Update(ctx, daemonSet); err != nil {
		return fmt.Errorf("failed to update DaemonSet: %w", err)
	}

	logger.Info("Successfully triggered DaemonSet delete reload",
		"daemonset", name,
		"strategy", strategy)

	return nil
}

// applyReloadStrategy applies the chosen reload strategy to a pod template
func applyReloadStrategy(template *corev1.PodTemplateSpec, strategy, reloadSourceJSON, resourceKind, resourceName, resourceHash string) error {
	timestamp := time.Now().Format(time.RFC3339)

	switch strategy {
	case util.ReloadStrategyEnvVars:
		return applyEnvVarsStrategy(template, resourceKind, resourceName, resourceHash)

	case util.ReloadStrategyAnnotations:
		return applyAnnotationsStrategy(template, timestamp, reloadSourceJSON)

	default:
		return fmt.Errorf("unknown reload strategy: %s", strategy)
	}
}

// applyEnvVarsStrategy updates environment variable based on the changed resource to trigger pod restart
// This matches the original Reloader's behavior of creating resource-specific env vars
func applyEnvVarsStrategy(template *corev1.PodTemplateSpec, resourceKind, resourceName, resourceHash string) error {
	// Ensure we have at least one container
	if len(template.Spec.Containers) == 0 {
		return fmt.Errorf("no containers found in pod template")
	}

	// Generate the environment variable name based on the resource
	// Format: STAKATER_<RESOURCE_NAME>_<TYPE>
	// Example: STAKATER_DB_CREDENTIALS_SECRET or STAKATER_APP_CONFIG_CONFIGMAP
	envVarName := util.GetEnvVarName(resourceKind, resourceName)

	// Update the first container's environment variable
	container := &template.Spec.Containers[0]

	// Find or add the env var
	found := false
	for i, env := range container.Env {
		if env.Name == envVarName {
			container.Env[i].Value = resourceHash
			found = true
			break
		}
	}

	if !found {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envVarName,
			Value: resourceHash,
		})
	}

	return nil
}

// applyAnnotationsStrategy updates pod template annotations to trigger pod restart
func applyAnnotationsStrategy(template *corev1.PodTemplateSpec, timestamp, reloadSourceJSON string) error {
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}

	// Update annotations
	template.Annotations[util.AnnotationLastReload] = timestamp
	template.Annotations[util.AnnotationLastReloadedFrom] = reloadSourceJSON

	return nil
}

// applyDeleteStrategy applies the delete reload strategy to a pod template
//
// Business Logic:
// When a Secret/ConfigMap is deleted, we need to trigger a pod restart:
// - env-vars strategy: Update the resource-specific environment variable with a "deleted" marker
// - annotations strategy: Set the annotation to timestamp (triggers restart via annotation change)
//
// Both strategies ensure a rolling restart by making a change to the pod template.
func applyDeleteStrategy(template *corev1.PodTemplateSpec, strategy, resourceKind, resourceName string) error {
	timestamp := time.Now().Format(time.RFC3339)

	switch strategy {
	case util.ReloadStrategyEnvVars:
		return applyDeleteEnvVarsStrategy(template, resourceKind, resourceName, timestamp)

	case util.ReloadStrategyAnnotations:
		return applyDeleteAnnotationsStrategy(template, timestamp)

	default:
		return fmt.Errorf("unknown reload strategy: %s", strategy)
	}
}

// applyDeleteEnvVarsStrategy updates the resource-specific environment variable to indicate deletion
// to trigger a rolling restart when a resource is deleted
// This matches the original Reloader's behavior of using resource-specific env vars
func applyDeleteEnvVarsStrategy(template *corev1.PodTemplateSpec, resourceKind, resourceName, timestamp string) error {
	// Ensure we have at least one container
	if len(template.Spec.Containers) == 0 {
		return fmt.Errorf("no containers found in pod template")
	}

	// Generate the environment variable name based on the resource
	// Format: STAKATER_<RESOURCE_NAME>_<TYPE>
	// Example: STAKATER_DB_CREDENTIALS_SECRET or STAKATER_APP_CONFIG_CONFIGMAP
	envVarName := util.GetEnvVarName(resourceKind, resourceName)

	// Set the resource-specific env var to "deleted" marker to trigger a restart
	// We use "deleted" as the value to distinguish from normal reloads
	container := &template.Spec.Containers[0]

	// Find and update the env var, or add it if it doesn't exist
	found := false
	for i, env := range container.Env {
		if env.Name == envVarName {
			container.Env[i].Value = "deleted-" + timestamp
			found = true
			break
		}
	}

	// If not found, add it
	if !found {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  envVarName,
			Value: "deleted-" + timestamp,
		})
	}

	// Remove hash annotation from pod template
	if template.Annotations != nil {
		delete(template.Annotations, util.AnnotationLastReloadedFrom)
	}

	return nil
}

// applyDeleteAnnotationsStrategy sets annotations to indicate deletion
func applyDeleteAnnotationsStrategy(template *corev1.PodTemplateSpec, timestamp string) error {
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}

	// Update annotation with timestamp to trigger restart
	// We use the timestamp to force a change even if the resource is recreated
	template.Annotations[util.AnnotationLastReload] = timestamp

	// Remove the resource hash annotation (resource no longer exists)
	delete(template.Annotations, util.AnnotationLastReloadedFrom)

	return nil
}

// IsPaused checks if a workload is in a pause period
func (u *Updater) IsPaused(ctx context.Context, target Target) (bool, error) {
	if target.PausePeriod == "" {
		return false, nil
	}

	// Parse duration
	duration, err := util.ParseDuration(target.PausePeriod)
	if err != nil {
		return false, fmt.Errorf("invalid pause period: %w", err)
	}

	// For ReloaderConfig-based targets, check status
	if target.Config != nil {
		for _, status := range target.Config.Status.TargetStatus {
			if status.Kind == target.Kind &&
				status.Name == target.Name &&
				status.Namespace == target.Namespace {

				if status.PausedUntil != nil && status.PausedUntil.After(time.Now()) {
					return true, nil
				}
			}
		}
		return false, nil
	}

	// For annotation-based targets, check the last reload annotation on the workload
	obj, err := u.getWorkload(ctx, target)
	if err != nil {
		return false, err
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false, nil
	}

	lastReloadStr, exists := annotations[util.AnnotationLastReload]
	if !exists {
		return false, nil
	}

	// Parse the last reload timestamp
	lastReload, err := time.Parse(time.RFC3339, lastReloadStr)
	if err != nil {
		// Invalid timestamp, treat as not paused
		return false, nil
	}

	// Check if still within pause period
	pausedUntil := lastReload.Add(duration)
	return pausedUntil.After(time.Now()), nil
}

// getWorkload retrieves the workload object based on target kind
func (u *Updater) getWorkload(ctx context.Context, target Target) (client.Object, error) {
	key := client.ObjectKey{
		Name:      target.Name,
		Namespace: target.Namespace,
	}

	switch target.Kind {
	case util.KindDeployment:
		obj := &appsv1.Deployment{}
		if err := u.Client.Get(ctx, key, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case util.KindStatefulSet:
		obj := &appsv1.StatefulSet{}
		if err := u.Client.Get(ctx, key, obj); err != nil {
			return nil, err
		}
		return obj, nil
	case util.KindDaemonSet:
		obj := &appsv1.DaemonSet{}
		if err := u.Client.Get(ctx, key, obj); err != nil {
			return nil, err
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", target.Kind)
	}
}

// setLastReloadAnnotation sets the last reload timestamp annotation on the workload
func (u *Updater) setLastReloadAnnotation(ctx context.Context, target Target) error {
	obj, err := u.getWorkload(ctx, target)
	if err != nil {
		return err
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[util.AnnotationLastReload] = time.Now().Format(time.RFC3339)
	obj.SetAnnotations(annotations)

	return u.Client.Update(ctx, obj)
}

// restartWorkloadPods deletes all pods for a workload, triggering recreation with updated configs
// This implements the "restart" strategy which is most GitOps-friendly as it doesn't modify templates
func (u *Updater) restartWorkloadPods(
	ctx context.Context,
	selector *metav1.LabelSelector,
	namespace, kind, name string,
) error {
	logger := log.FromContext(ctx)

	// Convert label selector to client.MatchingLabels
	if selector == nil || selector.MatchLabels == nil {
		return fmt.Errorf("workload has no label selector")
	}

	// List all pods matching the workload's selector
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(selector.MatchLabels),
	}

	if err := u.List(ctx, podList, listOpts...); err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(podList.Items) == 0 {
		logger.Info("No pods found to restart",
			"kind", kind,
			"name", name,
			"namespace", namespace)
		return nil
	}

	// Delete each pod - Kubernetes will recreate them
	deletedCount := 0
	for i := range podList.Items {
		pod := &podList.Items[i]

		if err := u.Delete(ctx, pod); err != nil {
			logger.Error(err, "Failed to delete pod",
				"pod", pod.Name,
				"namespace", namespace)
			// Continue with other pods even if one fails
			continue
		}
		deletedCount++
		logger.V(1).Info("Deleted pod for restart",
			"pod", pod.Name,
			"namespace", namespace)
	}

	logger.Info("Successfully triggered workload restart",
		"kind", kind,
		"name", name,
		"namespace", namespace,
		"podsDeleted", deletedCount,
		"totalPods", len(podList.Items))

	if deletedCount == 0 {
		return fmt.Errorf("failed to delete any pods")
	}

	return nil
}
