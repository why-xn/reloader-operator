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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/alerts"
	"github.com/stakater/Reloader/internal/pkg/util"
	"github.com/stakater/Reloader/internal/pkg/workload"
)

// ReloaderConfigReconciler reconciles a ReloaderConfig object
type ReloaderConfigReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	WorkloadFinder  *workload.Finder
	WorkloadUpdater *workload.Updater
	AlertManager    *alerts.Manager
}

// RBAC permissions for ReloaderConfig CRD
// +kubebuilder:rbac:groups=reloader.stakater.com,resources=reloaderconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=reloader.stakater.com,resources=reloaderconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=reloader.stakater.com,resources=reloaderconfigs/finalizers,verbs=update

// RBAC permissions for Secrets and ConfigMaps
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;update;patch

// RBAC permissions for Workloads
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;update;patch

// RBAC permissions for Argo Rollouts (optional)
// +kubebuilder:rbac:groups=argoproj.io,resources=rollouts,verbs=get;list;watch;update;patch

// RBAC permissions for OpenShift DeploymentConfigs (optional)
// +kubebuilder:rbac:groups=apps.openshift.io,resources=deploymentconfigs,verbs=get;list;watch;update;patch

// RBAC permissions for Events (for recording events)
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is the main entry point for the kubernetes reconciliation loop.
//
// Business Logic Flow:
// The operator watches three types of resources and handles them differently:
//
// 1. ReloaderConfig CRD:
//   - User creates/updates a ReloaderConfig resource defining which Secrets/ConfigMaps to watch
//   - Validates that watched resources and target workloads exist
//   - Initializes hash tracking for watched resources
//   - Updates status conditions to reflect availability
//
// 2. Secret Changes:
//   - Detects changes by comparing SHA256 hash of Secret.Data
//   - Finds all workloads that depend on this Secret (via CRD or annotations)
//   - Triggers rolling restart of affected workloads
//   - Sends alerts and updates status tracking
//
// 3. ConfigMap Changes:
//   - Same flow as Secrets but for ConfigMap.Data and ConfigMap.BinaryData
//
// The reconciliation is idempotent - running it multiple times with no changes
// will not trigger unnecessary workload restarts (hash comparison prevents this).
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *ReloaderConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Phase 1: Determine resource type
	// We try to fetch the resource as different types to figure out what changed.
	// This is necessary because controller-runtime enqueues only the object key,
	// not the object type.

	// Try to fetch as ReloaderConfig first
	reloaderConfig := &reloaderv1alpha1.ReloaderConfig{}
	err := r.Get(ctx, req.NamespacedName, reloaderConfig)

	if err == nil {
		// This is a ReloaderConfig change - user created or updated the CRD
		logger.Info("Reconciling ReloaderConfig", "name", reloaderConfig.Name, "namespace", reloaderConfig.Namespace)
		return r.reconcileReloaderConfig(ctx, reloaderConfig)
	}

	if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// Try to fetch as Secret
	secret := &corev1.Secret{}
	err = r.Get(ctx, req.NamespacedName, secret)

	if err == nil {
		// This is a Secret change - data was updated
		logger.Info("Reconciling Secret", "name", secret.Name, "namespace", secret.Namespace)
		return r.reconcileSecret(ctx, secret)
	}

	if !errors.IsNotFound(err) {
		logger.Error(err, "Failed to get Secret")
		return ctrl.Result{}, err
	}

	// Try to fetch as ConfigMap
	configMap := &corev1.ConfigMap{}
	err = r.Get(ctx, req.NamespacedName, configMap)

	if err == nil {
		// This is a ConfigMap change - data was updated
		logger.Info("Reconciling ConfigMap", "name", configMap.Name, "namespace", configMap.Namespace)
		return r.reconcileConfigMap(ctx, configMap)
	}

	// Resource was deleted or doesn't exist - nothing to do
	return ctrl.Result{}, client.IgnoreNotFound(err)
}

