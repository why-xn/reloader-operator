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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stakater/Reloader/test/utils"
)

var _ = Describe("Ignore Feature", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = utils.SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		utils.CleanupTestNamespace()
	})

	Context("CRD ignoreResources", func() {
		It("should NOT reload when resource is in ignoreResources list", func() {
			secretIgnored := "ignored-secret"
			secretWatched := "watched-secret"
			deploymentName := "test-app"
			reloaderConfigName := "ignore-config"

			By("creating two secrets")
			ignoredSecretYAML := utils.GenerateSecret(secretIgnored, testNS, map[string]string{
				"password": "initial-value-ignored",
			})
			Expect(utils.ApplyYAML(ignoredSecretYAML)).To(Succeed())

			watchedSecretYAML := utils.GenerateSecret(secretWatched, testNS, map[string]string{
				"password": "initial-value-watched",
			})
			Expect(utils.ApplyYAML(watchedSecretYAML)).To(Succeed())

			By("creating a Deployment that uses both secrets")
			deploymentYAML := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
      - name: app
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: PASSWORD_IGNORED
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
        - name: PASSWORD_WATCHED
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
`, deploymentName, testNS, deploymentName, deploymentName, secretIgnored, secretWatched)
			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial generation")
			initialGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())

			By("creating ReloaderConfig that watches both secrets but ignores one")
			reloaderConfigYAML := fmt.Sprintf(`apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: %s
  namespace: %s
spec:
  watchedResources:
    secrets:
      - %s
      - %s
  ignoreResources:
    - kind: Secret
      name: %s
  targets:
    - kind: Deployment
      name: %s
  reloadStrategy: env-vars
`, reloaderConfigName, testNS, secretIgnored, secretWatched, secretIgnored, deploymentName)
			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("waiting for ReloaderConfig status to be initialized")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return len(status.WatchedResourceHashes) > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("updating the ignored secret")
			updatedIgnoredSecret := utils.GenerateSecret(secretIgnored, testNS, map[string]string{
				"password": "updated-value-ignored",
			})
			Expect(utils.ApplyYAML(updatedIgnoredSecret)).To(Succeed())

			By("waiting to ensure no reload happens")
			time.Sleep(10 * time.Second)

			By("verifying generation did NOT change (ignored secret)")
			currentGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentGeneration).To(Equal(initialGeneration), "Generation should not change for ignored secret")

			By("updating the watched secret")
			updatedWatchedSecret := utils.GenerateSecret(secretWatched, testNS, map[string]string{
				"password": "updated-value-watched",
			})
			Expect(utils.ApplyYAML(updatedWatchedSecret)).To(Succeed())

			By("waiting for generation to change (watched secret)")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", deploymentName, initialGeneration, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying reload succeeded for watched secret")
			finalGeneration, err := utils.GetWorkloadGeneration(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalGeneration).To(BeNumerically(">", initialGeneration))

			// Cleanup resources on success
			utils.CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deploymentName},
				"secret":         {secretIgnored, secretWatched},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should ignore resources with namespace-specific rules", func() {
			secretName := "namespace-specific-secret"
			deployment1 := "app-ns1"
			deployment2 := "app-ns2"
			reloaderConfig1 := "config-ns1"
			reloaderConfig2 := "config-ns2"

			By("creating the same secret in both namespaces")
			secret1YAML := utils.GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value-ns1",
			})
			Expect(utils.ApplyYAML(secret1YAML)).To(Succeed())

			By("creating deployments in namespace")
			deployment1YAML := utils.GenerateDeployment(deployment1, testNS, utils.DeploymentOpts{
				Replicas:   1,
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(deployment1YAML)).To(Succeed())

			deployment2YAML := utils.GenerateDeployment(deployment2, testNS, utils.DeploymentOpts{
				Replicas:   1,
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(deployment2YAML)).To(Succeed())

			By("waiting for Deployments to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deployment1, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deployment2, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial generations")
			initialGen1, err := utils.GetWorkloadGeneration(testNS, "deployment", deployment1)
			Expect(err).NotTo(HaveOccurred())
			initialGen2, err := utils.GetWorkloadGeneration(testNS, "deployment", deployment2)
			Expect(err).NotTo(HaveOccurred())

			By("creating ReloaderConfig 1 that ignores secret in test-reloader namespace")
			config1YAML := fmt.Sprintf(`apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: %s
  namespace: %s
spec:
  watchedResources:
    secrets:
      - %s
  ignoreResources:
    - kind: Secret
      name: %s
      namespace: %s
  targets:
    - kind: Deployment
      name: %s
  reloadStrategy: env-vars
`, reloaderConfig1, testNS, secretName, secretName, testNS, deployment1)
			Expect(utils.ApplyYAML(config1YAML)).To(Succeed())

			By("creating ReloaderConfig 2 that does NOT ignore the secret")
			config2YAML := fmt.Sprintf(`apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: %s
  namespace: %s
spec:
  watchedResources:
    secrets:
      - %s
  targets:
    - kind: Deployment
      name: %s
  reloadStrategy: env-vars
`, reloaderConfig2, testNS, secretName, deployment2)
			Expect(utils.ApplyYAML(config2YAML)).To(Succeed())

			By("waiting for ReloaderConfigs to initialize")
			time.Sleep(5 * time.Second)

			By("updating the secret")
			updatedSecretYAML := utils.GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting a moment for reconciliation")
			time.Sleep(10 * time.Second)

			By("verifying deployment1 did NOT reload (ignored)")
			currentGen1, err := utils.GetWorkloadGeneration(testNS, "deployment", deployment1)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentGen1).To(Equal(initialGen1), "deployment1 should not reload (ignored)")

			By("verifying deployment2 DID reload (not ignored)")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", deployment2, initialGen2, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Cleanup resources on success
			utils.CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deployment1, deployment2},
				"secret":         {secretName},
				"reloaderconfig": {reloaderConfig1, reloaderConfig2},
			})
		})
	})
})
