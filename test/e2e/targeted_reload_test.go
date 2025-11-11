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

var _ = Describe("Targeted Reload (CRD)", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("CRD-based Targeted Reload", func() {
		It("should only reload workloads that reference the changed Secret", func() {
			secretA := "secret-a"
			secretB := "secret-b"
			appWithSecretA := "app-with-secret-a"
			appWithSecretB := "app-with-secret-b"
			appWithBothSecrets := "app-with-both-secrets"
			reloaderConfigName := "targeted-reload-config"

			By("creating Secret A")
			secretAYAML := GenerateSecret(secretA, testNS, map[string]string{
				"password": "initial-value-a",
			})
			Expect(utils.ApplyYAML(secretAYAML)).To(Succeed())

			By("creating Secret B")
			secretBYAML := GenerateSecret(secretB, testNS, map[string]string{
				"password": "initial-value-b",
			})
			Expect(utils.ApplyYAML(secretBYAML)).To(Succeed())

			By("creating Deployment that uses Secret A only")
			appADeployment := GenerateDeployment(appWithSecretA, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretA,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(appADeployment)).To(Succeed())

			By("creating Deployment that uses Secret B only")
			appBDeployment := GenerateDeployment(appWithSecretB, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretB,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(appBDeployment)).To(Succeed())

			By("creating Deployment that uses both secrets")
			// For this, we need a custom deployment with both secrets
			appBothDeployment := fmt.Sprintf(`apiVersion: apps/v1
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
        - name: PASSWORD_A
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
        - name: PASSWORD_B
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
`, appWithBothSecrets, testNS, appWithBothSecrets, appWithBothSecrets, secretA, secretB)
			Expect(utils.ApplyYAML(appBothDeployment)).To(Succeed())

			By("waiting for all Deployments to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+appWithSecretA, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+appWithSecretB, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+appWithBothSecrets, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial generations")
			initialGenA, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretA)
			Expect(err).NotTo(HaveOccurred())
			initialGenB, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretB)
			Expect(err).NotTo(HaveOccurred())
			initialGenBoth, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithBothSecrets)
			Expect(err).NotTo(HaveOccurred())

			By("creating ReloaderConfig with targeted reload enabled")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets:       []string{secretA, secretB},
				EnableTargetedReload: true,
				Targets: []Target{
					{
						Kind:         "Deployment",
						Name:         appWithSecretA,
						RequireReference: true,
					},
					{
						Kind:         "Deployment",
						Name:         appWithSecretB,
						RequireReference: true,
					},
					{
						Kind:         "Deployment",
						Name:         appWithBothSecrets,
						RequireReference: true,
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

			By("updating Secret A (should only trigger reload for app-with-secret-a and app-with-both-secrets)")
			updatedSecretA := GenerateSecret(secretA, testNS, map[string]string{
				"password": "updated-value-a",
			})
			Expect(utils.ApplyYAML(updatedSecretA)).To(Succeed())

			By("waiting for ReloaderConfig to process the change")
			time.Sleep(5 * time.Second)

			By("verifying that app-with-secret-a was reloaded")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", appWithSecretA, initialGenA, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying that app-with-both-secrets was reloaded")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", appWithBothSecrets, initialGenBoth, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying that app-with-secret-b was NOT reloaded (generation should not change)")
			currentGenB, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretB)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentGenB).To(Equal(initialGenB), "app-with-secret-b should not have been reloaded")

			By("updating initial generation for Secret B test")
			initialGenB, err = utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretB)
			Expect(err).NotTo(HaveOccurred())
			initialGenA, err = utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretA)
			Expect(err).NotTo(HaveOccurred())
			initialGenBoth, err = utils.GetWorkloadGeneration(testNS, "deployment", appWithBothSecrets)
			Expect(err).NotTo(HaveOccurred())

			By("updating Secret B (should only trigger reload for app-with-secret-b and app-with-both-secrets)")
			updatedSecretB := GenerateSecret(secretB, testNS, map[string]string{
				"password": "updated-value-b",
			})
			Expect(utils.ApplyYAML(updatedSecretB)).To(Succeed())

			By("waiting for ReloaderConfig to process the change")
			time.Sleep(5 * time.Second)

			By("verifying that app-with-secret-b was reloaded")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", appWithSecretB, initialGenB, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying that app-with-both-secrets was reloaded again")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", appWithBothSecrets, initialGenBoth, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying that app-with-secret-a was NOT reloaded (generation should not change)")
			currentGenA, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretA)
			Expect(err).NotTo(HaveOccurred())
			Expect(currentGenA).To(Equal(initialGenA), "app-with-secret-a should not have been reloaded")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {appWithSecretA, appWithSecretB, appWithBothSecrets},
				"secret":         {secretA, secretB},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should reload all workloads when enableSearch is false", func() {
			secretA := "secret-no-search-a"
			secretB := "secret-no-search-b"
			appWithSecretA := "app-no-search-a"
			appWithSecretB := "app-no-search-b"
			reloaderConfigName := "no-search-config"

			By("creating Secret A")
			secretAYAML := GenerateSecret(secretA, testNS, map[string]string{
				"password": "initial-value-a",
			})
			Expect(utils.ApplyYAML(secretAYAML)).To(Succeed())

			By("creating Secret B")
			secretBYAML := GenerateSecret(secretB, testNS, map[string]string{
				"password": "initial-value-b",
			})
			Expect(utils.ApplyYAML(secretBYAML)).To(Succeed())

			By("creating Deployment that uses Secret A")
			appADeployment := GenerateDeployment(appWithSecretA, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretA,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(appADeployment)).To(Succeed())

			By("creating Deployment that uses Secret B")
			appBDeployment := GenerateDeployment(appWithSecretB, testNS, DeploymentOpts{
				Replicas:   1,
				SecretName: secretB,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(appBDeployment)).To(Succeed())

			By("waiting for Deployments to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+appWithSecretA, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+appWithSecretB, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial generations")
			initialGenA, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretA)
			Expect(err).NotTo(HaveOccurred())
			initialGenB, err := utils.GetWorkloadGeneration(testNS, "deployment", appWithSecretB)
			Expect(err).NotTo(HaveOccurred())

			By("creating ReloaderConfig with targeted reload enabled but enableSearch false on targets")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets:       []string{secretA, secretB},
				EnableTargetedReload: true,
				Targets: []Target{
					{
						Kind:         "Deployment",
						Name:         appWithSecretA,
						RequireReference: false, // No search - should reload for ANY watched secret
					},
					{
						Kind:         "Deployment",
						Name:         appWithSecretB,
						RequireReference: false, // No search - should reload for ANY watched secret
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

			By("updating Secret A")
			updatedSecretA := GenerateSecret(secretA, testNS, map[string]string{
				"password": "updated-value-a",
			})
			Expect(utils.ApplyYAML(updatedSecretA)).To(Succeed())

			By("waiting for ReloaderConfig to process the change")
			time.Sleep(5 * time.Second)

			By("verifying that BOTH apps were reloaded (because enableSearch=false)")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", appWithSecretA, initialGenA, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "deployment", appWithSecretB, initialGenB, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {appWithSecretA, appWithSecretB},
				"secret":         {secretA, secretB},
				"reloaderconfig": {reloaderConfigName},
			})
		})
	})
})
