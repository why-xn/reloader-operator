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
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/workload"
)

// statusUpdateType defines the type of status update
type statusUpdateType string

const (
	statusUpdateTypeReloaderConfig statusUpdateType = "reloaderconfig"
	statusUpdateTypeTarget         statusUpdateType = "target"
)

// statusUpdateWorkItem represents a status update to be processed
type statusUpdateWorkItem struct {
	updateType        statusUpdateType
	configKey         client.ObjectKey
	resourceNamespace string
	resourceKind      string
	resourceName      string
	newHash           string
	target            *workload.Target
	errorMsg          string
}

// startStatusUpdateWorker runs a worker goroutine that processes status update queue items
func (r *ReloaderConfigReconciler) startStatusUpdateWorker() {
	for r.processNextStatusUpdate() {
	}
}

// processNextStatusUpdate processes a single item from the status update queue
func (r *ReloaderConfigReconciler) processNextStatusUpdate() bool {
	obj, shutdown := r.statusQueue.Get()
	if shutdown {
		return false
	}
	defer r.statusQueue.Done(obj)

	// Process the status update with retry logic
	err := r.processStatusUpdate(obj)
	if err != nil {
		// Retry the update on error
		if r.statusQueue.NumRequeues(obj) < 5 {
			log.Log.Error(err, "Error processing status update, retrying",
				"configKey", obj.configKey,
				"updateType", obj.updateType)
			r.statusQueue.AddRateLimited(obj)
		} else {
			// Max retries exceeded, give up
			log.Log.Error(err, "Max retries exceeded for status update",
				"configKey", obj.configKey,
				"updateType", obj.updateType)
			r.statusQueue.Forget(obj)
		}
		return true
	}

	// Success - remove from queue
	r.statusQueue.Forget(obj)
	return true
}

// processStatusUpdate performs the actual status update for a work item
func (r *ReloaderConfigReconciler) processStatusUpdate(workItem statusUpdateWorkItem) error {
	ctx := context.Background()

	// Fetch fresh ReloaderConfig to avoid conflicts
	config := &reloaderv1alpha1.ReloaderConfig{}
	if err := r.Get(ctx, workItem.configKey, config); err != nil {
		if apierrors.IsNotFound(err) {
			// Config was deleted, nothing to update
			return nil
		}
		return err
	}

	// Apply the update based on type
	switch workItem.updateType {
	case statusUpdateTypeReloaderConfig:
		return r.updateReloaderConfigStatusDirect(ctx, config, workItem.resourceNamespace, workItem.resourceKind, workItem.resourceName, workItem.newHash)
	case statusUpdateTypeTarget:
		return r.updateTargetStatusDirect(ctx, config, workItem.target, workItem.errorMsg)
	default:
		return fmt.Errorf("unknown status update type: %s", workItem.updateType)
	}
}

// updateReloaderConfigStatusDirect performs direct status update for ReloaderConfig-level fields
func (r *ReloaderConfigReconciler) updateReloaderConfigStatusDirect(ctx context.Context, config *reloaderv1alpha1.ReloaderConfig, resourceNamespace, resourceKind, resourceName, newHash string) error {
	if config.Status.WatchedResourceHashes == nil {
		config.Status.WatchedResourceHashes = make(map[string]string)
	}

	hashKey := fmt.Sprintf("%s/%s/%s", resourceNamespace, resourceKind, resourceName)

	// Empty hash signals deletion - remove the entry
	if newHash == "" {
		delete(config.Status.WatchedResourceHashes, hashKey)
	} else {
		// Update or add the hash
		config.Status.WatchedResourceHashes[hashKey] = newHash
	}

	config.Status.ReloadCount++

	now := metav1.Now()
	config.Status.LastReloadTime = &now

	return r.Status().Update(ctx, config)
}