// reconcileReloaderConfig handles ReloaderConfig CRD changes
//
// Business Logic:
// This function is called when a user creates or updates a ReloaderConfig resource.
// It performs validation and initialization:
//
// 1. Initialize Status: Creates the hash tracking map if it doesn't exist
// 2. Validate Watched Resources: Checks that all Secrets and ConfigMaps exist
// 3. Initialize Hash Tracking: Calculates initial hash for each watched resource
// 4. Validate Target Workloads: Ensures all target Deployments/StatefulSets/DaemonSets exist
// 5. Update Status Conditions: Sets Available/Degraded/Progressing conditions
//
// Why we do this:
// - Early validation prevents runtime errors later when Secrets/ConfigMaps change
// - Hash initialization establishes a baseline for change detection
// - Status conditions provide observability for users (kubectl get reloaderconfig)
func (r *ReloaderConfigReconciler) reconcileReloaderConfig(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Phase 1: Initialize status tracking
	// The hash map stores SHA256 hashes of watched resources for change detection
	if config.Status.WatchedResourceHashes == nil {
		config.Status.WatchedResourceHashes = make(map[string]string)
	}

	// Set progressing condition to indicate reconciliation is in progress
	util.SetCondition(&config.Status.Conditions, util.ConditionProgressing, metav1.ConditionTrue, util.ReasonReconciling, "Reconciling ReloaderConfig")

	// Phase 2: Validate and initialize watched resources
	if config.Spec.WatchedResources != nil {
		// Process all watched Secrets
		r.initializeWatchedSecrets(ctx, config)

		// Process all watched ConfigMaps
		r.initializeWatchedConfigMaps(ctx, config)
	}

	// Phase 3: Validate target workloads exist
	// This prevents configuration errors where users specify non-existent targets
	validTargets := r.validateTargetWorkloads(ctx, config)

	// Phase 4: Update status conditions
	// ObservedGeneration tracks which version of the spec we've reconciled
	config.Status.ObservedGeneration = config.Generation

	if validTargets {
		// All targets exist - mark as Available
		util.SetCondition(&config.Status.Conditions, util.ConditionAvailable, metav1.ConditionTrue,
			util.ReasonReconciled, "ReloaderConfig is active and watching resources")
		util.SetCondition(&config.Status.Conditions, util.ConditionDegraded, metav1.ConditionFalse,
			util.ReasonReconciled, "")
	}

	// Clear progressing condition - reconciliation complete
	util.SetCondition(&config.Status.Conditions, util.ConditionProgressing, metav1.ConditionFalse,
		util.ReasonReconciled, "")

	// Phase 5: Persist status updates
	// This updates the status subresource, which is separate from the main resource
	if err := r.Status().Update(ctx, config); err != nil {
		logger.Error(err, "Failed to update ReloaderConfig status")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully reconciled ReloaderConfig", "name", config.Name)
	return ctrl.Result{}, nil
}

// initializeWatchedSecrets validates and initializes hash tracking for watched Secrets
//
// Business Logic:
// For each Secret listed in the ReloaderConfig:
// 1. Fetch the Secret from the cluster
// 2. Calculate SHA256 hash of its data
// 3. Store the hash in status for future change detection
//
// If a Secret doesn't exist, we log an error and set a Degraded condition,
// but continue processing other Secrets (fail gracefully, not catastrophically).
func (r *ReloaderConfigReconciler) initializeWatchedSecrets(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
) {
	logger := log.FromContext(ctx)

	for _, secretName := range config.Spec.WatchedResources.Secrets {
		secret := &corev1.Secret{}
		key := client.ObjectKey{
			Namespace: config.Namespace,
			Name:      secretName,
		}

		if err := r.Get(ctx, key, secret); err != nil {
			logger.Error(err, "Failed to get watched Secret", "name", secretName)
			util.SetCondition(&config.Status.Conditions, util.ConditionDegraded, metav1.ConditionTrue,
				util.ReasonResourceNotFound, fmt.Sprintf("Secret %s not found", secretName))
			continue
		}

		// Calculate SHA256 hash of Secret data
		// This hash will be compared on future Secret updates to detect actual changes
		hash := util.CalculateHash(secret.Data)
		resourceKey := util.MakeResourceKey(secret.Namespace, util.KindSecret, secret.Name)
		config.Status.WatchedResourceHashes[resourceKey] = hash
		logger.V(1).Info("Initialized Secret hash", "secret", secretName, "hash", hash)
	}
}

// initializeWatchedConfigMaps validates and initializes hash tracking for watched ConfigMaps
//
// Business Logic:
// Same as initializeWatchedSecrets, but for ConfigMaps.
// Note: We hash both Data (string map) and BinaryData (byte map) together.
func (r *ReloaderConfigReconciler) initializeWatchedConfigMaps(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
) {
	logger := log.FromContext(ctx)

	for _, cmName := range config.Spec.WatchedResources.ConfigMaps {
		configMap := &corev1.ConfigMap{}
		key := client.ObjectKey{
			Namespace: config.Namespace,
			Name:      cmName,
		}

		if err := r.Get(ctx, key, configMap); err != nil {
			logger.Error(err, "Failed to get watched ConfigMap", "name", cmName)
			util.SetCondition(&config.Status.Conditions, util.ConditionDegraded, metav1.ConditionTrue,
				util.ReasonResourceNotFound, fmt.Sprintf("ConfigMap %s not found", cmName))
			continue
		}

		// ConfigMaps have both Data (string) and BinaryData ([]byte) fields
		// We merge them together for hash calculation
		data := util.MergeDataMaps(configMap.Data, configMap.BinaryData)
		hash := util.CalculateHash(data)
		resourceKey := util.MakeResourceKey(configMap.Namespace, util.KindConfigMap, configMap.Name)
		config.Status.WatchedResourceHashes[resourceKey] = hash
		logger.V(1).Info("Initialized ConfigMap hash", "configMap", cmName, "hash", hash)
	}
}

// validateTargetWorkloads checks that all target workloads exist in the cluster
//
// Business Logic:
// For each target defined in the ReloaderConfig:
// 1. Resolve the namespace (use target's namespace or default to ReloaderConfig's namespace)
// 2. Check if the workload exists in the cluster
// 3. Set Degraded condition if any target is missing
//
// Returns true if all targets exist, false otherwise.
//
// Why we do this:
// - Provides immediate feedback to users if they misconfigured the ReloaderConfig
// - Prevents silent failures where the operator watches resources but can't reload anything
func (r *ReloaderConfigReconciler) validateTargetWorkloads(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
) bool {
	logger := log.FromContext(ctx)
	validTargets := true

	for _, target := range config.Spec.Targets {
		// If target doesn't specify a namespace, use the ReloaderConfig's namespace
		targetNs := util.GetDefaultNamespace(target.Namespace, config.Namespace)

		// Check if workload exists
		exists, err := r.workloadExists(ctx, target.Kind, target.Name, targetNs)
		if err != nil || !exists {
			logger.Error(err, "Target workload not found or error checking", "kind", target.Kind, "name", target.Name, "namespace", targetNs)
			util.SetCondition(&config.Status.Conditions, util.ConditionDegraded, metav1.ConditionTrue,
				util.ReasonTargetNotFound, fmt.Sprintf("Target %s/%s not found", target.Kind, target.Name))
			validTargets = false
		}
	}

	return validTargets
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
	logger := log.FromContext(ctx)

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

	// Phase 3: Execute reloads for all discovered targets
	r.executeReloads(ctx, allTargets, util.KindSecret, secret.Name, currentHash)

	// Phase 4: Update ReloaderConfig statuses
	r.updateReloaderConfigStatuses(ctx, reloaderConfigs, secret.Namespace, util.KindSecret, secret.Name, currentHash)

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

	// Phase 3: Execute reloads for all discovered targets
	r.executeReloads(ctx, allTargets, util.KindConfigMap, configMap.Name, currentHash)

	// Phase 4: Update ReloaderConfig statuses
	r.updateReloaderConfigStatuses(ctx, reloaderConfigs, configMap.Namespace, util.KindConfigMap, configMap.Name, currentHash)

	// Phase 5: Persist new hash in ConfigMap annotation for future comparisons
	if err := r.updateResourceHash(ctx, configMap, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("ConfigMap reconciliation complete", "hash", currentHash, "reloadedTargets", len(allTargets))
	return ctrl.Result{}, nil
}

// ============================================================================
// Helper Functions - Extracted for Reusability and Clarity
// ============================================================================

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

// discoverTargets finds all workloads that need to be reloaded for a given resource
//
// Business Logic:
// The operator supports two ways to configure reloading:
//
//  1. CRD-based (declarative):
//     User creates a ReloaderConfig resource that specifies:
//     - Which Secrets/ConfigMaps to watch
//     - Which workloads to reload when those resources change
//
//  2. Annotation-based (imperative):
//     User adds annotations directly to Deployments/StatefulSets/DaemonSets:
//     - secret.reloader.stakater.com/reload: "secret-name"
//     - configmap.reloader.stakater.com/reload: "configmap-name"
//
// This function finds targets from BOTH sources and merges them, allowing
// users to mix and match configuration styles.
//
// Returns:
// - allTargets: Combined list of all workloads to reload
// - reloaderConfigs: List of ReloaderConfigs that reference this resource (for status updates)
// - error: Any errors encountered during discovery
func (r *ReloaderConfigReconciler) discoverTargets(
	ctx context.Context,
	resourceKind string,
	resourceName string,
	resourceNamespace string,
) ([]workload.Target, []*reloaderv1alpha1.ReloaderConfig, error) {
	logger := log.FromContext(ctx)

	// Find all ReloaderConfigs watching this resource
	reloaderConfigs, err := r.WorkloadFinder.FindReloaderConfigsWatchingResource(
		ctx, resourceKind, resourceName, resourceNamespace)
	if err != nil {
		logger.Error(err, "Failed to find ReloaderConfigs")
		return nil, nil, err
	}

	// Find workloads with annotation-based config
	annotatedWorkloads, err := r.WorkloadFinder.FindWorkloadsWithAnnotations(
		ctx, resourceKind, resourceName, resourceNamespace)
	if err != nil {
		logger.Error(err, "Failed to find annotated workloads")
		return nil, nil, err
	}

	// Merge targets from both sources
	allTargets := r.mergeTargets(reloaderConfigs, annotatedWorkloads)

	return allTargets, reloaderConfigs, nil
}

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
func (r *ReloaderConfigReconciler) executeReloads(
	ctx context.Context,
	targets []workload.Target,
	resourceKind string,
	resourceName string,
	resourceHash string,
) {
	logger := log.FromContext(ctx)

	for _, target := range targets {
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
		err = r.WorkloadUpdater.TriggerReload(ctx, target, resourceHash)
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
	}
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

	// Send error alerts if alerting is configured for this target
	if target.Config != nil && target.Config.Spec.Alerts != nil {
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

		if alertErr := r.AlertManager.SendReloadAlert(ctx, target.Config, message); alertErr != nil {
			logger.Error(alertErr, "Failed to send error alerts", "workload", target.Name)
		}
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

	// Send success alerts if alerting is configured for this target
	if target.Config != nil && target.Config.Spec.Alerts != nil {
		message := alerts.NewReloadSuccessMessage(
			target.Kind,
			target.Name,
			target.Namespace,
			resourceKind,
			resourceName,
			target.ReloadStrategy,
		)
		message.Timestamp = time.Now()

		if err := r.AlertManager.SendReloadAlert(ctx, target.Config, message); err != nil {
			logger.Error(err, "Failed to send success alerts", "workload", target.Name)
		}
	}

	// Update target status (clears any previous error)
	if target.Config != nil {
		r.updateTargetStatus(ctx, target.Config, target, "")
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
	logger := log.FromContext(ctx)

	for _, config := range configs {
		// Update hash for this specific resource
		resourceKey := util.MakeResourceKey(resourceNamespace, resourceKind, resourceName)
		config.Status.WatchedResourceHashes[resourceKey] = newHash

		// Increment reload counter and update timestamp
		config.Status.ReloadCount++
		now := metav1.Now()
		config.Status.LastReloadTime = &now

		// Persist status update
		if err := r.Status().Update(ctx, config); err != nil {
			logger.Error(err, "Failed to update ReloaderConfig status", "config", config.Name)
		}
	}
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

// ============================================================================
// End of Helper Functions
// ============================================================================

// workloadExists checks if a workload of the given kind exists
func (r *ReloaderConfigReconciler) workloadExists(ctx context.Context, kind, name, namespace string) (bool, error) {
	key := client.ObjectKey{Name: name, Namespace: namespace}

	switch kind {
	case util.KindDeployment:
		deployment := &appsv1.Deployment{}
		err := r.Get(ctx, key, deployment)
		return err == nil, client.IgnoreNotFound(err)

	case util.KindStatefulSet:
		statefulSet := &appsv1.StatefulSet{}
		err := r.Get(ctx, key, statefulSet)
		return err == nil, client.IgnoreNotFound(err)

	case util.KindDaemonSet:
		daemonSet := &appsv1.DaemonSet{}
		err := r.Get(ctx, key, daemonSet)
		return err == nil, client.IgnoreNotFound(err)

	default:
		return false, fmt.Errorf("unsupported workload kind: %s", kind)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReloaderConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch ReloaderConfig CRD
		For(&reloaderv1alpha1.ReloaderConfig{}).

		// Watch Secrets - enqueue requests when Secrets change
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToRequests),
			// Only trigger on actual data changes
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return true // Always process creates
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// Only trigger if annotations, data, or labels changed
					return true // We'll check hash in reconcile
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return true // Process deletes
				},
			}),
		).

		// Watch ConfigMaps - enqueue requests when ConfigMaps change
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapConfigMapToRequests),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return true
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return true // We'll check hash in reconcile
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return true
				},
			}),
		).
		Named("reloaderconfig").
		Complete(r)
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

