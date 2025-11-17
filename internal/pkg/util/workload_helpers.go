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
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetWorkload fetches a workload by kind, name, and namespace
// This consolidates the duplicate switch logic found in multiple files
func GetWorkload(ctx context.Context, c client.Client, kind, name, namespace string) (client.Object, error) {
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}

	switch kind {
	case KindDeployment:
		obj := &appsv1.Deployment{}
		if err := c.Get(ctx, key, obj); err != nil {
			return nil, err
		}
		return obj, nil

	case KindStatefulSet:
		obj := &appsv1.StatefulSet{}
		if err := c.Get(ctx, key, obj); err != nil {
			return nil, err
		}
		return obj, nil

	case KindDaemonSet:
		obj := &appsv1.DaemonSet{}
		if err := c.Get(ctx, key, obj); err != nil {
			return nil, err
		}
		return obj, nil

	default:
		return nil, fmt.Errorf("unsupported workload kind: %s", kind)
	}
}

// GetPodTemplate extracts the pod template from any workload type
// This consolidates the duplicate switch logic for extracting pod specs
func GetPodTemplate(obj client.Object) (*corev1.PodTemplateSpec, error) {
	switch workload := obj.(type) {
	case *appsv1.Deployment:
		return &workload.Spec.Template, nil
	case *appsv1.StatefulSet:
		return &workload.Spec.Template, nil
	case *appsv1.DaemonSet:
		return &workload.Spec.Template, nil
	default:
		return nil, fmt.Errorf("unsupported workload type: %T", obj)
	}
}

// GetPodSpec extracts the pod spec from any workload type
// This is a convenience function that combines GetPodTemplate with spec extraction
func GetPodSpec(obj client.Object) (*corev1.PodSpec, error) {
	template, err := GetPodTemplate(obj)
	if err != nil {
		return nil, err
	}
	return &template.Spec, nil
}

// WorkloadExists checks if a workload of the given kind exists
// This consolidates duplicate existence checking logic
func WorkloadExists(ctx context.Context, c client.Client, kind, name, namespace string) (bool, error) {
	key := client.ObjectKey{Name: name, Namespace: namespace}

	switch kind {
	case KindDeployment:
		deployment := &appsv1.Deployment{}
		err := c.Get(ctx, key, deployment)
		return err == nil, client.IgnoreNotFound(err)

	case KindStatefulSet:
		statefulSet := &appsv1.StatefulSet{}
		err := c.Get(ctx, key, statefulSet)
		return err == nil, client.IgnoreNotFound(err)

	case KindDaemonSet:
		daemonSet := &appsv1.DaemonSet{}
		err := c.Get(ctx, key, daemonSet)
		return err == nil, client.IgnoreNotFound(err)

	default:
		return false, fmt.Errorf("unsupported workload kind: %s", kind)
	}
}
