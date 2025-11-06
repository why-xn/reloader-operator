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

var _ = Describe("Volume Mount Scenarios", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("Secret as Volume", func() {
		It("should reload Deployment when Secret mounted as volume changes", func() {
			secretName := "volume-secret"
			deploymentName := "secret-volume-app"
			reloaderConfigName := "secret-volume-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"config.json": `{"version": "1.0"}`,
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment with Secret mounted as volume")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:    2,
				SecretName:  secretName,
				VolumeMount: true,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("creating a ReloaderConfig")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{
						Kind: "Deployment",
						Name: deploymentName,
					},
				},
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(initialUIDs)).To(Equal(2))

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"config.json": `{"version": "2.0"}`,
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to update")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying pods were recreated")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(newUIDs)).To(Equal(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs))
		})
	})

	Context("ConfigMap as Volume", func() {
		It("should reload Deployment when ConfigMap mounted as volume changes", func() {
			configMapName := "volume-configmap"
			deploymentName := "configmap-volume-app"
			reloaderConfigName := "configmap-volume-config"

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"app.conf": "server_name=localhost",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment with ConfigMap mounted as volume")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				ConfigMapName: configMapName,
				VolumeMount:   true,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("creating a ReloaderConfig")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedConfigMaps: []string{configMapName},
				Targets: []Target{
					{
						Kind: "Deployment",
						Name: deploymentName,
					},
				},
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(initialUIDs)).To(Equal(2))

			By("updating the ConfigMap")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"app.conf": "server_name=example.com",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to update")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying pods were recreated")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(newUIDs)).To(Equal(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs))
		})

		It("should reload Deployment when ConfigMap mounted as volume changes with annotation", func() {
			configMapName := "volume-configmap-annotation"
			deploymentName := "configmap-volume-annotation-app"

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"nginx.conf": "worker_processes 1;",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment with auto reload annotation and ConfigMap volume")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				ConfigMapName: configMapName,
				VolumeMount:   true,
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
			Expect(len(initialUIDs)).To(Equal(2))

			By("updating the ConfigMap")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"nginx.conf": "worker_processes 4;",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying pods were recreated")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(newUIDs)).To(Equal(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs))
		})
	})

	Context("Mixed Volume Mounts", func() {
		It("should reload Deployment when either Secret or ConfigMap volume changes", func() {
			secretName := "mixed-secret"
			configMapName := "mixed-configmap"
			deploymentName := "mixed-volume-app"
			reloaderConfigName := "mixed-volume-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "secret123",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"settings": "mode=production",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment with both Secret and ConfigMap volumes")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				SecretName:    secretName,
				ConfigMapName: configMapName,
				VolumeMount:   true,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("creating a ReloaderConfig")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets:    []string{secretName},
				WatchedConfigMaps: []string{configMapName},
				Targets: []Target{
					{
						Kind: "Deployment",
						Name: deploymentName,
					},
				},
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(initialUIDs)).To(Equal(2))

			By("updating the ConfigMap")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"settings": "mode=staging",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for first reload to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying pods were recreated after ConfigMap change")
			secondUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secondUIDs).NotTo(Equal(initialUIDs))

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "newsecret456",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for second reload to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying pods were recreated again after Secret change")
			thirdUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(thirdUIDs).NotTo(Equal(secondUIDs))

			By("verifying ReloaderConfig tracked both reloads")
			status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ReloadCount).To(BeNumerically(">=", 2))
		})
	})
})
