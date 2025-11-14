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
	"sync/atomic"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/alerts"
	"github.com/stakater/Reloader/internal/pkg/util"
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

// ReloaderConfigReconciler reconciles a ReloaderConfig object
type ReloaderConfigReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	WorkloadFinder  *workload.Finder
	WorkloadUpdater *workload.Updater
	AlertManager    *alerts.Manager
	statusQueue     workqueue.TypedRateLimitingInterface[statusUpdateWorkItem]
	ctx             context.Context
	cancelFunc      context.CancelFunc

	// Global flags for reload behavior
	ReloadOnCreate bool
	ReloadOnDelete bool

	// Resource filtering
	ResourceLabelSelector labels.Selector

	// Initialization tracking (safeguard to prevent processing events during startup)
	controllersInitialized atomic.Bool
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

// RBAC permissions for Pods (required for restart strategy)
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete

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

	if !apierrors.IsNotFound(err) {
		logger.Error(err, "Failed to get resource")
		return ctrl.Result{}, err
	}

	// Try to fetch as Secret
	secret := &corev1.Secret{}
	secretErr := r.Get(ctx, req.NamespacedName, secret)

	if secretErr == nil {
		// Secret exists - determine if it's a create or update event
		lastHash := secret.Annotations[util.AnnotationLastHash]

		if lastHash == "" && r.ReloadOnCreate {
			// No last-hash annotation means this is a newly created Secret
			logger.Info("Reconciling Secret (CREATE)", "name", secret.Name, "namespace", secret.Namespace)
			return r.reconcileSecretCreated(ctx, secret)
		}

		// Has last-hash annotation or ReloadOnCreate is disabled - treat as update
		logger.Info("Reconciling Secret (UPDATE)", "name", secret.Name, "namespace", secret.Namespace)
		return r.reconcileSecret(ctx, secret)
	}

	if !apierrors.IsNotFound(secretErr) {
		logger.Error(secretErr, "Failed to get Secret")
		return ctrl.Result{}, secretErr
	}

	// Try to fetch as ConfigMap
	configMap := &corev1.ConfigMap{}
	configMapErr := r.Get(ctx, req.NamespacedName, configMap)

	if configMapErr == nil {
		// ConfigMap exists - determine if it's a create or update event
		lastHash := configMap.Annotations[util.AnnotationLastHash]

		if lastHash == "" && r.ReloadOnCreate {
			// No last-hash annotation means this is a newly created ConfigMap
			logger.Info("Reconciling ConfigMap (CREATE)", "name", configMap.Name, "namespace", configMap.Namespace)
			return r.reconcileConfigMapCreated(ctx, configMap)
		}

		// Has last-hash annotation or ReloadOnCreate is disabled - treat as update
		logger.Info("Reconciling ConfigMap (UPDATE)", "name", configMap.Name, "namespace", configMap.Namespace)
		return r.reconcileConfigMap(ctx, configMap)
	}

	if !apierrors.IsNotFound(configMapErr) {
		logger.Error(configMapErr, "Failed to get ConfigMap")
		return ctrl.Result{}, configMapErr
	}

	// Both Secret and ConfigMap not found - check if this is a delete event
	if r.ReloadOnDelete {
		// We need to determine if this was a Secret or ConfigMap that was deleted
		// Try to check which watcher triggered this event by examining recent activity
		// For now, we'll try both delete handlers and let them handle the case gracefully

		// Check if there are any ReloaderConfigs watching a Secret with this name
		secretTargets, secretConfigs, _ := r.discoverTargets(ctx, util.KindSecret, req.Name, req.Namespace)
		if len(secretTargets) > 0 || len(secretConfigs) > 0 {
			logger.Info("Reconciling Secret (DELETE)", "name", req.Name, "namespace", req.Namespace)
			return r.reconcileSecretDeleted(ctx, req.NamespacedName)
		}

		// Check if there are any ReloaderConfigs watching a ConfigMap with this name
		configMapTargets, configMapConfigs, _ := r.discoverTargets(ctx, util.KindConfigMap, req.Name, req.Namespace)
		if len(configMapTargets) > 0 || len(configMapConfigs) > 0 {
			logger.Info("Reconciling ConfigMap (DELETE)", "name", req.Name, "namespace", req.Namespace)
			return r.reconcileConfigMapDeleted(ctx, req.NamespacedName)
		}
	}

	// Resource was deleted or doesn't exist - nothing to do
	return ctrl.Result{}, nil
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

	// Phase 2.5: Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindSecret, secret.Name, secret.Namespace)
	logger.Info("Filtered targets for targeted reload",
		"secret", secret.Name,
		"before", len(allTargets),
		"after", len(filteredTargets))

	// Phase 3: Execute reloads for filtered targets
	successCount := r.executeReloads(ctx, filteredTargets, util.KindSecret, secret.Name, currentHash)

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
	successCount := r.executeReloads(ctx, filteredTargets, util.KindConfigMap, configMap.Name, currentHash)

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

	// Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindSecret, secret.Name, secret.Namespace)

	// Execute reloads for filtered targets
	successCount := r.executeReloads(ctx, filteredTargets, util.KindSecret, secret.Name, currentHash)

	// Update ReloaderConfig statuses (only if at least one reload succeeded)
	if successCount > 0 {
		r.updateReloaderConfigStatuses(ctx, reloaderConfigs, secret.Namespace, util.KindSecret, secret.Name, currentHash)
	}

	// Persist hash in Secret annotation for future update events
	if err := r.updateResourceHash(ctx, secret, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Secret create reconciliation complete", "hash", currentHash, "reloadedTargets", successCount)
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

// reconcileConfigMapCreated handles ConfigMap CREATE events when --reload-on-create is enabled
func (r *ReloaderConfigReconciler) reconcileConfigMapCreated(
	ctx context.Context,
	configMap *corev1.ConfigMap,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
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

	// Filter targets based on targeted reload settings
	filteredTargets := r.filterTargetsForTargetedReload(ctx, allTargets, util.KindConfigMap, configMap.Name, configMap.Namespace)

	// Execute reloads for filtered targets
	successCount := r.executeReloads(ctx, filteredTargets, util.KindConfigMap, configMap.Name, currentHash)

	// Update ReloaderConfig statuses (only if at least one reload succeeded)
	if successCount > 0 {
		r.updateReloaderConfigStatuses(ctx, reloaderConfigs, configMap.Namespace, util.KindConfigMap, configMap.Name, currentHash)
	}

	// Persist hash in ConfigMap annotation for future update events
	if err := r.updateResourceHash(ctx, configMap, currentHash); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("ConfigMap create reconciliation complete", "hash", currentHash, "reloadedTargets", successCount)
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

	// Filter out ReloaderConfigs that have this resource in their ignoreResources list
	filteredConfigs := []*reloaderv1alpha1.ReloaderConfig{}
	for _, config := range reloaderConfigs {
		if r.shouldIgnoreResource(config, resourceKind, resourceName, resourceNamespace) {
			logger.Info("Ignoring resource due to ignoreResources configuration",
				"config", config.Name,
				"resource", resourceKind+"/"+resourceName,
				"namespace", resourceNamespace)
			continue
		}
		filteredConfigs = append(filteredConfigs, config)
	}
	reloaderConfigs = filteredConfigs

	// Get the resource to access its annotations for targeted reload (search + match)
	var resourceAnnotations map[string]string
	if resourceKind == util.KindSecret {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: resourceNamespace}, secret); err == nil {
			resourceAnnotations = secret.Annotations
		}
	} else if resourceKind == util.KindConfigMap {
		cm := &corev1.ConfigMap{}
		if err := r.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: resourceNamespace}, cm); err == nil {
			resourceAnnotations = cm.Annotations
		}
	}

	// Find workloads with annotation-based config
	annotatedWorkloads, err := r.WorkloadFinder.FindWorkloadsWithAnnotations(
		ctx, resourceKind, resourceName, resourceNamespace, resourceAnnotations)
	if err != nil {
		logger.Error(err, "Failed to find annotated workloads")
		return nil, nil, err
	}

	// Merge targets from both sources
	allTargets := r.mergeTargets(reloaderConfigs, annotatedWorkloads)

	return allTargets, reloaderConfigs, nil
}

