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
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stakater/Reloader/internal/pkg/util"
)

func TestTriggerReloadEnvVarsStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(deployment).Build()
	updater := NewUpdater(fakeClient)

	target := Target{
		Kind:            util.KindDeployment,
		Name:            "test-app",
		Namespace:       "default",
		RolloutStrategy: util.RolloutStrategyRollout,
		ReloadStrategy:  util.ReloadStrategyEnvVars,
	}

	err := updater.TriggerReload(context.Background(), target, util.KindSecret, "db-secret", "default", "test-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the deployment was updated
	updatedDeployment := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-app",
		Namespace: "default",
	}, updatedDeployment)
	if err != nil {
		t.Fatalf("failed to get updated deployment: %v", err)
	}

	// Check that RELOADER_TRIGGERED_AT env var was added
	found := false
	for _, container := range updatedDeployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == util.EnvReloaderTriggeredAt {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("RELOADER_TRIGGERED_AT env var not found in deployment")
	}

	// Check that NO annotations were added (env-vars strategy should only update env var)
	if updatedDeployment.Spec.Template.Annotations != nil {
		if _, exists := updatedDeployment.Spec.Template.Annotations[util.AnnotationLastReloadedFrom]; exists {
			t.Error("env-vars strategy should not add annotations")
		}
	}
}

func TestTriggerReloadAnnotationsStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(deployment).Build()
	updater := NewUpdater(fakeClient)

	target := Target{
		Kind:            util.KindDeployment,
		Name:            "test-app",
		Namespace:       "default",
		RolloutStrategy: util.RolloutStrategyRollout,
		ReloadStrategy:  util.ReloadStrategyAnnotations,
	}

	err := updater.TriggerReload(context.Background(), target, util.KindConfigMap, "app-config", "default", "test-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the deployment was updated
	updatedDeployment := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-app",
		Namespace: "default",
	}, updatedDeployment)
	if err != nil {
		t.Fatalf("failed to get updated deployment: %v", err)
	}

	// Check that reload annotation was added
	if updatedDeployment.Spec.Template.Annotations == nil {
		t.Fatal("pod template annotations should not be nil")
	}

	if updatedDeployment.Spec.Template.Annotations[util.AnnotationLastReload] == "" {
		t.Error("LAST_RELOAD annotation not found in pod template")
	}

	// Check that reload source annotation was added with JSON format
	reloadSourceJSON := updatedDeployment.Spec.Template.Annotations[util.AnnotationLastReloadedFrom]
	if reloadSourceJSON == "" {
		t.Error("reload source annotation not found in pod template")
	}

	// Verify it contains JSON with expected fields
	if !strings.Contains(reloadSourceJSON, "\"kind\":\"ConfigMap\"") || !strings.Contains(reloadSourceJSON, "\"name\":\"app-config\"") {
		t.Errorf("expected JSON metadata in annotation, got '%s'", reloadSourceJSON)
	}

	// Check that env var was NOT added (annotations strategy)
	for _, container := range updatedDeployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == util.EnvReloaderTriggeredAt {
				t.Error("env-vars strategy should not be used when annotations strategy is specified")
			}
		}
	}
}

func TestTriggerReloadStatefulSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	statefulset := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis:latest",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(statefulset).Build()
	updater := NewUpdater(fakeClient)

	target := Target{
		Kind:            util.KindStatefulSet,
		Name:            "test-sts",
		Namespace:       "default",
		RolloutStrategy: util.RolloutStrategyRollout,
		ReloadStrategy:  util.ReloadStrategyEnvVars,
	}

	err := updater.TriggerReload(context.Background(), target, util.KindSecret, "redis-secret", "default", "test-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the statefulset was updated
	updatedSts := &appsv1.StatefulSet{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-sts",
		Namespace: "default",
	}, updatedSts)
	if err != nil {
		t.Fatalf("failed to get updated statefulset: %v", err)
	}

	// Check that env var was added
	found := false
	for _, container := range updatedSts.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == util.EnvReloaderTriggeredAt {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("RELOADER_TRIGGERED_AT env var not found in statefulset")
	}
}

