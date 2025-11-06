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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/util"
)

// Target represents a workload that needs to be reloaded
type Target struct {
	Kind           string
	Name           string
	Namespace      string
	ReloadStrategy string
	PausePeriod    string
	Config         *reloaderv1alpha1.ReloaderConfig // Reference to the ReloaderConfig that triggered this
}

// Finder discovers workloads that need to be reloaded
type Finder struct {
	client.Client
}

// NewFinder creates a new workload finder
func NewFinder(c client.Client) *Finder {
	return &Finder{Client: c}
}

// FindReloaderConfigsWatchingResource finds all ReloaderConfigs that watch a specific resource
func (f *Finder) FindReloaderConfigsWatchingResource(
	ctx context.Context,
	resourceKind, resourceName, resourceNamespace string,
) ([]*reloaderv1alpha1.ReloaderConfig, error) {
	logger := log.FromContext(ctx)

	// List all ReloaderConfigs in the same namespace
	configList := &reloaderv1alpha1.ReloaderConfigList{}
	if err := f.List(ctx, configList, client.InNamespace(resourceNamespace)); err != nil {
		return nil, err
	}

	result := []*reloaderv1alpha1.ReloaderConfig{}

	for i := range configList.Items {
		config := &configList.Items[i]

		// Skip if ignored
		if config.Annotations != nil && config.Annotations[util.AnnotationIgnore] == "true" {
			continue
		}

		// Check if this config explicitly watches the resource
		if f.configWatchesResource(config, resourceKind, resourceName) {
			result = append(result, config)
			logger.V(1).Info("Found ReloaderConfig watching resource",
				"config", config.Name,
				"resource", resourceKind+"/"+resourceName)
			continue
		}

		// Check if autoReloadAll is enabled
		if config.Spec.AutoReloadAll {
			// Check if any target workload references this resource
			if f.anyTargetReferencesResource(ctx, config, resourceKind, resourceName, resourceNamespace) {
				result = append(result, config)
				logger.V(1).Info("Found ReloaderConfig with autoReloadAll referencing resource",
					"config", config.Name,
					"resource", resourceKind+"/"+resourceName)
			}
		}
	}

	return result, nil
}

// configWatchesResource checks if a ReloaderConfig explicitly watches a resource
func (f *Finder) configWatchesResource(config *reloaderv1alpha1.ReloaderConfig, kind, name string) bool {
	if config.Spec.WatchedResources == nil {
		return false
	}

	var watchList []string
	if kind == util.KindSecret {
		watchList = config.Spec.WatchedResources.Secrets
	} else if kind == util.KindConfigMap {
		watchList = config.Spec.WatchedResources.ConfigMaps
	}

	return util.ContainsString(watchList, name)
}

// anyTargetReferencesResource checks if any target workload references the resource
func (f *Finder) anyTargetReferencesResource(
	ctx context.Context,
	config *reloaderv1alpha1.ReloaderConfig,
	resourceKind, resourceName, resourceNamespace string,
) bool {
	for _, target := range config.Spec.Targets {
		targetNs := util.GetDefaultNamespace(target.Namespace, config.Namespace)

		// Only check workloads in the same namespace as the resource
		if targetNs != resourceNamespace {
			continue
		}

		if f.workloadReferencesResource(ctx, target.Kind, target.Name, targetNs, resourceKind, resourceName) {
			return true
		}
	}

	return false
}

// workloadReferencesResource checks if a workload references a Secret or ConfigMap
func (f *Finder) workloadReferencesResource(
	ctx context.Context,
	workloadKind, workloadName, workloadNamespace,
	resourceKind, resourceName string,
) bool {
	key := client.ObjectKey{Name: workloadName, Namespace: workloadNamespace}

	switch workloadKind {
	case util.KindDeployment:
		deployment := &appsv1.Deployment{}
		if err := f.Get(ctx, key, deployment); err != nil {
			return false
		}
		return podTemplateReferencesResource(&deployment.Spec.Template, resourceKind, resourceName)

	case util.KindStatefulSet:
		statefulSet := &appsv1.StatefulSet{}
		if err := f.Get(ctx, key, statefulSet); err != nil {
			return false
		}
		return podTemplateReferencesResource(&statefulSet.Spec.Template, resourceKind, resourceName)

	case util.KindDaemonSet:
		daemonSet := &appsv1.DaemonSet{}
		if err := f.Get(ctx, key, daemonSet); err != nil {
			return false
		}
		return podTemplateReferencesResource(&daemonSet.Spec.Template, resourceKind, resourceName)
	}

	return false
}