// shouldIgnoreResource checks if a resource should be ignored based on ignoreResources configuration
func (r *ReloaderConfigReconciler) shouldIgnoreResource(
	config *reloaderv1alpha1.ReloaderConfig,
	resourceKind string,
	resourceName string,
	resourceNamespace string,
) bool {
	// If no ignoreResources specified, don't ignore anything
	if len(config.Spec.IgnoreResources) == 0 {
		return false
	}

	// Check if this resource is in the ignore list
	for _, ignoredResource := range config.Spec.IgnoreResources {
		// Match by kind and name
		if ignoredResource.Kind != resourceKind || ignoredResource.Name != resourceName {
			continue
		}

		// If namespace is specified in the ignore rule, it must match
		// If namespace is not specified in the ignore rule, match any namespace
		if ignoredResource.Namespace != "" && ignoredResource.Namespace != resourceNamespace {
			continue
		}

		// Found a match - this resource should be ignored
		return true
	}

	return false
}

// filterTargetsForTargetedReload filters targets based on targeted reload settings
//
// Business Logic:
// CRD-based targeted reload works similarly to annotation-based search+match:
//
// For each target:
// 1. Check if it's CRD-based (has a Config reference)
// 2. Check if config has EnableTargetedReload=true (resources in "match" mode)
// 3. Check if target has RequireReference=true
// 4. If both are true, verify the workload actually references the changed resource
// 5. If not referenced, filter out this target
//
// This prevents unnecessary reloads when a ReloaderConfig watches multiple resources
// but individual targets only use some of them.
func (r *ReloaderConfigReconciler) filterTargetsForTargetedReload(
	ctx context.Context,
	targets []workload.Target,
	resourceKind string,
	resourceName string,
	resourceNamespace string,
) []workload.Target {
	logger := log.FromContext(ctx)
	filteredTargets := []workload.Target{}

	for _, target := range targets {
		// Include annotation-based targets without filtering (they handle their own logic)
		if target.Config == nil {
			filteredTargets = append(filteredTargets, target)
			continue
		}

		// For CRD-based targets, check if targeted reload is enabled
		enableTargetedReload := target.Config.Spec.WatchedResources != nil &&
			target.Config.Spec.WatchedResources.EnableTargetedReload

		// If targeted reload is NOT enabled on the config, include all targets
		if !enableTargetedReload {
			filteredTargets = append(filteredTargets, target)
			continue
		}

		// If targeted reload IS enabled on config but RequireReference=false on target,
		// include the target without filtering (broadcast reload for all watched resources)
		if !target.RequireReference {
			logger.V(1).Info("Including target - RequireReference=false, no filtering applied",
				"target", target.Name,
				"kind", target.Kind)
			filteredTargets = append(filteredTargets, target)
			continue
		}

		// Both EnableTargetedReload and RequireReference are true
		// Check if the workload actually references the changed resource
		references, err := r.workloadReferencesResource(ctx, target, resourceKind, resourceName)
		if err != nil {
			logger.Error(err, "Failed to check if workload references resource",
				"target", target.Name,
				"resource", resourceKind+"/"+resourceName)
			// On error, include the target to be safe (avoid missing a reload)
			filteredTargets = append(filteredTargets, target)
			continue
		}

		if references {
			logger.V(1).Info("Including target - references the changed resource",
				"target", target.Name,
				"resource", resourceKind+"/"+resourceName)
			filteredTargets = append(filteredTargets, target)
		} else {
			logger.Info("Skipping target - does not reference the changed resource",
				"target", target.Name,
				"kind", target.Kind,
				"resource", resourceKind+"/"+resourceName)
		}
	}

	return filteredTargets
}

