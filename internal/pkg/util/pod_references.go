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

package util

import (
	corev1 "k8s.io/api/core/v1"
)

// CheckPodSpecReferencesResource checks if a PodSpec references a specific Secret or ConfigMap
// by examining environment variables, envFrom, and volume mounts.
//
// This function consolidates the duplicate reference-checking logic that was previously
// scattered across multiple files (reconciler_discovery.go, workload/finder.go).
//
// Parameters:
//   - podSpec: The pod specification to check
//   - resourceKind: Either KindSecret or KindConfigMap
//   - resourceName: The name of the resource to look for
//
// Returns:
//   - true if the pod spec references the resource, false otherwise
func CheckPodSpecReferencesResource(podSpec *corev1.PodSpec, resourceKind, resourceName string) bool {
	if podSpec == nil {
		return false
	}

	// Check all containers (both regular and init containers)
	allContainers := append(podSpec.Containers, podSpec.InitContainers...)
	for _, container := range allContainers {
		if checkContainerReferencesResource(container, resourceKind, resourceName) {
			return true
		}
	}

	// Check volumes
	if checkVolumesReferenceResource(podSpec.Volumes, resourceKind, resourceName) {
		return true
	}

	return false
}

// checkContainerReferencesResource checks if a container references a specific resource
// through environment variables or envFrom directives
func checkContainerReferencesResource(container corev1.Container, resourceKind, resourceName string) bool {
	// Check individual environment variables
	for _, env := range container.Env {
		if env.ValueFrom != nil {
			if checkEnvValueFromReferencesResource(env.ValueFrom, resourceKind, resourceName) {
				return true
			}
		}
	}

	// Check envFrom (bulk environment variable sources)
	for _, envFrom := range container.EnvFrom {
		if checkEnvFromReferencesResource(envFrom, resourceKind, resourceName) {
			return true
		}
	}

	return false
}

// checkEnvValueFromReferencesResource checks if an EnvVarSource references a specific resource
func checkEnvValueFromReferencesResource(valueFrom *corev1.EnvVarSource, resourceKind, resourceName string) bool {
	if valueFrom == nil {
		return false
	}

	// Check for Secret reference
	if resourceKind == KindSecret && valueFrom.SecretKeyRef != nil {
		if valueFrom.SecretKeyRef.Name == resourceName {
			return true
		}
	}

	// Check for ConfigMap reference
	if resourceKind == KindConfigMap && valueFrom.ConfigMapKeyRef != nil {
		if valueFrom.ConfigMapKeyRef.Name == resourceName {
			return true
		}
	}

	return false
}

// checkEnvFromReferencesResource checks if an EnvFromSource references a specific resource
func checkEnvFromReferencesResource(envFrom corev1.EnvFromSource, resourceKind, resourceName string) bool {
	// Check for Secret reference
	if resourceKind == KindSecret && envFrom.SecretRef != nil {
		if envFrom.SecretRef.Name == resourceName {
			return true
		}
	}

	// Check for ConfigMap reference
	if resourceKind == KindConfigMap && envFrom.ConfigMapRef != nil {
		if envFrom.ConfigMapRef.Name == resourceName {
			return true
		}
	}

	return false
}

// checkVolumesReferenceResource checks if any volume references a specific resource
func checkVolumesReferenceResource(volumes []corev1.Volume, resourceKind, resourceName string) bool {
	for _, volume := range volumes {
		// Check for Secret volume source
		if resourceKind == KindSecret && volume.Secret != nil {
			if volume.Secret.SecretName == resourceName {
				return true
			}
		}

		// Check for ConfigMap volume source
		if resourceKind == KindConfigMap && volume.ConfigMap != nil {
			if volume.ConfigMap.Name == resourceName {
				return true
			}
		}

		// Check for projected volumes (can contain Secrets/ConfigMaps)
		if volume.Projected != nil {
			for _, source := range volume.Projected.Sources {
				if resourceKind == KindSecret && source.Secret != nil {
					if source.Secret.Name == resourceName {
						return true
					}
				}
				if resourceKind == KindConfigMap && source.ConfigMap != nil {
					if source.ConfigMap.Name == resourceName {
						return true
					}
				}
			}
		}
	}

	return false
}