// updateTargetStatusDirect performs direct status update for a specific target
func (r *ReloaderConfigReconciler) updateTargetStatusDirect(ctx context.Context, config *reloaderv1alpha1.ReloaderConfig, target *workload.Target, errorMsg string) error {
	// Find or create target status entry
	var targetStatus *reloaderv1alpha1.TargetWorkloadStatus
	for i := range config.Status.TargetStatus {
		if config.Status.TargetStatus[i].Kind == target.Kind &&
			config.Status.TargetStatus[i].Name == target.Name &&
			config.Status.TargetStatus[i].Namespace == target.Namespace {
			targetStatus = &config.Status.TargetStatus[i]
			break
		}
	}

	if targetStatus == nil {
		// Create new target status entry
		config.Status.TargetStatus = append(config.Status.TargetStatus, reloaderv1alpha1.TargetWorkloadStatus{
			Kind:      target.Kind,
			Name:      target.Name,
			Namespace: target.Namespace,
		})
		targetStatus = &config.Status.TargetStatus[len(config.Status.TargetStatus)-1]
	}

	// Update target status
	if errorMsg != "" {
		targetStatus.LastError = errorMsg
	} else {
		targetStatus.LastError = ""
		targetStatus.ReloadCount++
		now := metav1.Now()
		targetStatus.LastReloadTime = &now

		// Update pause period if configured
		if target.PausePeriod != "" {
			duration, err := time.ParseDuration(target.PausePeriod)
			if err == nil {
				pausedUntil := metav1.NewTime(now.Add(duration))
				targetStatus.PausedUntil = &pausedUntil
			}
		}
	}

	return r.Status().Update(ctx, config)
}

// removeReloaderConfigStatusEntries removes hash entries from ReloaderConfig statuses
//
// Business Logic:
// When a Secret/ConfigMap is deleted, we should remove its hash entry from all
// ReloaderConfig statuses that were tracking it. This keeps the status clean and
// prevents stale entries.
func (r *ReloaderConfigReconciler) removeReloaderConfigStatusEntries(
	ctx context.Context,
	configs []*reloaderv1alpha1.ReloaderConfig,
	resourceNamespace string,
	resourceKind string,
	resourceName string,
) {
	logger := log.FromContext(ctx)

	for _, config := range configs {
		// Create a key for this resource in the hash map
		hashKey := fmt.Sprintf("%s/%s/%s", resourceNamespace, resourceKind, resourceName)

		// Check if this resource is tracked in the status
		if _, exists := config.Status.WatchedResourceHashes[hashKey]; !exists {
			continue
		}

		// Queue status update to remove the hash entry
		r.statusQueue.Add(statusUpdateWorkItem{
			updateType:        statusUpdateTypeReloaderConfig,
			configKey:         client.ObjectKeyFromObject(config),
			resourceNamespace: resourceNamespace,
			resourceKind:      resourceKind,
			resourceName:      resourceName,
			newHash:           "", // Empty hash signals deletion
		})

		logger.V(1).Info("Queued status update to remove deleted resource hash",
			"config", config.Name,
			"resource", fmt.Sprintf("%s/%s", resourceKind, resourceName))
	}
}

// updateReloaderConfigStatuses updates status for all ReloaderConfigs that watched this resource
//
// Business Logic:
// After successfully reloading workloads, we update ReloaderConfig status to track:
// - New hash value (for the watched resource)
// - Total reload count (lifetime counter)
// - Last reload timestamp
//
// This provides observability into reload activity and history.
func (r *ReloaderConfigReconciler) updateReloaderConfigStatuses(
	ctx context.Context,
	configs []*reloaderv1alpha1.ReloaderConfig,
	resourceNamespace string,
	resourceKind string,
	resourceName string,
	newHash string,
) {
	// Enqueue status update work items instead of updating directly
	for _, config := range configs {
		configKey := client.ObjectKey{
			Namespace: config.Namespace,
			Name:      config.Name,
		}

		workItem := statusUpdateWorkItem{
			updateType:        statusUpdateTypeReloaderConfig,
			configKey:         configKey,
			resourceNamespace: resourceNamespace,
			resourceKind:      resourceKind,
			resourceName:      resourceName,
			newHash:           newHash,
		}

		r.statusQueue.Add(workItem)
	}
}

// updateTargetStatus updates the status for a specific target workload
func (r *ReloaderConfigReconciler) updateTargetStatus(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
	target workload.Target,
	errorMsg string,
) {
	// Enqueue status update work item instead of updating directly
	configKey := client.ObjectKey{
		Namespace: config.Namespace,
		Name:      config.Name,
	}

	workItem := statusUpdateWorkItem{
		updateType: statusUpdateTypeTarget,
		configKey:  configKey,
		target:     &target,
		errorMsg:   errorMsg,
	}

	r.statusQueue.Add(workItem)
}
