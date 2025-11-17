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

var _ = Describe("ReloaderConfig Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When reconciling a ReloaderConfig", func() {
		ctx := context.Background()

		It("Should initialize status and watch resources", func() {
			// Create a secret first
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret-rc1",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"password": []byte("test123"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-rc1",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
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
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config-rc1",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"test-secret-rc1"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind: util.KindDeployment,
							Name: "test-app-rc1",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			// Verify status is initialized
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-config-rc1",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}
				return config.Status.WatchedResourceHashes != nil &&
					len(config.Status.WatchedResourceHashes) > 0
			}, timeout, interval).Should(BeTrue())
		})

		It("Should set Available condition when targets exist", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-rc2",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test2"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "test2"},
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

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config-rc2",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind: util.KindDeployment,
							Name: "test-app-rc2",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			// Verify Available condition
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-config-rc2",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				for _, cond := range config.Status.Conditions {
					if cond.Type == util.ConditionAvailable && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When reconciling a Secret change", func() {
		ctx := context.Background()

		It("Should trigger deployment reload when Secret data changes", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-secret1",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "secret-test1"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "secret-test1"},
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

			// Create secret with initial hash
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret1",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationLastHash: util.CalculateHash(map[string][]byte{
							"password": []byte("old123"),
						}),
					},
				},
				Data: map[string][]byte{
					"password": []byte("old123"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config-secret1",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"test-secret1"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:           util.KindDeployment,
							Name:           "test-app-secret1",
							ReloadStrategy: "env-vars",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			// Wait for config to initialize
			time.Sleep(2 * time.Second)

			// Update secret data
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-secret1",
					Namespace: "default",
				}, secret)
				if err != nil {
					return err
				}

				secret.Data["password"] = []byte("new456")
				return k8sClient.Update(ctx, secret)
			}, timeout, interval).Should(Succeed())

			// Verify deployment was reloaded (resource-specific env var added)
			// For Secret "test-secret1", the expected env var is RELOADER_SECRET_TEST_SECRET1
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-app-secret1",
					Namespace: "default",
				}, deployment)
				if err != nil {
					return false
				}

				expectedEnvVar := util.GetEnvVarName(util.KindSecret, "test-secret1")
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

		It("Should not trigger reload when Secret data unchanged", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app-secret2",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "secret-test2"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "secret-test2"},
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

			// Create secret with hash
			hash := util.CalculateHash(map[string][]byte{
				"key": []byte("value"),
			})

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret2",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationLastHash: hash,
					},
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Update secret metadata only (not data)
			time.Sleep(time.Second)
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "test-secret2",
					Namespace: "default",
				}, secret)
				if err != nil {
					return err
				}

				secret.Labels = map[string]string{"test": "label"}
				return k8sClient.Update(ctx, secret)
			}, timeout, interval).Should(Succeed())

			// Verify deployment was NOT reloaded
			time.Sleep(2 * time.Second)
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-app-secret2",
				Namespace: "default",
			}, deployment)).To(Succeed())

			// No reloader env vars should be present (no reload should have been triggered)
			for _, container := range deployment.Spec.Template.Spec.Containers {
				for _, env := range container.Env {
					Expect(env.Name).NotTo(HavePrefix("RELOADER_"),
						"No RELOADER_* env vars should be present since data didn't change")
				}
			}
		})
	})

	Context("When using annotation-based discovery", func() {
		ctx := context.Background()

		It("Should reload workload with annotation when Secret changes", func() {
			// Create secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotated-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("value1"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Create deployment with annotation
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotated-app",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationSecretReload: "annotated-secret",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "annotated"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "annotated"},
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

			time.Sleep(2 * time.Second)

			// Update secret
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "annotated-secret",
					Namespace: "default",
				}, secret)
				if err != nil {
					return err
				}

				secret.Data["key"] = []byte("value2")
				return k8sClient.Update(ctx, secret)
			}, timeout, interval).Should(Succeed())

			// Verify deployment was reloaded (resource-specific env var added)
			// For Secret "annotated-secret", the expected env var is RELOADER_SECRET_ANNOTATED_SECRET
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "annotated-app",
					Namespace: "default",
				}, deployment)
				if err != nil {
					return false
				}

				expectedEnvVar := util.GetEnvVarName(util.KindSecret, "annotated-secret")
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

// Helper function
func int32Ptr(i int32) *int32 {
	return &i
}