// podTemplateReferencesResource checks if a pod template references a resource
func podTemplateReferencesResource(template *corev1.PodTemplateSpec, resourceKind, resourceName string) bool {
	// Check volumes
	for _, volume := range template.Spec.Volumes {
		if resourceKind == util.KindSecret && volume.Secret != nil && volume.Secret.SecretName == resourceName {
			return true
		}
		if resourceKind == util.KindConfigMap && volume.ConfigMap != nil && volume.ConfigMap.Name == resourceName {
			return true
		}
	}

	// Check envFrom in all containers
	for _, container := range template.Spec.Containers {
		for _, envFrom := range container.EnvFrom {
			if resourceKind == util.KindSecret && envFrom.SecretRef != nil && envFrom.SecretRef.Name == resourceName {
				return true
			}
			if resourceKind == util.KindConfigMap && envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name == resourceName {
				return true
			}
		}

		// Check env (individual keys)
		for _, env := range container.Env {
			if env.ValueFrom != nil {
				if resourceKind == util.KindSecret && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name == resourceName {
					return true
				}
				if resourceKind == util.KindConfigMap && env.ValueFrom.ConfigMapKeyRef != nil && env.ValueFrom.ConfigMapKeyRef.Name == resourceName {
					return true
				}
			}
		}
	}

	// Check init containers
	for _, container := range template.Spec.InitContainers {
		for _, envFrom := range container.EnvFrom {
			if resourceKind == util.KindSecret && envFrom.SecretRef != nil && envFrom.SecretRef.Name == resourceName {
				return true
			}
			if resourceKind == util.KindConfigMap && envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name == resourceName {
				return true
			}
		}
	}

	return false
}

// FindWorkloadsWithAnnotations finds workloads that have annotation-based reload config
func (f *Finder) FindWorkloadsWithAnnotations(
	ctx context.Context,
	resourceKind, resourceName, resourceNamespace string,
	resourceAnnotations map[string]string,
) ([]Target, error) {
	logger := log.FromContext(ctx)
	targets := []Target{}

	// Check Deployments
	deployments := &appsv1.DeploymentList{}
	if err := f.List(ctx, deployments, client.InNamespace(resourceNamespace)); err != nil {
		return nil, err
	}

	for _, deploy := range deployments.Items {
		if shouldReloadFromAnnotations(&deploy, resourceKind, resourceName, resourceAnnotations) {
			strategy := util.GetDefaultStrategy(
				deploy.Annotations[util.AnnotationReloadStrategy],
				util.ReloadStrategyEnvVars,
			)
			pausePeriod := deploy.Annotations[util.AnnotationDeploymentPausePeriod]

			targets = append(targets, Target{
				Kind:           util.KindDeployment,
				Name:           deploy.Name,
				Namespace:      deploy.Namespace,
				ReloadStrategy: strategy,
				PausePeriod:    pausePeriod,
				Config:         nil, // No ReloaderConfig for annotation-based
			})

			logger.V(1).Info("Found Deployment with annotations",
				"deployment", deploy.Name,
				"resource", resourceKind+"/"+resourceName)
		}
	}

	// Check StatefulSets
	statefulSets := &appsv1.StatefulSetList{}
	if err := f.List(ctx, statefulSets, client.InNamespace(resourceNamespace)); err != nil {
		return nil, err
	}

	for _, sts := range statefulSets.Items {
		if shouldReloadFromAnnotations(&sts, resourceKind, resourceName, resourceAnnotations) {
			strategy := util.GetDefaultStrategy(
				sts.Annotations[util.AnnotationReloadStrategy],
				util.ReloadStrategyEnvVars,
			)
			pausePeriod := sts.Annotations[util.AnnotationStatefulSetPausePeriod]

			targets = append(targets, Target{
				Kind:           util.KindStatefulSet,
				Name:           sts.Name,
				Namespace:      sts.Namespace,
				ReloadStrategy: strategy,
				PausePeriod:    pausePeriod,
				Config:         nil,
			})

			logger.V(1).Info("Found StatefulSet with annotations",
				"statefulset", sts.Name,
				"resource", resourceKind+"/"+resourceName)
		}
	}

	// Check DaemonSets
	daemonSets := &appsv1.DaemonSetList{}
	if err := f.List(ctx, daemonSets, client.InNamespace(resourceNamespace)); err != nil {
		return nil, err
	}

	for _, ds := range daemonSets.Items {
		if shouldReloadFromAnnotations(&ds, resourceKind, resourceName, resourceAnnotations) {
			strategy := util.GetDefaultStrategy(
				ds.Annotations[util.AnnotationReloadStrategy],
				util.ReloadStrategyEnvVars,
			)
			pausePeriod := ds.Annotations[util.AnnotationDaemonSetPausePeriod]

			targets = append(targets, Target{
				Kind:           util.KindDaemonSet,
				Name:           ds.Name,
				Namespace:      ds.Namespace,
				ReloadStrategy: strategy,
				PausePeriod:    pausePeriod,
				Config:         nil,
			})

			logger.V(1).Info("Found DaemonSet with annotations",
				"daemonset", ds.Name,
				"resource", resourceKind+"/"+resourceName)
		}
	}

	return targets, nil
}

