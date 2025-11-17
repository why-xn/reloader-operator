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

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stakater/Reloader/internal/pkg/util"
)

// reconcileResourceUpdate is a generic function that handles Secret/ConfigMap updates
func (r *ReloaderConfigReconciler) reconcileResourceUpdate(
	ctx context.Context,
	resourceKind string,
	obj client.Object,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	resourceName := obj.GetName()
	resourceNamespace := obj.GetNamespace()
	resourceTypeName := resourceKind // "Secret" or "ConfigMap" for logging

	// Phase 0: Check if resource should be ignored
	annotations := obj.GetAnnotations()
	if annotations != nil && annotations[util.AnnotationIgnore] == "true" {
		logger.V(1).Info(resourceTypeName+" marked as ignored, skipping reload",
			"name", resourceName,
			"namespace", resourceNamespace)
		return ctrl.Result{}, nil
	}

	// Phase 1: Check if resource data actually changed (hash-based change detection)
	currentHash, err := util.GetResourceDataAndHash(obj)
	if err != nil {
		return ctrl.Result{}, err
	}
	storedHash := r.getStoredHash(annotations)

	if currentHash == storedHash {
		// Hash matches - no actual change, skip reload
		logger.V(1).Info(resourceTypeName+" data unchanged, skipping reload", "hash", currentHash)
		return ctrl.Result{}, nil
	}

	logger.Info(resourceTypeName+" data changed", "oldHash", storedHash, "newHash", currentHash)

	// Phase 2: Discover all workloads that need to be reloaded
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, resourceKind, resourceName, resourceNamespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload",
		"resource", resourceTypeName+"/"+resourceName,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Phase 2.5: Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, resourceKind, resourceName, resourceNamespace)
	logger.Info("Filtered targets for targeted reload",
		"resource", resourceTypeName+"/"+resourceName,
		"before", len(allTargets),
		"after", len(filteredTargets))

	// Phase 3: Execute reloads for filtered targets
	successCount := r.executeReloads(ctx, filteredTargets, resourceKind, resourceName, resourceNamespace, currentHash)

	// Phase 4: Update ReloaderConfig statuses (only if at least one reload succeeded)
	if successCount > 0 {
		r.updateReloaderConfigStatuses(ctx, reloaderConfigs, resourceNamespace, resourceKind, resourceName, currentHash)
	}

	// Phase 5: Persist new hash in resource annotation for future comparisons
	if err := r.updateResourceHash(ctx, obj, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info(resourceTypeName+" reconciliation complete", "hash", currentHash, "reloadedTargets", len(allTargets))
	return ctrl.Result{}, nil
}

// reconcileSecret handles Secret changes and triggers workload reloads
//
// Business Logic Flow:
// This is the core reload logic that runs when a Secret changes.
//
// 1. Hash Comparison:
//   - Calculate SHA256 hash of Secret.Data
//   - Compare with previously stored hash (in annotation)
//   - If hashes match, no actual change occurred → skip reload (prevents reload storms)
//
// 2. Target Discovery:
//   - Find ReloaderConfigs that watch this Secret (CRD-based)
//   - Find Deployments/StatefulSets/DaemonSets with reload annotations (annotation-based)
//   - Merge both lists (supports hybrid configuration)
//
// 3. Reload Execution:
//   - For each target workload:
//     a. Check if it's in pause period (rate limiting)
//     b. Trigger rolling restart (via env-vars or annotations strategy)
//     c. Send alerts on success/failure
//     d. Update status tracking
//
// 4. Status Persistence:
//   - Update ReloaderConfig status (reload count, timestamps)
//   - Store new hash in Secret annotation (for next comparison)
//
// Why this design:
// - Hash-based change detection prevents unnecessary reloads
// - Dual discovery supports both declarative (CRD) and imperative (annotations) config
// - Pause periods prevent reload storms during multiple rapid changes
// - Status tracking provides observability and audit trail
func (r *ReloaderConfigReconciler) reconcileSecret(
	ctx context.Context,
	secret *corev1.Secret,
) (ctrl.Result, error) {
	return r.reconcileResourceUpdate(ctx, util.KindSecret, secret)
}

