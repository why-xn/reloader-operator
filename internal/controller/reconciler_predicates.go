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

	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// secretPredicates returns predicate functions for Secret event filtering
func (r *ReloaderConfigReconciler) secretPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Check namespace filtering first
			if !r.shouldProcessNamespace(context.Background(), e.Object.GetNamespace()) {
				return false
			}
			// Check label selector (if configured)
			if r.ResourceLabelSelector != nil && !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
				return false
			}
			// Always process creates after controllers are initialized
			// The ReloadOnCreate flag controls whether workloads are reloaded, not whether we track the resource
			return r.controllersInitialized.Load()
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Check namespace filtering first
			if !r.shouldProcessNamespace(context.Background(), e.ObjectNew.GetNamespace()) {
				return false
			}
			// Check label selector (if configured)
			if r.ResourceLabelSelector != nil && !r.ResourceLabelSelector.Matches(labels.Set(e.ObjectNew.GetLabels())) {
				return false
			}
			// Always process updates (hash check happens in reconcile)
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Check namespace filtering first
			if !r.shouldProcessNamespace(context.Background(), e.Object.GetNamespace()) {
				return false
			}
			// Check label selector (if configured)
			if r.ResourceLabelSelector != nil && !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
				return false
			}
			// Only process deletes if flag is enabled and controllers are initialized
			return r.ReloadOnDelete && r.controllersInitialized.Load()
		},
	}
}

// configMapPredicates returns predicate functions for ConfigMap event filtering
func (r *ReloaderConfigReconciler) configMapPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Check namespace filtering first
			if !r.shouldProcessNamespace(context.Background(), e.Object.GetNamespace()) {
				return false
			}
			// Check label selector (if configured)
			if r.ResourceLabelSelector != nil && !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
				return false
			}
			// Always process creates after controllers are initialized
			// The ReloadOnCreate flag controls whether workloads are reloaded, not whether we track the resource
			return r.controllersInitialized.Load()
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			// Check namespace filtering first
			if !r.shouldProcessNamespace(context.Background(), e.ObjectNew.GetNamespace()) {
				return false
			}
			// Check label selector (if configured)
			if r.ResourceLabelSelector != nil && !r.ResourceLabelSelector.Matches(labels.Set(e.ObjectNew.GetLabels())) {
				return false
			}
			// Always process updates (hash check happens in reconcile)
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Check namespace filtering first
			if !r.shouldProcessNamespace(context.Background(), e.Object.GetNamespace()) {
				return false
			}
			// Check label selector (if configured)
			if r.ResourceLabelSelector != nil && !r.ResourceLabelSelector.Matches(labels.Set(e.Object.GetLabels())) {
				return false
			}
			// Only process deletes if flag is enabled and controllers are initialized
			return r.ReloadOnDelete && r.controllersInitialized.Load()
		},
	}
}
