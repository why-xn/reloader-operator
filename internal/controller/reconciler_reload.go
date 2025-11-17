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

package controller

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/alerts"
	"github.com/stakater/Reloader/internal/pkg/workload"
)

// executeReloads triggers reload for all discovered target workloads
//
// Business Logic:
// For each target workload, this function:
//
// 1. Pause Check:
//   - Checks if the workload is in a pause period (rate limiting)
//   - If paused, skips reload to prevent reload storms
//   - Pause periods are configured per-target (e.g., pausePeriod: "5m")
//
// 2. Trigger Reload:
//   - Calls WorkloadUpdater.TriggerReload() which updates the workload
//   - Two strategies available:
//   - env-vars: Updates RELOADER_TRIGGERED_AT env var (forces pod restart)
//   - annotations: Updates pod template annotation (GitOps-friendly)
//
// 3. Alert Handling:
//   - On success: Sends success alert (if configured)
//   - On failure: Sends error alert with details (if configured)
//
// 4. Status Updates:
//   - Updates target-specific status in ReloaderConfig
//   - Tracks reload count, timestamp, and any errors
//
// Why we handle errors gracefully:
// If one target fails to reload, we continue with other targets.
// This prevents one bad workload from blocking all reloads.
//
// Returns the number of successful reloads.
func (r *ReloaderConfigReconciler) executeReloads(
	ctx context.Context,
	targets []workload.Target,
	resourceKind string,
	resourceName string,
	resourceNamespace string,
	resourceHash string,
) int {
	logger := log.FromContext(ctx)
	successCount := 0

	for _, target := range targets {
		// Refetch the ReloaderConfig to get the latest status (including PausedUntil)
		// This ensures we have fresh pause period information from the API server
		if target.Config != nil {
			freshConfig := &reloaderv1alpha1.ReloaderConfig{}
			configKey := client.ObjectKey{
				Name:      target.Config.Name,
				Namespace: target.Config.Namespace,
			}
			if err := r.Get(ctx, configKey, freshConfig); err != nil {
				logger.Error(err, "Failed to fetch fresh ReloaderConfig for pause check",
					"config", target.Config.Name)
				// Continue with existing config rather than blocking the reload
			} else {
				// Update the target's Config reference with fresh data
				target.Config = freshConfig
			}
		}

		// Check if workload is in pause period (rate limiting)
		isPaused, err := r.WorkloadUpdater.IsPaused(ctx, target)
		if err != nil {
			logger.Error(err, "Failed to check pause status", "workload", target.Name)
			continue
		}

		if isPaused {
			logger.Info("Skipping reload - workload is in pause period",
				"kind", target.Kind,
				"name", target.Name,
				"namespace", target.Namespace)
			continue
		}

		// Trigger the reload (rolling restart)
		err = r.WorkloadUpdater.TriggerReload(ctx, target, resourceKind, resourceName, resourceNamespace, resourceHash)
		if err != nil {
			// Reload failed - log error, send alert, update status
			logger.Error(err, "Failed to reload workload",
				"kind", target.Kind,
				"name", target.Name,
				"namespace", target.Namespace)

			r.handleReloadError(ctx, target, resourceKind, resourceName, err)
			continue
		}

		// Reload succeeded
		logger.Info("Successfully triggered reload",
			"kind", target.Kind,
			"name", target.Name,
			"namespace", target.Namespace,
			"strategy", target.ReloadStrategy)

		r.handleReloadSuccess(ctx, target, resourceKind, resourceName)
		successCount++
	}

	return successCount
}

