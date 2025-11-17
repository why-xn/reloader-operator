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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

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
	statusQueue     workqueue.TypedRateLimitingInterface[statusUpdateWorkItem]
	ctx             context.Context
	cancelFunc      context.CancelFunc

	// Global flags for reload behavior
	ReloadOnCreate bool
	ReloadOnDelete bool

	// Resource filtering
	ResourceLabelSelector labels.Selector

	// Namespace filtering
	NamespaceSelector labels.Selector
	IgnoredNamespaces map[string]bool

	// Initialization tracking (safeguard to prevent processing events during startup)
	controllersInitialized atomic.Bool
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

// RBAC permissions for Namespaces (required for namespace filtering)
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

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
			builder.WithPredicates(r.secretPredicates()),
		).
		// Watch ConfigMaps - enqueue requests when ConfigMaps change
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapConfigMapToRequests),
			builder.WithPredicates(r.configMapPredicates()),
		).
		Named("reloaderconfig").
		Complete(r)
}
