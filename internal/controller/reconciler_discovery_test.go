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

	reloaderv1alpha1 "github.com/stakater/Reloader/api/v1alpha1"
	"github.com/stakater/Reloader/internal/pkg/util"
	"github.com/stakater/Reloader/internal/pkg/workload"
)

var _ = Describe("Discovery Functions", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When using ignoreResources configuration", func() {
		It("Should ignore specified resources", func() {
			config := &reloaderv1alpha1.ReloaderConfig{
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					IgnoreResources: []reloaderv1alpha1.ResourceReference{
						{
							Kind:      util.KindSecret,
							Name:      "ignored-secret",
							Namespace: "default",
						},
					},
				},
			}

			// Should ignore exact match
			ignored := reconciler.shouldIgnoreResource(config, util.KindSecret, "ignored-secret", "default")
			Expect(ignored).To(BeTrue())

			// Should not ignore different name
			ignored = reconciler.shouldIgnoreResource(config, util.KindSecret, "other-secret", "default")
			Expect(ignored).To(BeFalse())

			// Should not ignore different kind
			ignored = reconciler.shouldIgnoreResource(config, util.KindConfigMap, "ignored-secret", "default")
			Expect(ignored).To(BeFalse())

			// Should not ignore different namespace
			ignored = reconciler.shouldIgnoreResource(config, util.KindSecret, "ignored-secret", "other-ns")
			Expect(ignored).To(BeFalse())
		})

		It("Should ignore resources across all namespaces when namespace not specified", func() {
			config := &reloaderv1alpha1.ReloaderConfig{
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					IgnoreResources: []reloaderv1alpha1.ResourceReference{
						{
							Kind: util.KindSecret,
							Name: "global-ignored",
							// No namespace specified - should match all namespaces
						},
					},
				},
			}

			// Should ignore in any namespace
			ignored := reconciler.shouldIgnoreResource(config, util.KindSecret, "global-ignored", "default")
			Expect(ignored).To(BeTrue())

			ignored = reconciler.shouldIgnoreResource(config, util.KindSecret, "global-ignored", "kube-system")
			Expect(ignored).To(BeTrue())
		})

		It("Should not ignore when no ignoreResources configured", func() {
			config := &reloaderv1alpha1.ReloaderConfig{
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					IgnoreResources: []reloaderv1alpha1.ResourceReference{},
				},
			}

			ignored := reconciler.shouldIgnoreResource(config, util.KindSecret, "any-secret", "default")
			Expect(ignored).To(BeFalse())
		})
	})

	Context("When checking workload references to resources", func() {
		ctx := context.Background()

		It("Should detect Secret reference in environment variable", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-env-ref",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "env-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "env-test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									Env: []corev1.EnvVar{
										{
											Name: "DB_PASSWORD",
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "db-secret",
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
			}
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			target := workload.Target{
				Kind:      util.KindDeployment,
				Name:      "test-env-ref",
				Namespace: "default",
			}

			// Wait for deployment to be available and verify reference detection
			Eventually(func() bool {
				references, err := reconciler.workloadReferencesResource(ctx, target, util.KindSecret, "db-secret")
				return err == nil && references
			}, timeout, interval).Should(BeTrue())

			// Should not match different secret
			references, err := reconciler.workloadReferencesResource(ctx, target, util.KindSecret, "other-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(references).To(BeFalse())
		})

		It("Should detect ConfigMap reference in envFrom", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-envfrom-ref",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "envfrom-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "envfrom-test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									EnvFrom: []corev1.EnvFromSource{
										{
											ConfigMapRef: &corev1.ConfigMapEnvSource{
												LocalObjectReference: corev1.LocalObjectReference{
													Name: "app-config",
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
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			target := workload.Target{
				Kind:      util.KindDeployment,
				Name:      "test-envfrom-ref",
				Namespace: "default",
			}

			// Wait for deployment to be available and verify reference detection
			Eventually(func() bool {
				references, err := reconciler.workloadReferencesResource(ctx, target, util.KindConfigMap, "app-config")
				return err == nil && references
			}, timeout, interval).Should(BeTrue())
		})

		It("Should detect Secret reference in volume", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-volume-ref",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "volume-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "volume-test"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "secret-volume",
									VolumeSource: corev1.VolumeSource{
										Secret: &corev1.SecretVolumeSource{
											SecretName: "tls-secret",
										},
									},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			target := workload.Target{
				Kind:      util.KindDeployment,
				Name:      "test-volume-ref",
				Namespace: "default",
			}

			// Wait for deployment to be available and verify reference detection
			Eventually(func() bool {
				references, err := reconciler.workloadReferencesResource(ctx, target, util.KindSecret, "tls-secret")
				return err == nil && references
			}, timeout, interval).Should(BeTrue())
		})

		It("Should detect references in init containers", func() {
			statefulSet := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-init-ref",
					Namespace: "default",
				},
				Spec: appsv1.StatefulSetSpec{
					ServiceName: "test-init-service",
					Replicas:    int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "init-test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "init-test"},
						},
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "init",
									Image: "busybox:latest",
									Env: []corev1.EnvVar{
										{
											Name: "INIT_SECRET",
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "init-secret",
													},
													Key: "key",
												},
											},
										},
									},
								},
							},
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, statefulSet)).To(Succeed())
			defer k8sClient.Delete(ctx, statefulSet)

			target := workload.Target{
				Kind:      util.KindStatefulSet,
				Name:      "test-init-ref",
				Namespace: "default",
			}

			// Wait for statefulset to be available and verify reference detection
			Eventually(func() bool {
				references, err := reconciler.workloadReferencesResource(ctx, target, util.KindSecret, "init-secret")
				return err == nil && references
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When checking workload existence", func() {
		ctx := context.Background()

		It("Should detect existing Deployment", func() {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exists-deployment",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "exists"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "exists"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Wait for deployment to be available
			Eventually(func() bool {
				exists, err := reconciler.workloadExists(ctx, util.KindDeployment, "exists-deployment", "default")
				return err == nil && exists
			}, timeout, interval).Should(BeTrue())

			// Verify nonexistent workload returns false
			exists, err := reconciler.workloadExists(ctx, util.KindDeployment, "nonexistent", "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("Should detect existing StatefulSet", func() {
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exists-sts",
					Namespace: "default",
				},
				Spec: appsv1.StatefulSetSpec{
					ServiceName: "exists-sts-service",
					Replicas:    int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "sts-exists"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "sts-exists"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())
			defer k8sClient.Delete(ctx, sts)

			// Wait for resource to propagate
			Eventually(func() bool {
				exists, err := reconciler.workloadExists(ctx, util.KindStatefulSet, "exists-sts", "default")
				return err == nil && exists
			}, timeout, interval).Should(BeTrue())
		})

		It("Should detect existing DaemonSet", func() {
			ds := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "exists-ds",
					Namespace: "default",
				},
				Spec: appsv1.DaemonSetSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "ds-exists"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "ds-exists"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ds)).To(Succeed())
			defer k8sClient.Delete(ctx, ds)

			// Wait for resource to propagate
			Eventually(func() bool {
				exists, err := reconciler.workloadExists(ctx, util.KindDaemonSet, "exists-ds", "default")
				return err == nil && exists
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When using targeted reload", func() {
		ctx := context.Background()

		It("Should filter out targets that don't reference the changed resource", func() {
			// Create deployment that references secret1 but not secret2
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-targeted-reload",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "targeted"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "targeted"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
									Env: []corev1.EnvVar{
										{
											Name: "SECRET1",
											ValueFrom: &corev1.EnvVarSource{
												SecretKeyRef: &corev1.SecretKeySelector{
													LocalObjectReference: corev1.LocalObjectReference{
														Name: "secret1",
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
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Create ReloaderConfig with targeted reload enabled
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "targeted-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets:              []string{"secret1", "secret2"},
						EnableTargetedReload: true,
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind:             util.KindDeployment,
							Name:             "test-targeted-reload",
							RequireReference: true,
						},
					},
				},
			}

			target := workload.Target{
				Kind:             util.KindDeployment,
				Name:             "test-targeted-reload",
				Namespace:        "default",
				RequireReference: true,
				Config:           config,
			}

			targets := []workload.Target{target}

			// Should include when secret1 changes (deployment references it)
			filtered := reconciler.filterTargetsForTargetedReload(ctx, targets, util.KindSecret, "secret1", "default")
			Expect(len(filtered)).To(Equal(1))

			// Should exclude when secret2 changes (deployment doesn't reference it)
			filtered = reconciler.filterTargetsForTargetedReload(ctx, targets, util.KindSecret, "secret2", "default")
			Expect(len(filtered)).To(Equal(0))
		})

		It("Should include all targets when RequireReference is false", func() {
			config := &reloaderv1alpha1.ReloaderConfig{
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						EnableTargetedReload: true,
					},
				},
			}

			target := workload.Target{
				Kind:             util.KindDeployment,
				Name:             "any-deployment",
				Namespace:        "default",
				RequireReference: false, // Should not filter
				Config:           config,
			}

			targets := []workload.Target{target}
			filtered := reconciler.filterTargetsForTargetedReload(ctx, targets, util.KindSecret, "any-secret", "default")
			Expect(len(filtered)).To(Equal(1))
		})

		It("Should include annotation-based targets without filtering", func() {
			target := workload.Target{
				Kind:      util.KindDeployment,
				Name:      "annotated-deployment",
				Namespace: "default",
				Config:    nil, // Annotation-based (no config)
			}

			targets := []workload.Target{target}
			filtered := reconciler.filterTargetsForTargetedReload(ctx, targets, util.KindSecret, "any-secret", "default")
			Expect(len(filtered)).To(Equal(1))
		})
	})

	Context("When merging targets from different sources", func() {
		It("Should combine CRD and annotation-based targets", func() {
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "merge-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind: util.KindDeployment,
							Name: "crd-target",
						},
					},
				},
			}

			annotatedTargets := []workload.Target{
				{
					Kind:      util.KindDeployment,
					Name:      "annotation-target",
					Namespace: "default",
				},
			}

			merged := reconciler.mergeTargets([]*reloaderv1alpha1.ReloaderConfig{config}, annotatedTargets)
			Expect(len(merged)).To(Equal(2))

			// Verify both targets are present
			names := []string{merged[0].Name, merged[1].Name}
			Expect(names).To(ContainElement("crd-target"))
			Expect(names).To(ContainElement("annotation-target"))
		})
	})

	Context("When processing namespace filters", func() {
		ctx := context.Background()

		It("Should process namespace when not in ignored list", func() {
			reconciler.IgnoredNamespaces = map[string]bool{
				"kube-system": true,
			}

			shouldProcess := reconciler.shouldProcessNamespace(ctx, "default")
			Expect(shouldProcess).To(BeTrue())

			shouldProcess = reconciler.shouldProcessNamespace(ctx, "kube-system")
			Expect(shouldProcess).To(BeFalse())
		})

		It("Should process all namespaces when no filters configured", func() {
			reconciler.IgnoredNamespaces = map[string]bool{}
			reconciler.NamespaceSelector = nil

			shouldProcess := reconciler.shouldProcessNamespace(ctx, "default")
			Expect(shouldProcess).To(BeTrue())

			shouldProcess = reconciler.shouldProcessNamespace(ctx, "any-namespace")
			Expect(shouldProcess).To(BeTrue())
		})
	})

	Context("When discovering targets", func() {
		ctx := context.Background()

		It("Should find targets from both ReloaderConfig and annotations", func() {
			// Create deployment with annotation
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "annotated-discovery",
					Namespace: "default",
					Annotations: map[string]string{
						util.AnnotationSecretReload: "discovery-secret",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "discovery"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "discovery"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment)

			// Create another deployment via ReloaderConfig
			deployment2 := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "crd-discovery",
					Namespace: "default",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(1),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "crd"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": "crd"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "app",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deployment2)).To(Succeed())
			defer k8sClient.Delete(ctx, deployment2)

			// Create ReloaderConfig
			config := &reloaderv1alpha1.ReloaderConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "discovery-config",
					Namespace: "default",
				},
				Spec: reloaderv1alpha1.ReloaderConfigSpec{
					WatchedResources: &reloaderv1alpha1.WatchedResources{
						Secrets: []string{"discovery-secret"},
					},
					Targets: []reloaderv1alpha1.TargetWorkload{
						{
							Kind: util.KindDeployment,
							Name: "crd-discovery",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, config)).To(Succeed())
			defer k8sClient.Delete(ctx, config)

			// Create the secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "discovery-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			defer k8sClient.Delete(ctx, secret)

			time.Sleep(2 * time.Second)

			// Discover targets
			targets, configs, err := reconciler.discoverTargets(ctx, util.KindSecret, "discovery-secret", "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(len(configs)).To(Equal(1))
			Expect(len(targets)).To(BeNumerically(">=", 2)) // Should find both targets
		})
	})
})