// handleReloadError handles failed reload attempts
//
// Business Logic:
// When a reload fails (e.g., workload not found, API error):
// 1. Send error alert to configured channels (Slack, Teams, Google Chat)
// 2. Update target status with error message
//
// This provides immediate notification to operators when reloads fail.
func (r *ReloaderConfigReconciler) handleReloadError(
	ctx context.Context,
	target workload.Target,
	resourceKind string,
	resourceName string,
	reloadErr error,
) {
	logger := log.FromContext(ctx)

	// Send error alerts if alerting is globally enabled
	message := alerts.NewReloadErrorMessage(
		target.Kind,
		target.Name,
		target.Namespace,
		resourceKind,
		resourceName,
		target.ReloadStrategy,
		reloadErr.Error(),
	)
	message.Timestamp = time.Now()

	if alertErr := r.AlertManager.SendReloadAlert(ctx, message); alertErr != nil {
		logger.Error(alertErr, "Failed to send error alerts", "workload", target.Name)
	}

	// Update target status with error message
	if target.Config != nil {
		r.updateTargetStatus(ctx, target.Config, target, reloadErr.Error())
	}
}

// handleReloadSuccess handles successful reload attempts
//
// Business Logic:
// When a reload succeeds:
// 1. Send success alert to configured channels (optional, for audit trail)
// 2. Update target status (reload count, timestamp, clear any previous errors)
//
// Success alerts are useful for audit trails and monitoring reload frequency.
func (r *ReloaderConfigReconciler) handleReloadSuccess(
	ctx context.Context,
	target workload.Target,
	resourceKind string,
	resourceName string,
) {
	logger := log.FromContext(ctx)

	// Send success alerts if alerting is globally enabled
	message := alerts.NewReloadSuccessMessage(
		target.Kind,
		target.Name,
		target.Namespace,
		resourceKind,
		resourceName,
		target.ReloadStrategy,
	)
	message.Timestamp = time.Now()

	if err := r.AlertManager.SendReloadAlert(ctx, message); err != nil {
		logger.Error(err, "Failed to send success alerts", "workload", target.Name)
	}

	// Update target status (clears any previous error)
	if target.Config != nil {
		r.updateTargetStatus(ctx, target.Config, target, "")
	}
}

// executeDeleteReloads executes delete-specific reloads for all target workloads
//
// Business Logic:
// Similar to executeReloads, but uses the delete strategy which either:
// - env-vars strategy: REMOVES the environment variable that was previously added
// - annotations strategy: Sets the annotation to the hash of empty data
//
// This is called when a Secret/ConfigMap is deleted and --reload-on-delete is enabled.
func (r *ReloaderConfigReconciler) executeDeleteReloads(
	ctx context.Context,
	targets []workload.Target,
	resourceKind string,
	resourceName string,
) int {
	logger := log.FromContext(ctx)
	successCount := 0

	for _, target := range targets {
		// Refetch the ReloaderConfig to get the latest status (including PausedUntil)
		if target.Config != nil {
			freshConfig := &reloaderv1alpha1.ReloaderConfig{}
			configKey := client.ObjectKey{
				Name:      target.Config.Name,
				Namespace: target.Config.Namespace,
			}
			if err := r.Get(ctx, configKey, freshConfig); err != nil {
				logger.Error(err, "Failed to fetch fresh ReloaderConfig for pause check",
					"config", target.Config.Name)
			} else {
				target.Config = freshConfig
			}
		}

		// Check if workload is in pause period
		isPaused, err := r.WorkloadUpdater.IsPaused(ctx, target)
		if err != nil {
			logger.Error(err, "Failed to check pause status", "workload", target.Name)
			continue
		}

		if isPaused {
			logger.Info("Skipping delete reload - workload is in pause period",
				"kind", target.Kind,
				"name", target.Name,
				"namespace", target.Namespace)
			continue
		}

		// Trigger the delete reload (using delete strategy)
		err = r.WorkloadUpdater.TriggerDeleteReload(ctx, target, resourceKind, resourceName)
		if err != nil {
			logger.Error(err, "Failed to reload workload on delete",
				"kind", target.Kind,
				"name", target.Name,
				"namespace", target.Namespace)

			r.handleReloadError(ctx, target, resourceKind, resourceName, err)
			continue
		}

		// Reload succeeded
		logger.Info("Successfully triggered delete reload",
			"kind", target.Kind,
			"name", target.Name,
			"namespace", target.Namespace,
			"strategy", target.ReloadStrategy)

		r.handleReloadSuccess(ctx, target, resourceKind, resourceName)
		successCount++
	}

	return successCount
}
