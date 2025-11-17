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
	logger := log.FromContext(ctx)

	// Phase 0: Check if Secret should be ignored
	if secret.Annotations != nil && secret.Annotations[util.AnnotationIgnore] == "true" {
		logger.V(1).Info("Secret marked as ignored, skipping reload",
			"secret", secret.Name,
			"namespace", secret.Namespace)
		return ctrl.Result{}, nil
	}

	// Phase 1: Check if Secret data actually changed (hash-based change detection)
	currentHash := util.CalculateHash(secret.Data)
	storedHash := r.getStoredHash(secret.Annotations)

	if currentHash == storedHash {
		// Hash matches - no actual change, skip reload
		logger.V(1).Info("Secret data unchanged, skipping reload", "hash", currentHash)
		return ctrl.Result{}, nil
	}

	logger.Info("Secret data changed", "oldHash", storedHash, "newHash", currentHash)

	// Phase 2: Discover all workloads that need to be reloaded
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, util.KindSecret, secret.Name, secret.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload",
		"secret", secret.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Phase 2.5: Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindSecret, secret.Name, secret.Namespace)
	logger.Info("Filtered targets for targeted reload",
		"secret", secret.Name,
		"before", len(allTargets),
		"after", len(filteredTargets))

	// Phase 3: Execute reloads for filtered targets
	successCount := r.executeReloads(ctx, filteredTargets, util.KindSecret, secret.Name, secret.Namespace, currentHash)

	// Phase 4: Update ReloaderConfig statuses (only if at least one reload succeeded)
	if successCount > 0 {
		r.updateReloaderConfigStatuses(ctx, reloaderConfigs, secret.Namespace, util.KindSecret, secret.Name, currentHash)
	}

	// Phase 5: Persist new hash in Secret annotation for future comparisons
	if err := r.updateResourceHash(ctx, secret, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Secret reconciliation complete", "hash", currentHash, "reloadedTargets", len(allTargets))
	return ctrl.Result{}, nil
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
	logger := log.FromContext(ctx)

	// Phase 0: Check if ConfigMap should be ignored
	if configMap.Annotations != nil && configMap.Annotations[util.AnnotationIgnore] == "true" {
		logger.V(1).Info("ConfigMap marked as ignored, skipping reload",
			"configMap", configMap.Name,
			"namespace", configMap.Namespace)
		return ctrl.Result{}, nil
	}

	// Phase 1: Check if ConfigMap data actually changed (hash-based change detection)
	// Note: ConfigMaps have both Data and BinaryData, so we merge them
	data := util.MergeDataMaps(configMap.Data, configMap.BinaryData)
	currentHash := util.CalculateHash(data)
	storedHash := r.getStoredHash(configMap.Annotations)

	if currentHash == storedHash {
		// Hash matches - no actual change, skip reload
		logger.V(1).Info("ConfigMap data unchanged, skipping reload", "hash", currentHash)
		return ctrl.Result{}, nil
	}

	logger.Info("ConfigMap data changed", "oldHash", storedHash, "newHash", currentHash)

	// Phase 2: Discover all workloads that need to be reloaded
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, util.KindConfigMap, configMap.Name, configMap.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload",
		"configMap", configMap.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Phase 2.5: Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindConfigMap, configMap.Name, configMap.Namespace)
	logger.Info("Filtered targets for targeted reload",
		"configMap", configMap.Name,
		"before", len(allTargets),
		"after", len(filteredTargets))

	// Phase 3: Execute reloads for filtered targets
	successCount := r.executeReloads(ctx, filteredTargets, util.KindConfigMap, configMap.Name, configMap.Namespace, currentHash)

	// Phase 4: Update ReloaderConfig statuses (only if at least one reload succeeded)
	if successCount > 0 {
		r.updateReloaderConfigStatuses(ctx, reloaderConfigs, configMap.Namespace, util.KindConfigMap, configMap.Name, currentHash)
	}

	// Phase 5: Persist new hash in ConfigMap annotation for future comparisons
	if err := r.updateResourceHash(ctx, configMap, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("ConfigMap reconciliation complete", "hash", currentHash, "reloadedTargets", len(allTargets))
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
	logger := log.FromContext(ctx)

	// Check if Secret should be ignored
	if secret.Annotations != nil && secret.Annotations[util.AnnotationIgnore] == "true" {
		logger.V(1).Info("Secret marked as ignored, skipping reload on create",
			"secret", secret.Name,
			"namespace", secret.Namespace)
		return ctrl.Result{}, nil
	}
	logger.Info("Secret created", "name", secret.Name, "namespace", secret.Namespace)

	// Calculate hash for the new Secret
	currentHash := util.CalculateHash(secret.Data)

	// Discover all workloads that need to be reloaded
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, util.KindSecret, secret.Name, secret.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload on create",
		"secret", secret.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Always update ReloaderConfig statuses to track the new resource
	r.updateReloaderConfigStatuses(ctx, reloaderConfigs, secret.Namespace, util.KindSecret, secret.Name, currentHash)

	// Only trigger workload reloads if ReloadOnCreate flag is enabled
	successCount := 0
	if r.ReloadOnCreate {
		// Filter targets based on targeted reload settings
		filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindSecret, secret.Name, secret.Namespace)

		// Execute reloads for filtered targets
		successCount = r.executeReloads(ctx, filteredTargets, util.KindSecret, secret.Name, secret.Namespace, currentHash)
	}

	// Persist hash in Secret annotation for future update events
	if err := r.updateResourceHash(ctx, secret, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Secret create reconciliation complete", "hash", currentHash, "reloadedTargets", successCount, "reloadOnCreate", r.ReloadOnCreate)
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
	logger := log.FromContext(ctx)
	logger.Info("Secret deleted", "name", secretKey.Name, "namespace", secretKey.Namespace)

	// Discover all workloads that were watching this Secret
	// Note: We can still find these because the workload annotations/ReloaderConfigs still exist
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, util.KindSecret, secretKey.Name, secretKey.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload on delete",
		"secret", secretKey.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindSecret, secretKey.Name, secretKey.Namespace)

	// Execute delete-specific reloads for filtered targets
	successCount := r.executeDeleteReloads(ctx, filteredTargets, util.KindSecret, secretKey.Name)

	// Update ReloaderConfig statuses (only if at least one reload succeeded)
	// For delete events, we remove the hash entry from the status
	if successCount > 0 {
		r.removeReloaderConfigStatusEntries(ctx, reloaderConfigs, secretKey.Namespace, util.KindSecret, secretKey.Name)
	}

	logger.Info("Secret delete reconciliation complete", "reloadedTargets", successCount)
	return ctrl.Result{}, nil
}

