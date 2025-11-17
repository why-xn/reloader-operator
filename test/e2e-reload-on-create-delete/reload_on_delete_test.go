//go:build e2e
// +build e2e

package e2e_reload_on_create_delete

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stakater/Reloader/test/utils"
)

var _ = Describe("ReloadOnDelete Feature Tests", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("When --reload-on-delete is enabled", func() {
		It("should reload deployment when a Secret is deleted (annotation-based)", func() {
			deploymentName := "test-reload-on-delete-secret"
			secretName := "deletable-secret"

			By("Creating a Secret first")
			secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  username: admin
  password: secretpass
`, secretName, testNS)

			Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

			By("Creating a Deployment watching the Secret")
			deploymentYAML := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  annotations:
    secret.reloader.stakater.com/reload: "%s"
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
        - name: SECRET_USERNAME
          valueFrom:
            secretKeyRef:
              name: %s
              key: username
              optional: true
`, deploymentName, testNS, secretName, deploymentName, deploymentName, secretName)

			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Updating the Secret to ensure it's tracked")
			secretUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
stringData:
  username: admin
  password: newsecretpass
`, secretName, testNS)

			Expect(utils.ApplyYAML(secretUpdateYAML)).To(Succeed())
			time.Sleep(5 * time.Second)

			By("waiting for Deployment to be ready after update")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Getting pod UIDs before deletion")
			initialPodUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialPodUIDs).NotTo(BeEmpty(), "Should have at least one pod running")

			By("Deleting the Secret (triggers reload)")
			cmd := exec.Command("kubectl", "delete", "secret", secretName, "-n", testNS)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for pods to be reloaded")
			time.Sleep(5 * time.Second)

			By("Verifying that new pods were created")
			Eventually(func() bool {
				newPodUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
				if err != nil {
					return false
				}
				return len(newPodUIDs) > 0 && !utils.StringSlicesEqual(initialPodUIDs, newPodUIDs)
			}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "Pods should have been reloaded after Secret deletion")

			By("Verifying pods are running (without the Secret)")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Cleanup resources on success
			utils.CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment": {deploymentName},
			})
		})

		It("should reload StatefulSet when a ConfigMap is deleted (CRD-based)", func() {
			statefulSetName := "test-reload-on-delete-cm"
			configMapName := "deletable-configmap"
			reloaderConfigName := "test-delete-rc"

			By("Creating a ConfigMap first")
			configMapYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  config: "initial-value"
`, configMapName, testNS)

			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("Creating a ReloaderConfig watching the ConfigMap")
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
  targets:
    - kind: StatefulSet
      name: %s
  reloadStrategy: env-vars
`, reloaderConfigName, testNS, configMapName, statefulSetName)

			Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

			By("Creating a StatefulSet")
			statefulSetYAML := fmt.Sprintf(`
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: %s
  namespace: %s
spec:
  serviceName: %s
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
        - name: CONFIG_DATA
          valueFrom:
            configMapKeyRef:
              name: %s
              key: config
              optional: true
`, statefulSetName, testNS, statefulSetName, statefulSetName, statefulSetName, configMapName)

			Expect(utils.ApplyYAML(statefulSetYAML)).To(Succeed())

			By("waiting for StatefulSet to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+statefulSetName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Updating the ConfigMap to ensure it's tracked")
			configMapUpdateYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  config: "updated-value"
`, configMapName, testNS)

			Expect(utils.ApplyYAML(configMapUpdateYAML)).To(Succeed())
			time.Sleep(5 * time.Second)

			By("waiting for StatefulSet to be ready after update")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+statefulSetName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Getting pod UIDs before deletion")
			initialPodUIDs, err := utils.GetPodUIDs(testNS, "statefulset", statefulSetName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialPodUIDs).NotTo(BeEmpty(), "Should have at least one pod running")

			By("Deleting the ConfigMap (triggers reload)")
			cmd := exec.Command("kubectl", "delete", "configmap", configMapName, "-n", testNS)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for pods to be reloaded")
			time.Sleep(5 * time.Second)

			By("Verifying that new pods were created")
			Eventually(func() bool {
				newPodUIDs, err := utils.GetPodUIDs(testNS, "statefulset", statefulSetName)
				if err != nil {
					return false
				}
				return len(newPodUIDs) > 0 && !utils.StringSlicesEqual(initialPodUIDs, newPodUIDs)
			}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "Pods should have been reloaded after ConfigMap deletion")

			By("Verifying pods are running (without the ConfigMap)")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+statefulSetName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Cleanup resources on success
			utils.CleanupResourcesOnSuccess(testNS, map[string][]string{
				"statefulset":    {statefulSetName},
				"reloaderconfig": {reloaderConfigName},
			})
		})
	})
})
