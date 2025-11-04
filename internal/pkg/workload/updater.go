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
func (u *Updater) TriggerReload(ctx context.Context, target Target, resourceHash string) error {
	logger := log.FromContext(ctx)

	strategy := target.ReloadStrategy
	if strategy == "" {
		strategy = util.ReloadStrategyEnvVars
	}

	logger.Info("Triggering reload",
		"kind", target.Kind,
		"name", target.Name,
		"namespace", target.Namespace,
		"strategy", strategy)

	switch target.Kind {
	case util.KindDeployment:
		return u.reloadDeployment(ctx, target.Name, target.Namespace, strategy, resourceHash)

	case util.KindStatefulSet:
		return u.reloadStatefulSet(ctx, target.Name, target.Namespace, strategy, resourceHash)

	case util.KindDaemonSet:
		return u.reloadDaemonSet(ctx, target.Name, target.Namespace, strategy, resourceHash)

	default:
		return fmt.Errorf("unsupported workload kind: %s", target.Kind)
	}
}

// reloadDeployment triggers a rolling update of a Deployment
func (u *Updater) reloadDeployment(
	ctx context.Context,
	name, namespace, strategy, hash string,
) error {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, deployment); err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
	}

	// Apply the reload strategy
	if err := applyReloadStrategy(&deployment.Spec.Template, strategy, hash); err != nil {
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
	name, namespace, strategy, hash string,
) error {
	logger := log.FromContext(ctx)

	statefulSet := &appsv1.StatefulSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, statefulSet); err != nil {
		return fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Apply the reload strategy
	if err := applyReloadStrategy(&statefulSet.Spec.Template, strategy, hash); err != nil {
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
	name, namespace, strategy, hash string,
) error {
	logger := log.FromContext(ctx)

	daemonSet := &appsv1.DaemonSet{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := u.Get(ctx, key, daemonSet); err != nil {
		return fmt.Errorf("failed to get DaemonSet: %w", err)
	}

	// Apply the reload strategy
	if err := applyReloadStrategy(&daemonSet.Spec.Template, strategy, hash); err != nil {
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

// applyReloadStrategy applies the chosen reload strategy to a pod template
func applyReloadStrategy(template *corev1.PodTemplateSpec, strategy, hash string) error {
	timestamp := time.Now().Format(time.RFC3339)

	switch strategy {
	case util.ReloadStrategyEnvVars:
		return applyEnvVarsStrategy(template, timestamp, hash)

	case util.ReloadStrategyAnnotations:
		return applyAnnotationsStrategy(template, timestamp, hash)

	default:
		return fmt.Errorf("unknown reload strategy: %s", strategy)
	}
}

// applyEnvVarsStrategy updates a dummy environment variable to trigger pod restart
func applyEnvVarsStrategy(template *corev1.PodTemplateSpec, timestamp, hash string) error {
	// Ensure we have at least one container
	if len(template.Spec.Containers) == 0 {
		return fmt.Errorf("no containers found in pod template")
	}

	// Update the first container's environment variable
	container := &template.Spec.Containers[0]

	// Find or add the RELOADER_TRIGGERED_AT env var
	found := false
	for i, env := range container.Env {
		if env.Name == util.EnvReloaderTriggeredAt {
			container.Env[i].Value = timestamp
			found = true
			break
		}
	}

	if !found {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  util.EnvReloaderTriggeredAt,
			Value: timestamp,
		})
	}

	// Also add hash annotation to pod template for tracking
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}
	template.Annotations[util.AnnotationResourceHash] = hash

	return nil
}

// applyAnnotationsStrategy updates pod template annotations to trigger pod restart
func applyAnnotationsStrategy(template *corev1.PodTemplateSpec, timestamp, hash string) error {
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}

	// Update annotations
	template.Annotations[util.AnnotationLastReload] = timestamp
	template.Annotations[util.AnnotationResourceHash] = hash

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

	// If no ReloaderConfig, we can't track pause state
	if target.Config == nil {
		return false, nil
	}

	// Check target status for pause expiration
	for _, status := range target.Config.Status.TargetStatus {
		if status.Kind == target.Kind &&
			status.Name == target.Name &&
			status.Namespace == target.Namespace {

			if status.PausedUntil != nil && status.PausedUntil.After(time.Now()) {
				return true, nil
			}
		}
	}

	// Not paused, but we should set pause for future reloads
	// This will be done when updating the status after successful reload
	_ = duration // Will use this when updating status

	return false, nil
}
