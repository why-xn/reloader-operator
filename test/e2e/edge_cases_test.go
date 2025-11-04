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

var _ = Describe("Edge Cases and Error Handling", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("Error Handling", func() {
		It("should handle missing target workload gracefully", func() {
			secretName := "orphan-secret"
			deploymentName := "non-existent-deployment"
			reloaderConfigName := "orphan-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a ReloaderConfig targeting non-existent Deployment")
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

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("verifying status shows no successful reloads")
			status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ReloadCount).To(Equal(int64(0)))

			By("creating the missing Deployment")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("updating the Secret to trigger reload")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("verifying reload now succeeds")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())
		})

		It("should handle missing watched resource gracefully", func() {
			deploymentName := "waiting-app"
			secretName := "future-secret"
			reloaderConfigName := "waiting-config"

			By("creating a Deployment first")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas: 2,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("creating a ReloaderConfig watching non-existent Secret")
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

			By("verifying ReloaderConfig exists and doesn't error")
			Eventually(func() error {
				_, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("creating the missing Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "now-exists",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("verifying ReloaderConfig picks up the new Secret")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())
		})

		It("should handle watched resource deletion", func() {
			secretName := "deletable-secret"
			deploymentName := "resilient-app"
			reloaderConfigName := "resilient-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

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

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("deleting the Secret")
			Expect(utils.DeleteYAML(secretYAML)).To(Succeed())

			By("waiting a moment for reconciliation")
			time.Sleep(10 * time.Second)

			By("recreating the Secret")
			newSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "recreated-value",
			})
			Expect(utils.ApplyYAML(newSecretYAML)).To(Succeed())

			By("verifying ReloaderConfig detects the recreated Secret")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())
		})

		It("should respect pause period between reloads", func() {
			secretName := "pause-secret"
			deploymentName := "pause-app"
			reloaderConfigName := "pause-config"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a Deployment")
			deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
				Replicas:   2,
				SecretName: secretName,
			})
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 2, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("creating a ReloaderConfig with pause period")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{
						Kind:        "Deployment",
						Name:        deploymentName,
						PausePeriod: "1m",
					},
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

			By("triggering first reload")
			updatedSecretYAML1 := GenerateSecret(secretName, testNS, map[string]string{
				"password": "first-update",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML1)).To(Succeed())

			By("waiting for first reload to complete")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount == 1
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("capturing reload count after first reload")
			firstStatus, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			firstReloadCount := firstStatus.ReloadCount

			By("immediately triggering second reload (should be skipped)")
			updatedSecretYAML2 := GenerateSecret(secretName, testNS, map[string]string{
				"password": "second-update-immediate",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML2)).To(Succeed())

			By("waiting a moment for potential second reload")
			time.Sleep(15 * time.Second)

			By("verifying second reload was skipped due to pause period")
			secondStatus, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(secondStatus.ReloadCount).To(Equal(firstReloadCount), "Reload count should not increase during pause period")
		})

		It("should handle multiple ReloaderConfigs watching the same resource", func() {
			secretName := "shared-watch-secret"
			deployment1Name := "multi-config-app-1"
			deployment2Name := "multi-config-app-2"
			config1Name := "config-1"
			config2Name := "config-2"

			By("creating a shared Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating first Deployment")
			deployment1YAML := GenerateDeployment(deployment1Name, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretName,
			})
			Expect(utils.ApplyYAML(deployment1YAML)).To(Succeed())

			By("creating second Deployment")
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

			By("creating first ReloaderConfig")
			config1YAML := GenerateReloaderConfig(config1Name, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{Kind: "Deployment", Name: deployment1Name},
				},
			})
			Expect(utils.ApplyYAML(config1YAML)).To(Succeed())

			By("creating second ReloaderConfig")
			config2YAML := GenerateReloaderConfig(config2Name, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{Kind: "Deployment", Name: deployment2Name},
				},
			})
			Expect(utils.ApplyYAML(config2YAML)).To(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs1, err := utils.GetPodUIDs(testNS, "deployment", deployment1Name)
			Expect(err).NotTo(HaveOccurred())
			initialUIDs2, err := utils.GetPodUIDs(testNS, "deployment", deployment2Name)
			Expect(err).NotTo(HaveOccurred())

			By("updating the shared Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("verifying both ReloaderConfigs trigger reloads")
			Eventually(func() bool {
				status1, err1 := utils.GetReloaderConfigStatus(testNS, config1Name)
				status2, err2 := utils.GetReloaderConfigStatus(testNS, config2Name)
				return err1 == nil && err2 == nil && status1.ReloadCount > 0 && status2.ReloadCount > 0
			}, 1*time.Minute, 2*time.Second).Should(BeTrue())

			By("waiting for both Deployment rollouts to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deployment1Name, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "deployment", deployment2Name, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying both deployments were reloaded")
			newUIDs1, err := utils.GetPodUIDs(testNS, "deployment", deployment1Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs1).NotTo(Equal(initialUIDs1))

			newUIDs2, err := utils.GetPodUIDs(testNS, "deployment", deployment2Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs2).NotTo(Equal(initialUIDs2))
		})
	})
})
