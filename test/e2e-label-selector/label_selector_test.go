//go:build e2e
// +build e2e

package e2e_label_selector

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stakater/Reloader/test/utils"
)

var _ = Describe("Label Selector Feature Tests", func() {
	Context("When --resource-label-selector=app=reloader-test is configured", func() {
		It("should only watch Secrets with matching labels (annotation-based)", func() {
			deploymentName := "test-label-selector-secret"
			matchingSecretName := "matching-secret"
			nonMatchingSecretName := "non-matching-secret"

			By("Creating a Deployment that watches secrets")
			deploymentYAML := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  annotations:
    reloader.stakater.com/auto: "true"
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
      - name: test
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: MATCHING_SECRET
          valueFrom:
            secretKeyRef:
              name: %s
              key: data
              optional: true
        - name: NON_MATCHING_SECRET
          valueFrom:
            secretKeyRef:
              name: %s
              key: data
              optional: true
`, deploymentName, testNamespace, deploymentName, deploymentName, matchingSecretName, nonMatchingSecretName)

			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNamespace, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Creating a Secret WITH matching labels (app=reloader-test)")
			matchingSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    app: reloader-test
    team: backend
type: Opaque
stringData:
  data: "initial-value"
`, matchingSecretName, testNamespace)

			Expect(utils.ApplyYAML(matchingSecretYAML)).To(Succeed())

			By("Creating a Secret WITHOUT matching labels")
			nonMatchingSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    app: other-app
type: Opaque
stringData:
  data: "initial-value"
`, nonMatchingSecretName, testNamespace)

			Expect(utils.ApplyYAML(nonMatchingSecretYAML)).To(Succeed())

			By("Getting pod UIDs before updating matching secret")
			initialPodUIDs, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialPodUIDs).NotTo(BeEmpty())

			By("Updating the Secret WITH matching labels (should trigger reload)")
			matchingSecretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    app: reloader-test
    team: backend
type: Opaque
stringData:
  data: "updated-value"
`, matchingSecretName, testNamespace)

			Expect(utils.ApplyYAML(matchingSecretUpdateYAML)).To(Succeed())

			By("Waiting for deployment to reload after matching secret update")
			Eventually(func() bool {
				newPodUIDs, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
				if err != nil {
					return false
				}
				return len(newPodUIDs) > 0 && !utils.StringSlicesEqual(initialPodUIDs, newPodUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Deployment should reload when matching secret changes")

			By("Getting pod UIDs after successful reload")
			podUIDsAfterMatchingUpdate, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the Secret WITHOUT matching labels (should NOT trigger reload)")
			nonMatchingSecretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
  labels:
    app: other-app
type: Opaque
stringData:
  data: "updated-value"
`, nonMatchingSecretName, testNamespace)

			Expect(utils.ApplyYAML(nonMatchingSecretUpdateYAML)).To(Succeed())

			By("Waiting to ensure no reload happens after non-matching secret update")
			time.Sleep(15 * time.Second)

			By("Verifying pods were NOT restarted")
			finalPodUIDs, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(utils.StringSlicesEqual(podUIDsAfterMatchingUpdate, finalPodUIDs)).To(BeTrue(),
				"Pods should NOT be reloaded when non-matching secret changes")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNamespace, map[string][]string{
				"deployment": {deploymentName},
				"secret":     {matchingSecretName, nonMatchingSecretName},
			})
		})

		It("should only watch ConfigMaps with matching labels (CRD-based)", func() {
			deploymentName := "test-label-selector-cm"
			matchingConfigMapName := "matching-configmap"
			nonMatchingConfigMapName := "non-matching-configmap"
			reloaderConfigName := "test-label-selector-rc"

			By("Creating a ConfigMap WITH matching labels")
			matchingCMYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    app: reloader-test
    team: backend
data:
  config: "initial-value"
`, matchingConfigMapName, testNamespace)

			Expect(utils.ApplyYAML(matchingCMYAML)).To(Succeed())

			By("Creating a ConfigMap WITHOUT matching labels")
			nonMatchingCMYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    app: other-app
data:
  config: "initial-value"
`, nonMatchingConfigMapName, testNamespace)

			Expect(utils.ApplyYAML(nonMatchingCMYAML)).To(Succeed())

			By("Creating a ReloaderConfig watching both ConfigMaps")
			reloaderConfigYAML := fmt.Sprintf(`
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: %s
  namespace: %s
spec:
  watchedResources:
    configMaps:
      - %s
      - %s
  targets:
    - kind: Deployment
      name: %s
  reloadStrategy: env-vars
`, reloaderConfigName, testNamespace, matchingConfigMapName, nonMatchingConfigMapName, deploymentName)

			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("Creating a Deployment")
			deploymentYAML := fmt.Sprintf(`
apiVersion: apps/v1
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
      - name: test
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: CONFIG_MATCHING
          valueFrom:
            configMapKeyRef:
              name: %s
              key: config
              optional: true
        - name: CONFIG_NON_MATCHING
          valueFrom:
            configMapKeyRef:
              name: %s
              key: config
              optional: true
`, deploymentName, testNamespace, deploymentName, deploymentName, matchingConfigMapName, nonMatchingConfigMapName)

			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNamespace, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Getting pod UIDs before updates")
			initialPodUIDs, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialPodUIDs).NotTo(BeEmpty())

			By("Updating the ConfigMap WITH matching labels (should trigger reload)")
			matchingCMUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    app: reloader-test
    team: backend
data:
  config: "updated-value"
`, matchingConfigMapName, testNamespace)

			Expect(utils.ApplyYAML(matchingCMUpdateYAML)).To(Succeed())

			By("Waiting for deployment to reload after matching configmap update")
			Eventually(func() bool {
				newPodUIDs, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
				if err != nil {
					return false
				}
				return len(newPodUIDs) > 0 && !utils.StringSlicesEqual(initialPodUIDs, newPodUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Deployment should reload when matching configmap changes")

			By("Getting pod UIDs after successful reload")
			podUIDsAfterMatchingUpdate, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())

			By("Updating the ConfigMap WITHOUT matching labels (should NOT trigger reload)")
			nonMatchingCMUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
  labels:
    app: other-app
data:
  config: "updated-value-2"
`, nonMatchingConfigMapName, testNamespace)

			Expect(utils.ApplyYAML(nonMatchingCMUpdateYAML)).To(Succeed())

			By("Waiting to ensure no reload happens after non-matching configmap update")
			time.Sleep(15 * time.Second)

			By("Verifying pods were NOT restarted")
			finalPodUIDs, err := utils.GetPodUIDs(testNamespace, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(utils.StringSlicesEqual(podUIDsAfterMatchingUpdate, finalPodUIDs)).To(BeTrue(),
				"Pods should NOT be reloaded when non-matching configmap changes")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNamespace, map[string][]string{
				"deployment":     {deploymentName},
				"configmap":      {matchingConfigMapName, nonMatchingConfigMapName},
				"reloaderconfig": {reloaderConfigName},
			})
		})
	})
})
