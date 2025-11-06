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
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/util"
)

var scheme *runtime.Scheme

func init() {
	scheme = runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = reloaderv1alpha1.AddToScheme(scheme)
}

func TestFindReloaderConfigsWatchingResource(t *testing.T) {
	tests := []struct {
		name             string
		configs          []*reloaderv1alpha1.ReloaderConfig
		resourceKind     string
		resourceName     string
		resourceNS       string
		expectedCount    int
		expectAutoReload bool
	}{
		{
			name: "finds config with explicit secret watch",
			configs: []*reloaderv1alpha1.ReloaderConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "default",
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							Secrets: []string{"my-secret"},
						},
						Targets: []reloaderv1alpha1.TargetWorkload{
							{Kind: "Deployment", Name: "app1"},
						},
					},
				},
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "finds config with explicit configmap watch",
			configs: []*reloaderv1alpha1.ReloaderConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "default",
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							ConfigMaps: []string{"my-config"},
						},
						Targets: []reloaderv1alpha1.TargetWorkload{
							{Kind: "Deployment", Name: "app1"},
						},
					},
				},
			},
			resourceKind:  util.KindConfigMap,
			resourceName:  "my-config",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "does not find config watching different resource",
			configs: []*reloaderv1alpha1.ReloaderConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "default",
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							Secrets: []string{"other-secret"},
						},
					},
				},
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 0,
		},
		{
			name: "skips configs with ignore annotation",
			configs: []*reloaderv1alpha1.ReloaderConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationIgnore: "true",
						},
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							Secrets: []string{"my-secret"},
						},
					},
				},
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := make([]runtime.Object, len(tt.configs))
			for i, config := range tt.configs {
				objects[i] = config
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
			finder := NewFinder(fakeClient)

			configs, err := finder.FindReloaderConfigsWatchingResource(
				context.Background(),
				tt.resourceKind,
				tt.resourceName,
				tt.resourceNS,
			)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(configs) != tt.expectedCount {
				t.Errorf("expected %d configs, got %d", tt.expectedCount, len(configs))
			}
		})
	}
}

func TestFindWorkloadsWithAnnotations(t *testing.T) {
	tests := []struct {
		name          string
		deployments   []*appsv1.Deployment
		resourceKind  string
		resourceName  string
		resourceNS    string
		expectedCount int
	}{
		{
			name: "finds deployment with secret reload annotation",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationSecretReload: "my-secret",
						},
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
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "finds deployment with configmap reload annotation",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationConfigMapReload: "my-config",
						},
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
			},
			resourceKind:  util.KindConfigMap,
			resourceName:  "my-config",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "finds deployment with comma-separated reload list",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationSecretReload: "secret1,my-secret,secret2",
						},
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
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "does not find deployment without matching annotation",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationSecretReload: "other-secret",
						},
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
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create objects slice for fake client
			objects := make([]runtime.Object, len(tt.deployments))
			for i, deployment := range tt.deployments {
				objects[i] = deployment
			}

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
			finder := NewFinder(fakeClient)

			targets, err := finder.FindWorkloadsWithAnnotations(
				context.Background(),
				tt.resourceKind,
				tt.resourceName,
				tt.resourceNS,
				nil, // resourceAnnotations
			)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(targets) != tt.expectedCount {
				t.Errorf("expected %d targets, got %d", tt.expectedCount, len(targets))
			}
		})
	}
}