// mergeTargets merges targets from ReloaderConfigs and annotation-based workloads
func (r *ReloaderConfigReconciler) mergeTargets(
	configs []*reloaderv1alpha1.ReloaderConfig,
	annotatedWorkloads []workload.Target,
) []workload.Target {
	allTargets := []workload.Target{}

	// Add targets from ReloaderConfigs
	for _, config := range configs {
		for _, target := range config.Spec.Targets {
			allTargets = append(allTargets, workload.Target{
				Kind:           target.Kind,
				Name:           target.Name,
				Namespace:      util.GetDefaultNamespace(target.Namespace, config.Namespace),
				ReloadStrategy: util.GetDefaultStrategy(target.ReloadStrategy, config.Spec.ReloadStrategy),
				PausePeriod:    target.PausePeriod,
				Config:         config,
			})
		}
	}

	// Add annotation-based workloads
	allTargets = append(allTargets, annotatedWorkloads...)

	return allTargets
}

// updateTargetStatus updates the status for a specific target workload
func (r *ReloaderConfigReconciler) updateTargetStatus(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
	target workload.Target,
	errorMsg string,
) {
	logger := log.FromContext(ctx)

	// Find existing status entry
	found := false
	for i := range config.Status.TargetStatus {
		status := &config.Status.TargetStatus[i]
		if status.Kind == target.Kind &&
			status.Name == target.Name &&
			status.Namespace == target.Namespace {

			// Update existing entry
			now := metav1.Now()
			status.LastReloadTime = &now
			status.ReloadCount++
			status.LastError = errorMsg

			// Set pause period if configured
			if target.PausePeriod != "" && errorMsg == "" {
				duration, err := util.ParseDuration(target.PausePeriod)
				if err == nil {
					pausedUntil := metav1.NewTime(now.Add(duration))
					status.PausedUntil = &pausedUntil
				}
			}

			found = true
			break
		}
	}

	// Add new entry if not found
	if !found {
		now := metav1.Now()
		newStatus := reloaderv1alpha1.TargetWorkloadStatus{
			Kind:           target.Kind,
			Name:           target.Name,
			Namespace:      target.Namespace,
			LastReloadTime: &now,
			ReloadCount:    1,
			LastError:      errorMsg,
		}

		// Set pause period if configured
		if target.PausePeriod != "" && errorMsg == "" {
			duration, err := util.ParseDuration(target.PausePeriod)
			if err == nil {
				pausedUntil := metav1.NewTime(now.Add(duration))
				newStatus.PausedUntil = &pausedUntil
			}
		}

		config.Status.TargetStatus = append(config.Status.TargetStatus, newStatus)
	}

	// Update the ReloaderConfig status
	if err := r.Status().Update(ctx, config); err != nil {
		logger.Error(err, "Failed to update target status",
			"config", config.Name,
			"target", target.Name)
	}
}