// shouldReloadFromAnnotations checks if a workload should reload based on annotations
func shouldReloadFromAnnotations(obj client.Object, resourceKind, resourceName string, resourceAnnotations map[string]string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	// Check if ignored
	if annotations[util.AnnotationIgnore] == "true" {
		return false
	}

	// Get pod template spec based on workload type
	var podSpec *corev1.PodSpec
	switch v := obj.(type) {
	case *appsv1.Deployment:
		podSpec = &v.Spec.Template.Spec
	case *appsv1.StatefulSet:
		podSpec = &v.Spec.Template.Spec
	case *appsv1.DaemonSet:
		podSpec = &v.Spec.Template.Spec
	}

	// Rule 1: Check auto-reload (takes precedence over search)
	autoValue := annotations[util.AnnotationAuto]
	if autoValue == "true" {
		if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
			return true
		}
	} else if autoValue == "false" {
		// Explicitly disabled - no reload regardless of other annotations
		return false
	}

	// Rule 2: Check type-specific auto
	if resourceKind == util.KindSecret && annotations[util.AnnotationSecretAuto] == "true" {
		if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
			return true
		}
	}
	if resourceKind == util.KindConfigMap && annotations[util.AnnotationConfigMapAuto] == "true" {
		if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
			return true
		}
	}

	// Rule 3: Check specific reload lists (named reload)
	var reloadList string
	if resourceKind == util.KindSecret {
		reloadList = annotations[util.AnnotationSecretReload]
	} else if resourceKind == util.KindConfigMap {
		reloadList = annotations[util.AnnotationConfigMapReload]
	}

	if reloadList != "" {
		names := util.ParseCommaSeparatedList(reloadList)
		return util.ContainsString(names, resourceName)
	}

	// Rule 4: Check targeted reload (search + match)
	// Workload must have search: "true" AND resource must have match: "true" AND resource must be referenced
	searchValue := annotations[util.AnnotationSearch]
	if searchValue == "true" {
		// Workload is in search mode
		// Check if resource has match annotation
		if resourceAnnotations != nil && resourceAnnotations[util.AnnotationMatch] == "true" {
			// Check if resource is referenced in pod spec
			if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
				return true
			}
		}
	}

	return false
}

// workloadReferencesResource checks if a pod spec references a specific resource
func workloadReferencesResource(podSpec *corev1.PodSpec, resourceKind, resourceName string) bool {
	// Check environment variables
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