// reconcileConfigMapCreated handles ConfigMap CREATE events
func (r *ReloaderConfigReconciler) reconcileConfigMapCreated(
	ctx context.Context,
	configMap *corev1.ConfigMap,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Check if ConfigMap should be ignored
	if configMap.Annotations != nil && configMap.Annotations[util.AnnotationIgnore] == "true" {
		logger.V(1).Info("ConfigMap marked as ignored, skipping reload on create",
			"configMap", configMap.Name,
			"namespace", configMap.Namespace)
		return ctrl.Result{}, nil
	}

	logger.Info("ConfigMap created", "name", configMap.Name, "namespace", configMap.Namespace)

	// Calculate hash for the new ConfigMap
	data := util.MergeDataMaps(configMap.Data, configMap.BinaryData)
	currentHash := util.CalculateHash(data)

	// Discover all workloads that need to be reloaded
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, util.KindConfigMap, configMap.Name, configMap.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload on create",
		"configMap", configMap.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Always update ReloaderConfig statuses to track the new resource
	r.updateReloaderConfigStatuses(ctx, reloaderConfigs, configMap.Namespace, util.KindConfigMap, configMap.Name, currentHash)

	// Only trigger workload reloads if ReloadOnCreate flag is enabled
	successCount := 0
	if r.ReloadOnCreate {
		// Filter targets based on targeted reload settings
		filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindConfigMap, configMap.Name, configMap.Namespace)

		// Execute reloads for filtered targets
		successCount = r.executeReloads(ctx, filteredTargets, util.KindConfigMap, configMap.Name, configMap.Namespace, currentHash)
	}

	// Persist hash in ConfigMap annotation for future update events
	if err := r.updateResourceHash(ctx, configMap, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("ConfigMap create reconciliation complete", "hash", currentHash, "reloadedTargets", successCount, "reloadOnCreate", r.ReloadOnCreate)
	return ctrl.Result{}, nil
}

// reconcileConfigMapDeleted handles ConfigMap DELETE events when --reload-on-delete is enabled
func (r *ReloaderConfigReconciler) reconcileConfigMapDeleted(
	ctx context.Context,
	configMapKey client.ObjectKey,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("ConfigMap deleted", "name", configMapKey.Name, "namespace", configMapKey.Namespace)

	// Discover all workloads that were watching this ConfigMap
	allTargets, reloaderConfigs, err := r.discoverTargets(ctx, util.KindConfigMap, configMapKey.Name, configMapKey.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Found targets for reload on delete",
		"configMap", configMapKey.Name,
		"totalTargets", len(allTargets),
		"fromCRD", len(reloaderConfigs))

	// Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindConfigMap, configMapKey.Name, configMapKey.Namespace)

	// Execute delete-specific reloads for filtered targets
	successCount := r.executeDeleteReloads(ctx, filteredTargets, util.KindConfigMap, configMapKey.Name)

	// Update ReloaderConfig statuses (only if at least one reload succeeded)
	// For delete events, we remove the hash entry from the status
	if successCount > 0 {
		r.removeReloaderConfigStatusEntries(ctx, reloaderConfigs, configMapKey.Namespace, util.KindConfigMap, configMapKey.Name)
	}

	logger.Info("ConfigMap delete reconciliation complete", "reloadedTargets", successCount)
	return ctrl.Result{}, nil
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
