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

func TestFindReloaderConfigsWatchingResource(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = reloaderv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name          string
		configs       []*reloaderv1alpha1.ReloaderConfig
		resourceKind  string
		resourceName  string
		resourceNS    string
		expectedCount int
	}{
		{
			name: "finds config watching secret",
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
					},
					Status: reloaderv1alpha1.ReloaderConfigStatus{
						WatchedResourceHashes: make(map[string]string),
					},
				},
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "finds config watching configmap",
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
					},
					Status: reloaderv1alpha1.ReloaderConfigStatus{
						WatchedResourceHashes: make(map[string]string),
					},
				},
			},
			resourceKind:  util.KindConfigMap,
			resourceName:  "my-config",
			resourceNS:    "default",
			expectedCount: 1,
		},
		{
			name: "finds multiple configs watching same resource",
			configs: []*reloaderv1alpha1.ReloaderConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "default",
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							Secrets: []string{"shared-secret"},
						},
					},
					Status: reloaderv1alpha1.ReloaderConfigStatus{
						WatchedResourceHashes: make(map[string]string),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config2",
						Namespace: "default",
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							Secrets: []string{"shared-secret"},
						},
					},
					Status: reloaderv1alpha1.ReloaderConfigStatus{
						WatchedResourceHashes: make(map[string]string),
					},
				},
			},
			resourceKind:  util.KindSecret,
			resourceName:  "shared-secret",
			resourceNS:    "default",
			expectedCount: 2,
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
					Status: reloaderv1alpha1.ReloaderConfigStatus{
						WatchedResourceHashes: make(map[string]string),
					},
				},
			},
			resourceKind:  util.KindSecret,
			resourceName:  "my-secret",
			resourceNS:    "default",
			expectedCount: 0,
		},
		{
			name: "does not find config in different namespace",
			configs: []*reloaderv1alpha1.ReloaderConfig{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "config1",
						Namespace: "other-namespace",
					},
					Spec: reloaderv1alpha1.ReloaderConfigSpec{
						WatchedResources: &reloaderv1alpha1.WatchedResources{
							Secrets: []string{"my-secret"},
						},
					},
					Status: reloaderv1alpha1.ReloaderConfigStatus{
						WatchedResourceHashes: make(map[string]string),
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
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)

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
			name: "finds deployment with auto-reload annotation",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationAuto: "true",
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
									{
										Name:  "test",
										Image: "test",
										Env: []corev1.EnvVar{
											{
												Name: "DB_PASSWORD",
												ValueFrom: &corev1.EnvVarSource{
													SecretKeyRef: &corev1.SecretKeySelector{
														LocalObjectReference: corev1.LocalObjectReference{
															Name: "my-secret",
														},
														Key: "password",
													},
												},
											},
										},
									},
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
			name: "handles multiple secrets in annotation (comma-separated)",
			deployments: []*appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app1",
						Namespace: "default",
						Annotations: map[string]string{
							util.AnnotationSecretReload: "secret1,my-secret,secret3",
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