// reconcileConfigMap handles ConfigMap changes and triggers workload reloads
//
// Business Logic Flow:
// Identical to reconcileSecret, but for ConfigMaps.
// Key difference: ConfigMaps have both Data (string) and BinaryData ([]byte) fields,
// so we merge them before hash calculation.
//
// See reconcileSecret for detailed flow explanation.
func (r *ReloaderConfigReconciler) reconcileConfigMap(
	ctx context.Context,
	configMap *corev1.ConfigMap,
) (ctrl.Result, error) {
	return r.reconcileResourceUpdate(ctx, util.KindConfigMap, configMap)
}

// reconcileResourceCreated is a generic function that handles Secret/ConfigMap CREATE events
func (r *ReloaderConfigReconciler) reconcileResourceCreated(
	ctx context.Context,
	resourceKind string,
	obj client.Object,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	resourceName := obj.GetName()
	resourceNamespace := obj.GetNamespace()
	resourceTypeName := resourceKind // "Secret" or "ConfigMap" for logging

	// Check if resource should be ignored
	annotations := obj.GetAnnotations()
	if annotations != nil && annotations[util.AnnotationIgnore] == "true" {
		logger.V(1).Info(resourceTypeName+" marked as ignored, skipping reload on create",
			"name", resourceName,
			"namespace", resourceNamespace)
		return ctrl.Result{}, nil
	}

	logger.Info(resourceTypeName+" created", "name", resourceName, "namespace", resourceNamespace)

	// Calculate hash for the new resource
	currentHash, err := util.GetResourceDataAndHash(obj)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Discover all workloads that need to be reloaded
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, resourceKind, resourceName, resourceNamespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload on create",
		"resource", resourceTypeName+"/"+resourceName,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Always update ReloaderConfig statuses to track the new resource
	r.updateReloaderConfigStatuses(ctx, reloaderConfigs, resourceNamespace, resourceKind, resourceName, currentHash)

	// Only trigger workload reloads if ReloadOnCreate flag is enabled
	successCount := 0
	if r.ReloadOnCreate {
		// Filter targets based on targeted reload settings
		filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, resourceKind, resourceName, resourceNamespace)

		// Execute reloads for filtered targets
		successCount = r.executeReloads(ctx, filteredTargets, resourceKind, resourceName, resourceNamespace, currentHash)
	}

	// Persist hash in resource annotation for future update events
	if err := r.updateResourceHash(ctx, obj, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info(resourceTypeName+" create reconciliation complete", "hash", currentHash, "reloadedTargets", successCount, "reloadOnCreate", r.ReloadOnCreate)
	return ctrl.Result{}, nil
}

// reconcileSecretCreated handles Secret CREATE events when --reload-on-create is enabled
//
// Business Logic:
// When a new Secret is created, this function triggers workload reloads for all workloads
// that are configured to watch this Secret (via annotations or ReloaderConfig).
// Unlike updates, there's no previous hash to compare, so we always trigger the reload.
func (r *ReloaderConfigReconciler) reconcileSecretCreated(
	ctx context.Context,
	secret *corev1.Secret,
) (ctrl.Result, error) {
	return r.reconcileResourceCreated(ctx, util.KindSecret, secret)
}

// reconcileResourceDeleted is a generic function that handles Secret/ConfigMap DELETE events
func (r *ReloaderConfigReconciler) reconcileResourceDeleted(
	ctx context.Context,
	resourceKind string,
	resourceKey client.ObjectKey,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	resourceTypeName := resourceKind // "Secret" or "ConfigMap" for logging

	logger.Info(resourceTypeName+" deleted", "name", resourceKey.Name, "namespace", resourceKey.Namespace)

	// Discover all workloads that were watching this resource
	// Note: We can still find these because the workload annotations/ReloaderConfigs still exist
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, resourceKind, resourceKey.Name, resourceKey.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload on delete",
		"resource", resourceTypeName+"/"+resourceKey.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, resourceKind, resourceKey.Name, resourceKey.Namespace)

	// Execute delete-specific reloads for filtered targets
	successCount := r.executeDeleteReloads(ctx, filteredTargets, resourceKind, resourceKey.Name)

	// Update ReloaderConfig statuses (only if at least one reload succeeded)
	// For delete events, we remove the hash entry from the status
	if successCount > 0 {
		r.removeReloaderConfigStatusEntries(ctx, reloaderConfigs, resourceKey.Namespace, resourceKind, resourceKey.Name)
	}

	logger.Info(resourceTypeName+" delete reconciliation complete", "reloadedTargets", successCount)
	return ctrl.Result{}, nil
}