// workloadReferencesResource checks if a workload references a specific resource
func (r *ReloaderConfigReconciler) workloadReferencesResource(
	ctx context.Context,
	target workload.Target,
	resourceKind string,
	resourceName string,
) (bool, error) {
	// Fetch the workload to get its pod spec
	var podSpec *corev1.PodSpec

	key := client.ObjectKey{
		Name:      target.Name,
		Namespace: target.Namespace,
	}

	switch target.Kind {
	case util.KindDeployment:
		deployment := &appsv1.Deployment{}
		if err := r.Get(ctx, key, deployment); err != nil {
			return false, err
		}
		podSpec = &deployment.Spec.Template.Spec

	case util.KindStatefulSet:
		sts := &appsv1.StatefulSet{}
		if err := r.Get(ctx, key, sts); err != nil {
			return false, err
		}
		podSpec = &sts.Spec.Template.Spec

	case util.KindDaemonSet:
		ds := &appsv1.DaemonSet{}
		if err := r.Get(ctx, key, ds); err != nil {
			return false, err
		}
		podSpec = &ds.Spec.Template.Spec

	default:
		return false, fmt.Errorf("unsupported workload kind: %s", target.Kind)
	}

	// Check if pod spec references the resource
	return workloadPodSpecReferencesResource(podSpec, resourceKind, resourceName), nil
}

