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
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/util"
	"github.com/stakater/Reloader/internal/pkg/workload"
)

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
	// Fetch the workload using consolidated utility function
	obj, err := util.GetWorkload(ctx, r.Client, target.Kind, target.Name, target.Namespace)
	if err != nil {
		return false, err
	}

	// Extract pod spec using consolidated utility function
	podSpec, err := util.GetPodSpec(obj)
	if err != nil {
		return false, err
	}

	// Check if pod spec references the resource
	return workloadPodSpecReferencesResource(podSpec, resourceKind, resourceName), nil
}

// workloadPodSpecReferencesResource checks if a pod spec references a specific resource
// This is the same logic used in annotation-based targeted reload
func workloadPodSpecReferencesResource(podSpec *corev1.PodSpec, resourceKind, resourceName string) bool {
	return util.CheckPodSpecReferencesResource(podSpec, resourceKind, resourceName)
}

// workloadExists checks if a workload of the given kind exists
func (r *ReloaderConfigReconciler) workloadExists(ctx context.Context, kind, name, namespace string) (bool, error) {
	return util.WorkloadExists(ctx, r.Client, kind, name, namespace)
}

// shouldProcessNamespace checks if a namespace should be processed based on filters
func (r *ReloaderConfigReconciler) shouldProcessNamespace(ctx context.Context, namespace string) bool {
	// Check if namespace is in the ignored list
	if r.IgnoredNamespaces[namespace] {
		return false
	}

	// If namespace selector is set, check if namespace matches
	if r.NamespaceSelector != nil && !r.NamespaceSelector.Empty() {
		// Fetch the namespace object to check its labels
		ns := &corev1.Namespace{}
		if err := r.Get(ctx, client.ObjectKey{Name: namespace}, ns); err != nil {
			// If we can't fetch the namespace, log and skip it
			log.FromContext(ctx).Error(err, "Failed to fetch namespace for filtering", "namespace", namespace)
			return false
		}

		// Check if namespace labels match the selector
		if !r.NamespaceSelector.Matches(labels.Set(ns.Labels)) {
			return false
		}
	}

	return true
}

// mergeTargets merges targets from ReloaderConfigs and annotation-based workloads
func (r *ReloaderConfigReconciler) mergeTargets(
	configs []*reloaderv1alpha1.ReloaderConfig,
	annotatedWorkloads []workload.Target,
) []workload.Target {
	allTargets := []workload.Target{}

	// Add targets from ReloaderConfigs
	for _, config := range configs {
		// Get default strategies from config or global
		defaultRolloutStrategy := util.GetDefaultRolloutStrategy(config.Spec.RolloutStrategy, r.RolloutStrategy)
		defaultReloadStrategy := util.GetDefaultReloadStrategy(config.Spec.ReloadStrategy, r.ReloadStrategy)

		for _, target := range config.Spec.Targets {
			allTargets = append(allTargets, workload.Target{
				Kind:             target.Kind,
				Name:             target.Name,
				Namespace:        util.GetDefaultNamespace(target.Namespace, config.Namespace),
				RolloutStrategy:  util.GetDefaultRolloutStrategy(target.RolloutStrategy, defaultRolloutStrategy),
				ReloadStrategy:   util.GetDefaultReloadStrategy(target.ReloadStrategy, defaultReloadStrategy),
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
