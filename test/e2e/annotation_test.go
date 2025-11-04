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
				"setting": "initial-value",
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
				"setting": "updated-value",
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
	})
})
