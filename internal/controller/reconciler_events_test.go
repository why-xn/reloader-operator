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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/util"
)

var _ = Describe("Event Handlers", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When handling Secret CREATE events", func() {
		ctx := context.Background()

		It("Should track new Secret in ReloaderConfig status", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-secret-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "create-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "create-test"},
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
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Create ReloaderConfig first
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"new-secret"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:           util.KindDeployment,
							Name:           "test-create-secret-app",
							ReloadStrategy: "env-vars",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			time.Sleep(2 * time.Second)

			// Now create the Secret (simulating CREATE event)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("initial-value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Verify hash was added to status
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-create-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				hashKey := util.MakeResourceKey("default", util.KindSecret, "new-secret")
				_, exists := config.Status.WatchedResourceHashes[hashKey]
				return exists
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When handling ConfigMap CREATE events", func() {
		ctx := context.Background()

		It("Should track new ConfigMap in ReloaderConfig status", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-cm-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "cm-create-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "cm-create-test"},
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
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Create ReloaderConfig first
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-cm-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						ConfigMaps: []string{"new-configmap"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:           util.KindDeployment,
							Name:           "test-create-cm-app",
							ReloadStrategy: "env-vars",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			time.Sleep(2 * time.Second)

			// Now create the ConfigMap
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "new-configmap",
					Namespace: "default",
				},
				Data: map[string]string{
					"key": "initial-value",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer k8sClient.Delete(ctx, cm)

			// Verify hash was added to status
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-create-cm-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				hashKey := util.MakeResourceKey("default", util.KindConfigMap, "new-configmap")
				_, exists := config.Status.WatchedResourceHashes[hashKey]
				return exists
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When handling resource hash updates", func() {
		ctx := context.Background()

		It("Should update hash annotation on Secret", func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hash-test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			hash := util.CalculateHash(secret.Data)

			// Wait for secret to be available then update hash
			Eventually(func() error {
				// Fetch latest version to avoid resource version conflicts
				latestSecret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "hash-test-secret",
					Namespace: "default",
				}, latestSecret); err != nil {
					return err
				}
				return reconciler.updateResourceHash(ctx, latestSecret, hash)
			}, timeout, interval).Should(Succeed())

			// Verify annotation was added
			Eventually(func() bool {
				updatedSecret := &corev1.Secret{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "hash-test-secret",
					Namespace: "default",
				}, updatedSecret)
				if err != nil {
					return false
				}
				return updatedSecret.Annotations != nil &&
					updatedSecret.Annotations[util.AnnotationLastHash] == hash
			}, timeout, interval).Should(BeTrue())
		})

		It("Should update hash annotation on ConfigMap", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hash-test-cm",
					Namespace: "default",
				},
				Data: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer k8sClient.Delete(ctx, cm)

			data := util.MergeDataMaps(cm.Data, cm.BinaryData)
			hash := util.CalculateHash(data)

			// Wait for configmap to be available then update hash
			Eventually(func() error {
				// Fetch latest version to avoid resource version conflicts
				latestCM := &corev1.ConfigMap{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "hash-test-cm",
					Namespace: "default",
				}, latestCM); err != nil {
					return err
				}
				return reconciler.updateResourceHash(ctx, latestCM, hash)
			}, timeout, interval).Should(Succeed())

			// Verify annotation was added
			Eventually(func() bool {
				updatedCM := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "hash-test-cm",
					Namespace: "default",
				}, updatedCM)
				if err != nil {
					return false
				}
				return updatedCM.Annotations != nil &&
					updatedCM.Annotations[util.AnnotationLastHash] == hash
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When handling ConfigMap changes", func() {
		ctx := context.Background()

		It("Should trigger deployment reload when ConfigMap data changes", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-cm-reload",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "cm-reload-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "cm-reload-test"},
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
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Create ConfigMap with initial hash
			initialData := map[string]string{"config": "value1"}
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-configmap",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationLastHash: util.CalculateHash(util.MergeDataMaps(initialData, nil)),
					},
				},
				Data: initialData,
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())
			defer k8sClient.Delete(ctx, cm)

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm-reload-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						ConfigMaps: []string{"test-configmap"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:           util.KindDeployment,
							Name:           "test-app-cm-reload",
							ReloadStrategy: "env-vars",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			time.Sleep(2 * time.Second)

			// Update ConfigMap data
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-configmap",
					Namespace: "default",
				}, cm)
				if err != nil {
					return err
				}

				cm.Data["config"] = "value2"
				return k8sClient.Update(ctx, cm)
			}, timeout, interval).Should(Succeed())

			// Verify deployment was reloaded (resource-specific env var added)
			// For ConfigMap "test-configmap", the expected env var is RELOADER_CONFIGMAP_TEST_CONFIGMAP
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-app-cm-reload",
					Namespace: "default",
				}, deployment)
				if err != nil {
					return false
				}

				expectedEnvVar := util.GetEnvVarName(util.KindConfigMap, "test-configmap")
				for _, container := range deployment.Spec.Template.Spec.Containers {
					for _, env := range container.Env {
						if env.Name == expectedEnvVar {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
