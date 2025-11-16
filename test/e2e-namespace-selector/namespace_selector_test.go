//go:build e2e
// +build e2e

package e2e_namespace_selector

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stakater/Reloader/test/utils"
)

var _ = Describe("Namespace Selector Feature Tests", func() {
	Context("When --namespace-selector=environment=production is configured", func() {
		It("should only watch resources in namespaces with matching labels", func() {
			deploymentNameMatching := "test-ns-selector-matching"
			deploymentNameNonMatching := "test-ns-selector-non-matching"
			secretName := "test-secret"

			By("Creating a Deployment in the MATCHING namespace")
			deploymentMatchingYAML := fmt.Sprintf(`
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
        - name: SECRET_DATA
          valueFrom:
            secretKeyRef:
              name: %s
              key: data
              optional: true
`, deploymentNameMatching, matchingNamespace, deploymentNameMatching, deploymentNameMatching, secretName)

			Expect(utils.ApplyYAML(deploymentMatchingYAML)).To(Succeed())

			By("Creating a Deployment in the NON-MATCHING namespace")
			deploymentNonMatchingYAML := fmt.Sprintf(`
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
        - name: SECRET_DATA
          valueFrom:
            secretKeyRef:
              name: %s
              key: data
              optional: true
`, deploymentNameNonMatching, nonMatchingNamespace, deploymentNameNonMatching, deploymentNameNonMatching, secretName)

			Expect(utils.ApplyYAML(deploymentNonMatchingYAML)).To(Succeed())

			By("waiting for Deployments to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(matchingNamespace, "app="+deploymentNameMatching, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			Eventually(func() error {
				return utils.WaitForPodsReady(nonMatchingNamespace, "app="+deploymentNameNonMatching, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Creating a Secret in the MATCHING namespace")
			matchingSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "initial-value"
`, secretName, matchingNamespace)

			Expect(utils.ApplyYAML(matchingSecretYAML)).To(Succeed())

			By("Creating a Secret in the NON-MATCHING namespace")
			nonMatchingSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "initial-value"
`, secretName, nonMatchingNamespace)

			Expect(utils.ApplyYAML(nonMatchingSecretYAML)).To(Succeed())

			By("Getting pod UIDs in MATCHING namespace before update")
			matchingInitialPodUIDs, err := utils.GetPodUIDs(matchingNamespace, "deployment", deploymentNameMatching)
			Expect(err).NotTo(HaveOccurred())
			Expect(matchingInitialPodUIDs).NotTo(BeEmpty())

			By("Getting pod UIDs in NON-MATCHING namespace before update")
			nonMatchingInitialPodUIDs, err := utils.GetPodUIDs(nonMatchingNamespace, "deployment", deploymentNameNonMatching)
			Expect(err).NotTo(HaveOccurred())
			Expect(nonMatchingInitialPodUIDs).NotTo(BeEmpty())

			By("Updating the Secret in MATCHING namespace (should trigger reload)")
			matchingSecretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "updated-value"
`, secretName, matchingNamespace)

			Expect(utils.ApplyYAML(matchingSecretUpdateYAML)).To(Succeed())

			By("Waiting for deployment in MATCHING namespace to reload")
			Eventually(func() bool {
				newPodUIDs, err := utils.GetPodUIDs(matchingNamespace, "deployment", deploymentNameMatching)
				if err != nil {
					return false
				}
				return len(newPodUIDs) > 0 && !utils.StringSlicesEqual(matchingInitialPodUIDs, newPodUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Deployment in matching namespace should reload")

			By("Updating the Secret in NON-MATCHING namespace (should NOT trigger reload)")
			nonMatchingSecretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "updated-value"
`, secretName, nonMatchingNamespace)

			Expect(utils.ApplyYAML(nonMatchingSecretUpdateYAML)).To(Succeed())

			By("Waiting to ensure no reload happens in NON-MATCHING namespace")
			time.Sleep(15 * time.Second)

			By("Verifying pods in NON-MATCHING namespace were NOT restarted")
			finalNonMatchingPodUIDs, err := utils.GetPodUIDs(nonMatchingNamespace, "deployment", deploymentNameNonMatching)
			Expect(err).NotTo(HaveOccurred())
			Expect(utils.StringSlicesEqual(nonMatchingInitialPodUIDs, finalNonMatchingPodUIDs)).To(BeTrue(),
				"Pods in non-matching namespace should NOT be reloaded")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(matchingNamespace, map[string][]string{
				"deployment": {deploymentNameMatching},
				"secret":     {secretName},
			})
			CleanupResourcesOnSuccess(nonMatchingNamespace, map[string][]string{
				"deployment": {deploymentNameNonMatching},
				"secret":     {secretName},
			})
		})
	})

	Context("When --namespaces-to-ignore=test-ns-ignored is configured", func() {
		It("should not watch resources in ignored namespaces", func() {
			deploymentNameIgnored := "test-ns-ignore-deployment"
			deploymentNameWatched := "test-ns-watched-deployment"
			secretName := "test-secret"

			By("Creating a Deployment in the IGNORED namespace")
			deploymentIgnoredYAML := fmt.Sprintf(`
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
        - name: SECRET_DATA
          valueFrom:
            secretKeyRef:
              name: %s
              key: data
              optional: true
`, deploymentNameIgnored, ignoredNamespace, deploymentNameIgnored, deploymentNameIgnored, secretName)

			Expect(utils.ApplyYAML(deploymentIgnoredYAML)).To(Succeed())

			By("Creating a Deployment in a WATCHED namespace")
			deploymentWatchedYAML := fmt.Sprintf(`
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
        - name: SECRET_DATA
          valueFrom:
            secretKeyRef:
              name: %s
              key: data
              optional: true
`, deploymentNameWatched, matchingNamespace, deploymentNameWatched, deploymentNameWatched, secretName)

			Expect(utils.ApplyYAML(deploymentWatchedYAML)).To(Succeed())

			By("waiting for Deployments to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(ignoredNamespace, "app="+deploymentNameIgnored, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			Eventually(func() error {
				return utils.WaitForPodsReady(matchingNamespace, "app="+deploymentNameWatched, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Creating a Secret in the IGNORED namespace")
			ignoredSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "initial-value"
`, secretName, ignoredNamespace)

			Expect(utils.ApplyYAML(ignoredSecretYAML)).To(Succeed())

			By("Creating a Secret in the WATCHED namespace")
			watchedSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "initial-value"
`, secretName, matchingNamespace)

			Expect(utils.ApplyYAML(watchedSecretYAML)).To(Succeed())

			By("Getting pod UIDs in IGNORED namespace before update")
			ignoredInitialPodUIDs, err := utils.GetPodUIDs(ignoredNamespace, "deployment", deploymentNameIgnored)
			Expect(err).NotTo(HaveOccurred())
			Expect(ignoredInitialPodUIDs).NotTo(BeEmpty())

			By("Getting pod UIDs in WATCHED namespace before update")
			watchedInitialPodUIDs, err := utils.GetPodUIDs(matchingNamespace, "deployment", deploymentNameWatched)
			Expect(err).NotTo(HaveOccurred())
			Expect(watchedInitialPodUIDs).NotTo(BeEmpty())

			By("Updating the Secret in IGNORED namespace (should NOT trigger reload)")
			ignoredSecretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "updated-value"
`, secretName, ignoredNamespace)

			Expect(utils.ApplyYAML(ignoredSecretUpdateYAML)).To(Succeed())

			By("Waiting to ensure no reload happens in IGNORED namespace")
			time.Sleep(15 * time.Second)

			By("Verifying pods in IGNORED namespace were NOT restarted")
			finalIgnoredPodUIDs, err := utils.GetPodUIDs(ignoredNamespace, "deployment", deploymentNameIgnored)
			Expect(err).NotTo(HaveOccurred())
			Expect(utils.StringSlicesEqual(ignoredInitialPodUIDs, finalIgnoredPodUIDs)).To(BeTrue(),
				"Pods in ignored namespace should NOT be reloaded")

			By("Updating the Secret in WATCHED namespace (should trigger reload)")
			watchedSecretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  data: "updated-value"
`, secretName, matchingNamespace)

			Expect(utils.ApplyYAML(watchedSecretUpdateYAML)).To(Succeed())

			By("Waiting for deployment in WATCHED namespace to reload")
			Eventually(func() bool {
				newPodUIDs, err := utils.GetPodUIDs(matchingNamespace, "deployment", deploymentNameWatched)
				if err != nil {
					return false
				}
				return len(newPodUIDs) > 0 && !utils.StringSlicesEqual(watchedInitialPodUIDs, newPodUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Deployment in watched namespace should reload")

			// Cleanup resources on success
			CleanupResourcesOnSuccess(ignoredNamespace, map[string][]string{
				"deployment": {deploymentNameIgnored},
				"secret":     {secretName},
			})
			CleanupResourcesOnSuccess(matchingNamespace, map[string][]string{
				"deployment": {deploymentNameWatched},
				"secret":     {secretName},
			})
		})
	})
})
