//go:build e2e
// +build e2e

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

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stakater/Reloader/test/utils"
)

var _ = Describe("Annotation-based Configuration", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("Legacy Annotation Support", func() {
		It("should reload Deployment with secret.reloader.stakater.com/reload annotation", func() {
			secretName := "annotated-secret"
			deploymentName := "annotated-app"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment with secret reload annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
				Annotations: map[string]string{
					"secret.reloader.stakater.com/reload": secretName,
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")
		})

		It("should reload Deployment with configmap.reloader.stakater.com/reload annotation", func() {
			configMapName := "annotated-configmap"
			deploymentName := "annotated-app-cm"

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "initial-value",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment with configmap reload annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				ConfigMapName: configMapName,
				Annotations: map[string]string{
					"configmap.reloader.stakater.com/reload": configMapName,
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the ConfigMap")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")
		})

		It("should auto-reload when workload has reloader.stakater.com/auto annotation", func() {
			secretName := "auto-secret"
			deploymentName := "auto-app"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment with auto annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
				Annotations: map[string]string{
					"reloader.stakater.com/auto": "true",
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after auto-reload")
		})

		It("should NOT reload when reloader.stakater.com/ignore annotation is true", func() {
			secretName := "ignored-secret"
			deploymentName := "ignored-app"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment with ignore annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
				Annotations: map[string]string{
					"reloader.stakater.com/ignore": "true",
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("capturing initial generation")
			initialGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting a reasonable time for potential reload")
			time.Sleep(15 * time.Second)

			By("verifying generation did NOT change")
			currentGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentGeneration).To(Equal(initialGeneration), "Generation should not change for ignored workload")

			By("verifying pods were NOT restarted")
			currentUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentUIDs).To(Equal(initialUIDs), "Pod UIDs should remain the same for ignored workload")
		})

		It("should support multiple secrets in comma-separated annotation", func() {
			secret1Name := "multi-secret-1"
			secret2Name := "multi-secret-2"
			deploymentName := "multi-secret-app"

			By("creating first Secret")
			secret1YAML := GenerateSecret(secret1Name, testNS, map[string]string{
				"password": "secret1-initial",
			})
			Expect(utils.ApplyYAML(secret1YAML)).To(Succeed())

			By("creating second Secret")
			secret2YAML := GenerateSecret(secret2Name, testNS, map[string]string{
				"token": "secret2-initial",
			})
			Expect(utils.ApplyYAML(secret2YAML)).To(Succeed())

			By("creating a Deployment with multiple secrets in annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secret1Name,
				Annotations: map[string]string{
					"secret.reloader.stakater.com/reload": secret1Name + "," + secret2Name,
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the second Secret (not the one in env)")
			updatedSecret2YAML := GenerateSecret(secret2Name, testNS, map[string]string{
				"token": "secret2-updated",
			})
			Expect(utils.ApplyYAML(updatedSecret2YAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")
		})

		It("should auto-reload only Secrets with secret.reloader.stakater.com/auto annotation", func() {
			secretName := "test-secret-only"
			configMapName := "test-config-ignored"
			deploymentName := "test-secret-auto"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-secret",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "initial-config",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment with secret-only auto annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				SecretName:    secretName,
				ConfigMapName: configMapName,
				Annotations: map[string]string{
					"secret.reloader.stakater.com/auto": "true",
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the ConfigMap (should NOT trigger reload)")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "updated-config",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting a reasonable time")
			time.Sleep(15 * time.Second)

			By("verifying pods were NOT restarted after ConfigMap update")
			currentUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentUIDs).To(Equal(initialUIDs), "Pod UIDs should remain the same when ConfigMap changes")

			By("updating the Secret (should trigger reload)")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-secret",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created after Secret update")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after Secret update")
		})

		It("should auto-reload only ConfigMaps with configmap.reloader.stakater.com/auto annotation", func() {
			secretName := "test-secret-ignored"
			configMapName := "test-config-only"
			deploymentName := "test-configmap-auto"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-secret",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "initial-config",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment with configmap-only auto annotation")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				SecretName:    secretName,
				ConfigMapName: configMapName,
				Annotations: map[string]string{
					"configmap.reloader.stakater.com/auto": "true",
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the Secret (should NOT trigger reload)")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-secret",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting a reasonable time")
			time.Sleep(15 * time.Second)

			By("verifying pods were NOT restarted after Secret update")
			currentUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentUIDs).To(Equal(initialUIDs), "Pod UIDs should remain the same when Secret changes")

			By("updating the ConfigMap (should trigger reload)")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "updated-config",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created after ConfigMap update")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after ConfigMap update")
		})

		It("should reload only when named Secret in list changes", func() {
			namedSecretName := "test-named-secret"
			otherSecretName := "test-other-secret"
			deploymentName := "test-named-secret"

			By("creating named Secret")
			namedSecretYAML := GenerateSecret(namedSecretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(namedSecretYAML)).To(Succeed())

			By("creating other Secret")
			otherSecretYAML := GenerateSecret(otherSecretName, testNS, map[string]string{
				"password": "other-value",
			})
			Expect(utils.ApplyYAML(otherSecretYAML)).To(Succeed())

			By("creating a Deployment that references both Secrets but only watches one")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: namedSecretName,
				Annotations: map[string]string{
					"secret.reloader.stakater.com/reload": namedSecretName,
				},
				AdditionalEnv: []map[string]string{
					{
						"name":      "OTHER_SECRET",
						"valueFrom": otherSecretName,
						"key":       "password",
					},
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the other Secret (not in reload list)")
			updatedOtherSecretYAML := GenerateSecret(otherSecretName, testNS, map[string]string{
				"password": "updated-other",
			})
			Expect(utils.ApplyYAML(updatedOtherSecretYAML)).To(Succeed())

			By("waiting a reasonable time")
			time.Sleep(15 * time.Second)

			By("verifying pods were NOT restarted")
			currentUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentUIDs).To(Equal(initialUIDs), "Pod UIDs should remain the same when non-watched Secret changes")

			By("updating the named Secret (in reload list)")
			updatedNamedSecretYAML := GenerateSecret(namedSecretName, testNS, map[string]string{
				"password": "updated-named",
			})
			Expect(utils.ApplyYAML(updatedNamedSecretYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after watched Secret update")
		})

		It("should reload when named ConfigMap in list changes", func() {
			namedConfigMapName := "test-named-config"
			otherConfigMapName := "test-other-config"
			deploymentName := "test-named-config"

			By("creating named ConfigMap")
			namedConfigMapYAML := GenerateConfigMap(namedConfigMapName, testNS, map[string]string{
				"config": "initial-value",
			})
			Expect(utils.ApplyYAML(namedConfigMapYAML)).To(Succeed())

			By("creating other ConfigMap")
			otherConfigMapYAML := GenerateConfigMap(otherConfigMapName, testNS, map[string]string{
				"config": "other-value",
			})
			Expect(utils.ApplyYAML(otherConfigMapYAML)).To(Succeed())

			By("creating a Deployment that references both ConfigMaps but only watches one")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				ConfigMapName: namedConfigMapName,
				Annotations: map[string]string{
					"configmap.reloader.stakater.com/reload": namedConfigMapName,
				},
				AdditionalEnv: []map[string]string{
					{
						"name":      "OTHER_CONFIG",
						"valueFrom": otherConfigMapName,
						//"valueFromRef": "configMapKeyRef",
						"key": "config",
					},
				},
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the other ConfigMap (not in reload list)")
			updatedOtherConfigMapYAML := GenerateConfigMap(otherConfigMapName, testNS, map[string]string{
				"config": "updated-other",
			})
			Expect(utils.ApplyYAML(updatedOtherConfigMapYAML)).To(Succeed())

			By("waiting a reasonable time")
			time.Sleep(15 * time.Second)

			By("verifying pods were NOT restarted")
			currentUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentUIDs).To(Equal(initialUIDs), "Pod UIDs should remain the same when non-watched ConfigMap changes")

			By("updating the named ConfigMap (in reload list)")
			updatedNamedConfigMapYAML := GenerateConfigMap(namedConfigMapName, testNS, map[string]string{
				"config": "updated-named",
			})
			Expect(utils.ApplyYAML(updatedNamedConfigMapYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after watched ConfigMap update")
		})

		It("should reload when named Secret changes even if NOT referenced in pod spec", func() {
			externalSecretName := "external-secret"
			deploymentName := "test-external-secret"

			By("creating an external Secret not referenced in pod spec")
			externalSecretYAML := GenerateSecret(externalSecretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(externalSecretYAML)).To(Succeed())

			By("creating a Deployment that watches external Secret but doesn't reference it")
			// This uses GenerateDeploymentWithNoResources helper
			deploymentYAML := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ` + deploymentName + `
  namespace: ` + testNS + `
  annotations:
    secret.reloader.stakater.com/reload: "` + externalSecretName + `"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ` + deploymentName + `
  template:
    metadata:
      labels:
        app: ` + deploymentName + `
    spec:
      containers:
      - name: app
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: DUMMY
          value: "no-secret-here"
`
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("updating the external Secret (not referenced in pod spec)")
			updatedExternalSecretYAML := GenerateSecret(externalSecretName, testNS, map[string]string{
				"password": "updated-external",
			})
			Expect(utils.ApplyYAML(updatedExternalSecretYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created despite Secret not being referenced")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different even for non-referenced Secret")
		})
	})
})
