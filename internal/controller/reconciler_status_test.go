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

var _ = Describe("Status Management", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When updating ReloaderConfig status", func() {
		ctx := context.Background()

		It("Should track reload count and timestamp", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-test-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "status-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "status-test"},
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

			// Create secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("value1"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"status-secret"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:           util.KindDeployment,
							Name:           "status-test-app",
							ReloadStrategy: "env-vars",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			// Wait for initialization
			time.Sleep(2 * time.Second)

			// Update secret to trigger reload
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "status-secret",
					Namespace: "default",
				}, secret)
				if err != nil {
					return err
				}
				secret.Data["key"] = []byte("value2")
				return k8sClient.Update(ctx, secret)
			}, timeout, interval).Should(Succeed())

			// Verify status was updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "status-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				return config.Status.ReloadCount > 0 &&
					config.Status.LastReloadTime != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("Should track target workload status", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-status-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "target-status"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "target-status"},
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

			// Create secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-status-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("value1"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-status-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"target-status-secret"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:           util.KindDeployment,
							Name:           "target-status-app",
							ReloadStrategy: "env-vars",
						},
					},
					ReloadStrategy: "env-vars",
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			time.Sleep(2 * time.Second)

			// Update secret
			Eventually(func() error {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "target-status-secret",
					Namespace: "default",
				}, secret)
				if err != nil {
					return err
				}
				secret.Data["key"] = []byte("value2")
				return k8sClient.Update(ctx, secret)
			}, timeout, interval).Should(Succeed())

			// Verify target status was updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "target-status-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				if len(config.Status.TargetStatus) == 0 {
					return false
				}

				for _, targetStatus := range config.Status.TargetStatus {
					if targetStatus.Kind == util.KindDeployment &&
						targetStatus.Name == "target-status-app" &&
						targetStatus.ReloadCount > 0 &&
						targetStatus.LastReloadTime != nil {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When handling status update work items", func() {
		ctx := context.Background()

		It("Should process ReloaderConfig status updates from queue", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "queue-test-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "queue-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "queue-test"},
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
					Name:      "queue-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"queue-secret"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind: util.KindDeployment,
							Name: "queue-test-app",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			time.Sleep(2 * time.Second)

			// Manually add a work item to test the queue
			configs := []*reloaderv1alpha1.ReloaderConfig{config}
			reconciler.updateReloaderConfigStatuses(ctx, configs, "default", util.KindSecret, "queue-secret", "test-hash")

			// Verify status was updated eventually
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "queue-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				hashKey := util.MakeResourceKey("default", util.KindSecret, "queue-secret")
				hash, exists := config.Status.WatchedResourceHashes[hashKey]
				return exists && hash == "test-hash"
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When removing status entries for deleted resources", func() {
		ctx := context.Background()

		It("Should remove hash entry when resource is deleted", func() {
			// Create deployment
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remove-status-app",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "remove-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "remove-test"},
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
					Name:      "remove-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"remove-secret"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind: util.KindDeployment,
							Name: "remove-status-app",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			time.Sleep(time.Second)

			// Manually add a hash entry
			configs := []*reloaderv1alpha1.ReloaderConfig{config}
			reconciler.updateReloaderConfigStatuses(ctx, configs, "default", util.KindSecret, "remove-secret", "initial-hash")

			// Wait for hash to be added
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "remove-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return false
				}

				hashKey := util.MakeResourceKey("default", util.KindSecret, "remove-secret")
				_, exists := config.Status.WatchedResourceHashes[hashKey]
				return exists
			}, timeout, interval).Should(BeTrue())

			// Now remove the entry
			reconciler.removeReloaderConfigStatusEntries(ctx, configs, "default", util.KindSecret, "remove-secret")

			// Verify hash was removed
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "remove-config",
					Namespace: "default",
				}, config)
				if err != nil {
					return true // Error is ok, might not exist
				}

				hashKey := util.MakeResourceKey("default", util.KindSecret, "remove-secret")
				_, exists := config.Status.WatchedResourceHashes[hashKey]
				return !exists // Should NOT exist anymore
			}, timeout, interval).Should(BeTrue())
		})
	})
})
