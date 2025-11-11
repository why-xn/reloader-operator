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

var _ = Describe("ReloaderConfig", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("CRD-based Configuration", func() {
		It("should reload Deployment when Secret changes using env-vars strategy", func() {
			secretName := "test-secret"
			deploymentName := "test-app"
			reloaderConfigName := "test-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment that uses the Secret")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
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

			By("creating a ReloaderConfig")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{
						Kind: "Deployment",
						Name: deploymentName,
					},
				},
				ReloadStrategy: "env-vars",
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for ReloaderConfig to trigger reload")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for generation to change")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", deploymentName, initialGeneration, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Deployment rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
			}, 2*time.Minute, 10*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")

			By("verifying RELOADER_TRIGGERED_AT env var was added")
			envVars, err := utils.GetPodTemplateEnvVars(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(HaveKey("RELOADER_TRIGGERED_AT"))

			By("verifying resource hash annotation was added")
			annotations, err := utils.GetPodTemplateAnnotations(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(annotations).To(HaveKey("reloader.stakater.com/resource-hash"))

			By("verifying ReloaderConfig status")
			status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ReloadCount).To(Equal(int64(1)))
			Expect(status.LastReloadTime).NotTo(BeNil())
			Expect(status.TargetStatus).To(HaveLen(1))
			Expect(status.TargetStatus[0].Name).To(Equal(deploymentName))
			Expect(status.TargetStatus[0].Kind).To(Equal("Deployment"))

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deploymentName},
				"secret":         {secretName},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should reload Deployment when ConfigMap changes using env-vars strategy", func() {
			configMapName := "test-configmap"
			deploymentName := "test-app-cm"
			reloaderConfigName := "test-config-cm"

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "initial-value",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a Deployment that uses the ConfigMap")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:      2,
				ConfigMapName: configMapName,
				ConfigMapKey:  "config",
				EnvVarName:    "CONFIG_SETTING",
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

			By("creating a ReloaderConfig")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedConfigMaps: []string{configMapName},
				Targets: []Target{
					{
						Kind: "Deployment",
						Name: deploymentName,
					},
				},
				ReloadStrategy: "env-vars",
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("updating the ConfigMap")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for ReloaderConfig to trigger reload")
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

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")

			By("verifying RELOADER_TRIGGERED_AT env var was added")
			envVars, err := utils.GetPodTemplateEnvVars(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(HaveKey("RELOADER_TRIGGERED_AT"))

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deploymentName},
				"configmap":      {configMapName},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should reload StatefulSet when ConfigMap changes using annotations strategy", func() {
			configMapName := "sts-configmap"
			statefulSetName := "test-sts"
			reloaderConfigName := "test-config-sts"

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "initial-value",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a StatefulSet that uses the ConfigMap")
			statefulSetYAML := GenerateStatefulSet(statefulSetName, testNS, StatefulSetOpts{
				Replicas:      2,
				ConfigMapName: configMapName,
				ConfigMapKey:  "config",
				EnvVarName:    "CONFIG_SETTING",
			})
			Expect(utils.ApplyYAML(statefulSetYAML)).To(Succeed())

			By("waiting for StatefulSet to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+statefulSetName, 2, 30*time.Second)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(2))

			By("creating a ReloaderConfig with annotations strategy")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedConfigMaps: []string{configMapName},
				Targets: []Target{
					{
						Kind: "StatefulSet",
						Name: statefulSetName,
					},
				},
				ReloadStrategy: "annotations",
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("updating the ConfigMap")
			updatedConfigMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedConfigMapYAML)).To(Succeed())

			By("waiting for ReloaderConfig to trigger reload")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for StatefulSet rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "statefulset", statefulSetName, 60*time.Second)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")

			By("verifying pod template annotations were updated (not env vars)")
			annotations, err := utils.GetPodTemplateAnnotations(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(annotations).To(HaveKey("reloader.stakater.com/last-reload"))
			Expect(annotations).To(HaveKey("reloader.stakater.com/resource-hash"))

			By("verifying RELOADER_TRIGGERED_AT env var was NOT added (annotations strategy)")
			envVars, err := utils.GetPodTemplateEnvVars(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).NotTo(HaveKey("RELOADER_TRIGGERED_AT"))

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"statefulset":    {statefulSetName},
				"configmap":      {configMapName},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should reload multiple workloads when shared Secret changes", func() {
			secretName := "shared-secret"
			deployment1Name := "app-1"
			deployment2Name := "app-2"
			reloaderConfigName := "test-config-multi"

			By("creating a shared Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "shared-initial",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating first Deployment using the Secret")
			deployment1YAML := GenerateDeployment(deployment1Name, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretName,
			})
			Expect(utils.ApplyYAML(deployment1YAML)).To(Succeed())

			By("creating second Deployment using the Secret")
			deployment2YAML := GenerateDeployment(deployment2Name, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretName,
			})
			Expect(utils.ApplyYAML(deployment2YAML)).To(Succeed())

			By("waiting for both Deployments to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deployment1Name, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deployment2Name, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs for both deployments")
			initialUIDs1, err := utils.GetPodUIDs(testNS, "deployment", deployment1Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs1).To(HaveLen(1))

			initialUIDs2, err := utils.GetPodUIDs(testNS, "deployment", deployment2Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs2).To(HaveLen(1))

			By("creating a ReloaderConfig targeting both deployments")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{Kind: "Deployment", Name: deployment1Name},
					{Kind: "Deployment", Name: deployment2Name},
				},
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("updating the shared Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "shared-updated",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for ReloaderConfig to trigger reload")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0 && len(status.TargetStatus) == 2
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for both Deployment rollouts to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deployment1Name, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deployment2Name, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created for both deployments")
			newUIDs1, err := utils.GetPodUIDs(testNS, "deployment", deployment1Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs1).NotTo(Equal(initialUIDs1))

			newUIDs2, err := utils.GetPodUIDs(testNS, "deployment", deployment2Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs2).NotTo(Equal(initialUIDs2))

			By("verifying ReloaderConfig status shows both targets reloaded")
			status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.TargetStatus).To(HaveLen(2))

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deployment1Name, deployment2Name},
				"secret":         {secretName},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should reload Deployment using restart strategy without modifying template", func() {
			secretName := "restart-strategy-secret"
			deploymentName := "restart-strategy-deploy"
			reloaderConfigName := "restart-strategy-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment that uses the Secret")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "DB_PASSWORD",
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

			By("capturing initial deployment generation")
			initialGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())

			By("creating a ReloaderConfig with restart strategy")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{
						Kind: "Deployment",
						Name: deploymentName,
					},
				},
				ReloadStrategy: "restart",
			})
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for ReloaderConfig to trigger reload")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for pods to be recreated")
			Eventually(func() bool {
				currentUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
				if err != nil {
					return false
				}
				// Check if UIDs changed (pods were recreated)
				return len(currentUIDs) == 2 && !utils.StringSlicesEqual(currentUIDs, initialUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).To(HaveLen(2))
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after restart")

			By("verifying deployment template was NOT modified (restart strategy)")
			currentGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentGeneration).To(Equal(initialGeneration), "Deployment generation should not change with restart strategy")

			By("verifying no env var was added to pod template")
			envVars, err := utils.GetPodTemplateEnvVars(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).NotTo(HaveKey("RELOADER_TRIGGERED_AT"), "RELOADER_TRIGGERED_AT should not be added with restart strategy")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deploymentName},
				"secret":         {secretName},
				"reloaderconfig": {reloaderConfigName},
			})
		})
	})
})