// TestFindWorkloadsWithAnnotations_SearchAndMatch tests the targeted reload feature
func TestFindWorkloadsWithAnnotations_SearchAndMatch(t *testing.T) {
	tests := []struct {
		name                string
		workloadAnnotations map[string]string
		resourceAnnotations map[string]string
		referencesResource  bool
		expectedReload      bool
		description         string
	}{
		{
			name: "search true, match true, referenced - should reload",
			workloadAnnotations: map[string]string{
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "true",
			},
			referencesResource: true,
			expectedReload:     true,
			description:        "All three conditions met: search annotation, match annotation, and resource reference",
		},
		{
			name: "search true, match false, referenced - should NOT reload",
			workloadAnnotations: map[string]string{
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "false",
			},
			referencesResource: true,
			expectedReload:     false,
			description:        "Match explicitly set to false, so no reload",
		},
		{
			name: "search true, no match annotation, referenced - should NOT reload",
			workloadAnnotations: map[string]string{
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{},
			referencesResource:  true,
			expectedReload:      false,
			description:         "Resource lacks match annotation, so no reload",
		},
		{
			name: "search true, match true, NOT referenced - should NOT reload",
			workloadAnnotations: map[string]string{
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "true",
			},
			referencesResource: false,
			expectedReload:     false,
			description:        "Resource not referenced in pod spec, so no reload",
		},
		{
			name: "auto true takes precedence over search",
			workloadAnnotations: map[string]string{
				util.AnnotationAuto:   "true",
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{},
			referencesResource:  true,
			expectedReload:      true,
			description:         "Auto annotation takes precedence, search and match are ignored",
		},
		{
			name: "auto false blocks search",
			workloadAnnotations: map[string]string{
				util.AnnotationAuto:   "false",
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "true",
			},
			referencesResource: true,
			expectedReload:     false,
			description:        "Auto false explicitly disables reload, blocking search+match",
		},
		{
			name:                "no search annotation - should NOT reload",
			workloadAnnotations: map[string]string{
				// No search annotation
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "true",
			},
			referencesResource: true,
			expectedReload:     false,
			description:        "Workload lacks search annotation, so no targeted reload",
		},
		{
			name: "ignore annotation blocks search+match",
			workloadAnnotations: map[string]string{
				util.AnnotationSearch: "true",
				util.AnnotationIgnore: "true",
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "true",
			},
			referencesResource: true,
			expectedReload:     false,
			description:        "Ignore annotation takes precedence and blocks reload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create deployment with test annotations
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-deployment",
					Namespace:   "default",
					Annotations: tt.workloadAnnotations,
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
									Name:  "test",
									Image: "test:latest",
								},
							},
						},
					},
				},
			}

			// Add resource reference if test requires it
			if tt.referencesResource {
				deployment.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{
					{
						Name: "TEST_SECRET",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "test-secret",
								},
								Key: "key",
							},
						},
					},
				}
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(deployment).
				Build()

			finder := NewFinder(fakeClient)

			targets, err := finder.FindWorkloadsWithAnnotations(
				context.Background(),
				util.KindSecret,
				"test-secret",
				"default",
				tt.resourceAnnotations,
			)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			found := len(targets) > 0
			if found != tt.expectedReload {
				t.Errorf("%s: expected reload=%v, got reload=%v", tt.description, tt.expectedReload, found)
			}
		})
	}
}

// TestShouldReloadFromAnnotations_Precedence tests annotation precedence rules
func TestShouldReloadFromAnnotations_Precedence(t *testing.T) {
	tests := []struct {
		name                string
		workloadAnnotations map[string]string
		resourceAnnotations map[string]string
		expectedReload      bool
		description         string
	}{
		{
			name: "auto true wins over search",
			workloadAnnotations: map[string]string{
				util.AnnotationAuto:   "true",
				util.AnnotationSearch: "true",
			},
			resourceAnnotations: map[string]string{},
			expectedReload:      true,
			description:         "Auto annotation should take precedence, ignore search",
		},
		{
			name: "auto false blocks everything",
			workloadAnnotations: map[string]string{
				util.AnnotationAuto:          "false",
				util.AnnotationSearch:        "true",
				util.AnnotationSecretReload:  "test-secret",
				util.AnnotationSecretAuto:    "true",
				util.AnnotationConfigMapAuto: "true",
			},
			resourceAnnotations: map[string]string{
				util.AnnotationMatch: "true",
			},
			expectedReload: false,
			description:    "Auto false should block all other reload mechanisms",
		},
		{
			name: "type-specific auto works",
			workloadAnnotations: map[string]string{
				util.AnnotationSecretAuto: "true",
			},
			resourceAnnotations: map[string]string{},
			expectedReload:      true,
			description:         "Type-specific auto should trigger reload",
		},
		{
			name: "named reload works",
			workloadAnnotations: map[string]string{
				util.AnnotationSecretReload: "test-secret",
			},
			resourceAnnotations: map[string]string{},
			expectedReload:      true,
			description:         "Named reload annotation should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-deployment",
					Namespace:   "default",
					Annotations: tt.workloadAnnotations,
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
									Name:  "test",
									Image: "test:latest",
									Env: []corev1.EnvVar{
										{
											Name: "TEST_SECRET",
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "test-secret",
													},
													Key: "key",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}

			result := shouldReloadFromAnnotations(
				deployment,
				util.KindSecret,
				"test-secret",
				tt.resourceAnnotations,
			)

			if result != tt.expectedReload {
				t.Errorf("%s: expected %v, got %v", tt.description, tt.expectedReload, result)
			}
		})
	}
}