// workloadPodSpecReferencesResource checks if a pod spec references a specific resource
// This is the same logic used in annotation-based targeted reload
func workloadPodSpecReferencesResource(podSpec *corev1.PodSpec, resourceKind, resourceName string) bool {
	if podSpec == nil {
		return false
	}

	// Check environment variables in all containers
	for _, container := range podSpec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if resourceKind == util.KindSecret && env.ValueFrom.SecretKeyRef != nil {
					if env.ValueFrom.SecretKeyRef.Name == resourceName {
						return true
					}
				}
				if resourceKind == util.KindConfigMap && env.ValueFrom.ConfigMapKeyRef != nil {
					if env.ValueFrom.ConfigMapKeyRef.Name == resourceName {
						return true
					}
				}
			}
		}

		// Check envFrom
		for _, envFrom := range container.EnvFrom {
			if resourceKind == util.KindSecret && envFrom.SecretRef != nil {
				if envFrom.SecretRef.Name == resourceName {
					return true
				}
			}
			if resourceKind == util.KindConfigMap && envFrom.ConfigMapRef != nil {
				if envFrom.ConfigMapRef.Name == resourceName {
					return true
				}
			}
		}
	}

	// Check init containers
	for _, container := range podSpec.InitContainers {
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if resourceKind == util.KindSecret && env.ValueFrom.SecretKeyRef != nil {
					if env.ValueFrom.SecretKeyRef.Name == resourceName {
						return true
					}
				}
				if resourceKind == util.KindConfigMap && env.ValueFrom.ConfigMapKeyRef != nil {
					if env.ValueFrom.ConfigMapKeyRef.Name == resourceName {
						return true
					}
				}
			}
		}

		for _, envFrom := range container.EnvFrom {
			if resourceKind == util.KindSecret && envFrom.SecretRef != nil {
				if envFrom.SecretRef.Name == resourceName {
					return true
				}
			}
			if resourceKind == util.KindConfigMap && envFrom.ConfigMapRef != nil {
				if envFrom.ConfigMapRef.Name == resourceName {
					return true
				}
			}
		}
	}

	// Check volumes
	for _, volume := range podSpec.Volumes {
		if resourceKind == util.KindSecret && volume.Secret != nil {
			if volume.Secret.SecretName == resourceName {
				return true
			}
		}
		if resourceKind == util.KindConfigMap && volume.ConfigMap != nil {
			if volume.ConfigMap.Name == resourceName {
				return true
			}
		}
	}

	return false
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
//
// Returns the number of successful reloads.
func (r *ReloaderConfigReconciler) executeReloads(
	ctx context.Context,
	targets []workload.Target,
	resourceKind string,
	resourceName string,
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
	// Initialize the status update queue
	r.statusQueue = workqueue.NewTypedRateLimitingQueue[statusUpdateWorkItem](workqueue.DefaultTypedControllerRateLimiter[statusUpdateWorkItem]())
	r.ctx, r.cancelFunc = context.WithCancel(context.Background())

	// Start the status update worker
	go r.startStatusUpdateWorker()

	// Register cleanup on manager stop
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		<-ctx.Done()
		r.cancelFunc()
		r.statusQueue.ShutDown()
		return nil
	})); err != nil {
		return err
	}

	// Initialize controllers after cache sync to prevent spurious reloads during startup
	// This is a safeguard similar to the original Reloader's secretControllerInitialized flag
	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		// Wait for cache to sync
		if mgr.GetCache().WaitForCacheSync(ctx) {
			r.controllersInitialized.Store(true)
			log.Log.Info("Controllers initialized - CREATE/DELETE events will now be processed")
		}
		// Keep running until context is done
		<-ctx.Done()
		return nil
	})); err != nil {
		return err
	}

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
					// Check label selector first
					if !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
						return false
					}
					// Only process creates if flag is enabled and controllers are initialized
					return r.ReloadOnCreate && r.controllersInitialized.Load()
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// Check label selector first
					if !r.ResourceLabelSelector.Matches(labels.Set(e.ObjectNew.GetLabels())) {
						return false
					}
					// Always process updates (hash check happens in reconcile)
					return true
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// Check label selector first
					if !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
						return false
					}
					// Only process deletes if flag is enabled and controllers are initialized
					return r.ReloadOnDelete && r.controllersInitialized.Load()
				},
			}),
		).

		// Watch ConfigMaps - enqueue requests when ConfigMaps change
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapConfigMapToRequests),
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					// Check label selector first
					if !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
						return false
					}
					// Only process creates if flag is enabled and controllers are initialized
					return r.ReloadOnCreate && r.controllersInitialized.Load()
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					// Check label selector first
					if !r.ResourceLabelSelector.Matches(labels.Set(e.ObjectNew.GetLabels())) {
						return false
					}
					// Always process updates (hash check happens in reconcile)
					return true
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					// Check label selector first
					if !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
						return false
					}
					// Only process deletes if flag is enabled and controllers are initialized
					return r.ReloadOnDelete && r.controllersInitialized.Load()
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
				Kind:             target.Kind,
				Name:             target.Name,
				Namespace:        util.GetDefaultNamespace(target.Namespace, config.Namespace),
				ReloadStrategy:   util.GetDefaultStrategy(target.ReloadStrategy, config.Spec.ReloadStrategy),
				PausePeriod:      target.PausePeriod,
				RequireReference: target.RequireReference,
				Config:           config,
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