// reconcileSecretDeleted handles Secret DELETE events when --reload-on-delete is enabled
//
// Business Logic:
// When a Secret is deleted, this function triggers workload reloads using the delete strategy.
// The delete strategy either removes the environment variable or sets the annotation to an empty hash.
// Note: The Secret object no longer exists, so we work with the NamespacedName only.
func (r *ReloaderConfigReconciler) reconcileSecretDeleted(
	ctx context.Context,
	secretKey client.ObjectKey,
) (ctrl.Result, error) {
	return r.reconcileResourceDeleted(ctx, util.KindSecret, secretKey)
}

// reconcileConfigMapCreated handles ConfigMap CREATE events
func (r *ReloaderConfigReconciler) reconcileConfigMapCreated(
	ctx context.Context,
	configMap *corev1.ConfigMap,
) (ctrl.Result, error) {
	return r.reconcileResourceCreated(ctx, util.KindConfigMap, configMap)
}

// reconcileConfigMapDeleted handles ConfigMap DELETE events when --reload-on-delete is enabled
func (r *ReloaderConfigReconciler) reconcileConfigMapDeleted(
	ctx context.Context,
	configMapKey client.ObjectKey,
) (ctrl.Result, error) {
	return r.reconcileResourceDeleted(ctx, util.KindConfigMap, configMapKey)
}

// mapSecretToRequests maps a Secret to reconcile requests
// This function enqueues the Secret itself for reconciliation
func (r *ReloaderConfigReconciler) mapSecretToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return []reconcile.Request{}
	}

	// Enqueue the Secret for reconciliation
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: secret.Namespace,
				Name:      secret.Name,
			},
		},
	}
}

// mapConfigMapToRequests maps a ConfigMap to reconcile requests
// This function enqueues the ConfigMap itself for reconciliation
func (r *ReloaderConfigReconciler) mapConfigMapToRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return []reconcile.Request{}
	}

	// Enqueue the ConfigMap for reconciliation
	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: configMap.Namespace,
				Name:      configMap.Name,
			},
		},
	}
}

// getStoredHash retrieves the previously stored hash from resource annotations
//
// Business Logic:
// The last known hash of a Secret/ConfigMap is stored in its annotations
// under the key "reloader.stakater.com/last-hash". This allows us to
// detect actual data changes vs. metadata-only changes.
func (r *ReloaderConfigReconciler) getStoredHash(annotations map[string]string) string {
	if annotations == nil {
		return ""
	}
	return annotations[util.AnnotationLastHash]
}

// updateResourceHash updates the hash annotation on a Secret or ConfigMap
//
// Business Logic:
// After processing a resource change, we store the new hash in the resource's
// annotations. This serves as the baseline for future change detection.
//
// The annotation key is "reloader.stakater.com/last-hash".
//
// This function is generic and works with any resource that has annotations
// (Secrets, ConfigMaps, etc.).
func (r *ReloaderConfigReconciler) updateResourceHash(
	ctx context.Context,
	obj client.Object,
	newHash string,
) error {
	logger := log.FromContext(ctx)

	// Get current annotations
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Store new hash
	annotations[util.AnnotationLastHash] = newHash
	obj.SetAnnotations(annotations)

	// Persist update
	if err := r.Update(ctx, obj); err != nil {
		logger.Error(err, "Failed to update resource hash annotation",
			"kind", obj.GetObjectKind(),
			"name", obj.GetName())
		return err
	}

	return nil
}