func TestTriggerReloadDaemonSet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	daemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ds",
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "fluentd",
							Image: "fluentd:latest",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(daemonset).Build()
	updater := NewUpdater(fakeClient)

	target := Target{
		Kind:            util.KindDaemonSet,
		Name:            "test-ds",
		Namespace:       "default",
		RolloutStrategy: util.RolloutStrategyRollout,
		ReloadStrategy:  util.ReloadStrategyEnvVars,
	}

	err := updater.TriggerReload(context.Background(), target, util.KindConfigMap, "fluentd-config", "default", "test-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the daemonset was updated
	updatedDs := &appsv1.DaemonSet{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-ds",
		Namespace: "default",
	}, updatedDs)
	if err != nil {
		t.Fatalf("failed to get updated daemonset: %v", err)
	}

	// Check that env var was added
	found := false
	for _, container := range updatedDs.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == util.EnvReloaderTriggeredAt {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("RELOADER_TRIGGERED_AT env var not found in daemonset")
	}
}

func TestIsPaused(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	tests := []struct {
		name           string
		deployment     *appsv1.Deployment
		target         Target
		expectedPaused bool
	}{
		{
			name: "not paused - no pause period",
			deployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: "test", Image: "test"},
							},
						},
					},
				},
			},
			target: Target{
				Kind:      util.KindDeployment,
				Name:      "test-app",
				Namespace: "default",
				// No pause period set
			},
			expectedPaused: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.deployment).Build()
			updater := NewUpdater(fakeClient)

			isPaused, err := updater.IsPaused(context.Background(), tt.target)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if isPaused != tt.expectedPaused {
				t.Errorf("expected paused=%v, got %v", tt.expectedPaused, isPaused)
			}
		})
	}
}

func TestTriggerReloadRestartStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	// Create some pods for the deployment
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-pod-1",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:latest"},
			},
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-pod-2",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "nginx", Image: "nginx:latest"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(deployment, pod1, pod2).
		Build()
	updater := NewUpdater(fakeClient)

	target := Target{
		Kind:            util.KindDeployment,
		Name:            "test-app",
		Namespace:       "default",
		RolloutStrategy: util.RolloutStrategyRestart,
	}

	err := updater.TriggerReload(context.Background(), target, util.KindSecret, "app-secret", "default", "test-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the deployment template was NOT updated (restart strategy doesn't modify template)
	updatedDeployment := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-app",
		Namespace: "default",
	}, updatedDeployment)
	if err != nil {
		t.Fatalf("failed to get updated deployment: %v", err)
	}

	// Check that env var was NOT added (restart strategy doesn't modify template)
	for _, container := range updatedDeployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == util.EnvReloaderTriggeredAt {
				t.Error("restart strategy should not modify pod template with env vars")
			}
		}
	}

	// Check that annotations were NOT added (restart strategy doesn't modify template)
	if updatedDeployment.Spec.Template.Annotations != nil {
		if _, exists := updatedDeployment.Spec.Template.Annotations[util.AnnotationLastReload]; exists {
			t.Error("restart strategy should not modify pod template annotations")
		}
	}

	// Verify pods were deleted (fake client removes them on delete)
	podList := &corev1.PodList{}
	err = fakeClient.List(context.Background(), podList,
		client.InNamespace("default"),
		client.MatchingLabels(map[string]string{"app": "test"}))
	if err != nil {
		t.Fatalf("failed to list pods: %v", err)
	}

	// In fake client, pods are actually removed when deleted
	if len(podList.Items) == 2 {
		t.Error("pods should have been deleted with restart strategy")
	}
}

func TestTriggerReloadDefaultReloadStrategy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(deployment).Build()
	updater := NewUpdater(fakeClient)

	// Use "rollout" rollout strategy with no reload strategy specified (should default to env-vars)
	target := Target{
		Kind:            util.KindDeployment,
		Name:            "test-app",
		Namespace:       "default",
		RolloutStrategy: util.RolloutStrategyRollout,
		// ReloadStrategy not specified - should default to env-vars
	}

	err := updater.TriggerReload(context.Background(), target, util.KindConfigMap, "nginx-config", "default", "test-hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the deployment was updated
	updatedDeployment := &appsv1.Deployment{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      "test-app",
		Namespace: "default",
	}, updatedDeployment)
	if err != nil {
		t.Fatalf("failed to get updated deployment: %v", err)
	}

	// Check that RELOADER_TRIGGERED_AT env var was added (default reload strategy is env-vars)
	found := false
	for _, container := range updatedDeployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == util.EnvReloaderTriggeredAt {
				found = true
				break
			}
		}
	}

	if !found {
		t.Error("when reload strategy not specified, should default to env-vars and add RELOADER_TRIGGERED_AT env var")
	}
}
