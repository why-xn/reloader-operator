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

var _ = Describe("Workload Types", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("StatefulSet", func() {
		It("should reload StatefulSet when Secret changes using env-vars strategy", func() {
			secretName := "sts-secret"
			statefulSetName := "test-sts-envvars"
			reloaderConfigName := "sts-config-envvars"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a StatefulSet that uses the Secret")
			statefulSetYAML := GenerateStatefulSet(statefulSetName, testNS, StatefulSetOpts{
				Replicas:   2,
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(statefulSetYAML)).To(Succeed())

			By("waiting for StatefulSet to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+statefulSetName, 2, 30*time.Second)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial generation")
			initialGeneration, err := utils.GetWorkloadGeneration(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())

			By("creating a ReloaderConfig with env-vars strategy")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{
						Kind: "StatefulSet",
						Name: statefulSetName,
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

			By("waiting for generation to change")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "statefulset", statefulSetName, initialGeneration, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying RELOADER_TRIGGERED_AT env var was added")
			envVars, err := utils.GetPodTemplateEnvVars(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(HaveKey("RELOADER_TRIGGERED_AT"))

			By("verifying ReloaderConfig status")
			status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ReloadCount).To(Equal(int64(1)))

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"statefulset":    {statefulSetName},
				"secret":         {secretName},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should respect pause period for StatefulSet", func() {
			secretName := "sts-pause-secret"
			statefulSetName := "test-sts-pause"
			reloaderConfigName := "sts-config-pause"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a StatefulSet")
			statefulSetYAML := GenerateStatefulSet(statefulSetName, testNS, StatefulSetOpts{
				Replicas:   1,
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
			})
			Expect(utils.ApplyYAML(statefulSetYAML)).To(Succeed())

			By("waiting for StatefulSet to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+statefulSetName, 1, 30*time.Second)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("creating ReloaderConfig with pause period")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedSecrets: []string{secretName},
				Targets: []Target{
					{
						Kind:        "StatefulSet",
						Name:        statefulSetName,
						PausePeriod: "1m",
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

			By("updating Secret first time")
			updatedSecret1 := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value-1",
			})
			Expect(utils.ApplyYAML(updatedSecret1)).To(Succeed())

			By("waiting for first reload")
			Eventually(func() bool {
				status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
				if err != nil {
					return false
				}
				return status.ReloadCount > 0
			}, 30*time.Second, 1*time.Second).Should(BeTrue())

			By("capturing generation after first reload")
			generation1, err := utils.GetWorkloadGeneration(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())

			By("updating Secret second time (within pause period)")
			time.Sleep(2 * time.Second)
			updatedSecret2 := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value-2",
			})
			Expect(utils.ApplyYAML(updatedSecret2)).To(Succeed())

			By("waiting to ensure no reload happens")
			time.Sleep(10 * time.Second)

			By("verifying generation did not change (paused)")
			generation2, err := utils.GetWorkloadGeneration(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(generation2).To(Equal(generation1), "Generation should not change during pause period")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"statefulset":    {statefulSetName},
				"secret":         {secretName},
				"reloaderconfig": {reloaderConfigName},
			})
		})
	})

	Context("DaemonSet", func() {
		It("should reload DaemonSet when ConfigMap changes using env-vars strategy", func() {
			configMapName := "ds-configmap"
			daemonSetName := "test-ds-envvars"
			reloaderConfigName := "ds-config-envvars"

			By("creating a ConfigMap")
			configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
				"config": "initial-value",
			})
			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("creating a DaemonSet that uses the ConfigMap")
			daemonSetYAML := GenerateDaemonSet(daemonSetName, testNS, DaemonSetOpts{
				ConfigMapName: configMapName,
				ConfigMapKey:  "config",
				EnvVarName:    "CONFIG",
			})
			Expect(utils.ApplyYAML(daemonSetYAML)).To(Succeed())

			By("waiting for DaemonSet to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+daemonSetName, 1, 30*time.Second)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial generation")
			initialGeneration, err := utils.GetWorkloadGeneration(testNS, "daemonset", daemonSetName)
			Expect(err).NotTo(HaveOccurred())

			By("creating a ReloaderConfig with env-vars strategy")
			reloaderConfigYAML := GenerateReloaderConfig(reloaderConfigName, testNS, ReloaderConfigSpec{
				WatchedConfigMaps: []string{configMapName},
				Targets: []Target{
					{
						Kind: "DaemonSet",
						Name: daemonSetName,
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

			By("waiting for generation to change")
			Eventually(func() error {
				return utils.WaitForGenerationChange(testNS, "daemonset", daemonSetName, initialGeneration, 10*time.Second)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying RELOADER_TRIGGERED_AT env var was added")
			envVars, err := utils.GetPodTemplateEnvVars(testNS, "daemonset", daemonSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(HaveKey("RELOADER_TRIGGERED_AT"))

			By("verifying ReloaderConfig status")
			status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ReloadCount).To(Equal(int64(1)))

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"daemonset":      {daemonSetName},
				"configmap":      {configMapName},
				"reloaderconfig": {reloaderConfigName},
			})
		})

		It("should reload DaemonSet with annotation-based configuration", func() {
			secretName := "ds-annotation-secret"
			daemonSetName := "test-ds-annotation"

			By("creating a Secret")
			secretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "initial-value",
			})
			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("creating a DaemonSet with auto annotation")
			daemonSetYAML := GenerateDaemonSet(daemonSetName, testNS, DaemonSetOpts{
				SecretName: secretName,
				SecretKey:  "password",
				EnvVarName: "PASSWORD",
				Annotations: map[string]string{
					"reloader.stakater.com/auto": "true",
				},
			})
			Expect(utils.ApplyYAML(daemonSetYAML)).To(Succeed())

			By("waiting for DaemonSet to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+daemonSetName, 1, 30*time.Second)
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("capturing initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "daemonset", daemonSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(initialUIDs)).To(BeNumerically(">=", 1))

			By("updating the Secret")
			updatedSecretYAML := GenerateSecret(secretName, testNS, map[string]string{
				"password": "updated-value",
			})
			Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

			By("waiting for DaemonSet rollout to complete")
			Eventually(func() error {
				return utils.WaitForRolloutComplete(testNS, "daemonset", daemonSetName, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying new pods were created")
			newUIDs, err := utils.GetPodUIDs(testNS, "daemonset", daemonSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"daemonset": {daemonSetName},
				"secret":    {secretName},
			})
		})
	})
})
