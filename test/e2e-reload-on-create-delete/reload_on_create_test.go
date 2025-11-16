//go:build e2e
// +build e2e

package e2e_reload_on_create_delete

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stakater/Reloader/test/utils"
)

var _ = Describe("ReloadOnCreate Feature Tests", Ordered, func() {
	var testNS string

	BeforeAll(func() {
		By("creating test namespace")
		testNS = SetupTestNamespace()
	})

	AfterAll(func() {
		By("cleaning up test namespace")
		CleanupTestNamespace()
	})

	Context("When --reload-on-create is enabled", func() {
		It("should reload deployment when a new Secret is created (annotation-based)", func() {
			deploymentName := "test-reload-on-create-secret"
			secretName := "new-secret"

			By("Creating a Deployment with auto reload annotation")
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

			By("Getting initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(1))

			By("Creating the new Secret (triggers reload)")
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

			By("Waiting for pods to be reloaded")
			time.Sleep(10 * time.Second)

			By("Verifying that new pods were created")
			Eventually(func() bool {
				newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
				if err != nil {
					return false
				}
				return len(newUIDs) > 0 && !utils.StringSlicesEqual(initialUIDs, newUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Pods should have been reloaded after Secret creation")

			By("Verifying pods are running with the new Secret")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment": {deploymentName},
				"secret":     {secretName},
			})
		})

		It("should reload deployment when a new ConfigMap is created (CRD-based)", func() {
			deploymentName := "test-reload-on-create-cm"
			configMapName := "new-configmap"
			reloaderConfigName := "test-create-rc"

			By("Creating a ReloaderConfig watching a not-yet-created ConfigMap")
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
    - kind: Deployment
      name: %s
  reloadStrategy: env-vars
`, reloaderConfigName, testNS, configMapName, deploymentName)

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
        - name: CONFIG_DATA
          valueFrom:
            configMapKeyRef:
              name: %s
              key: data
              optional: true
`, deploymentName, testNS, deploymentName, deploymentName, configMapName)

			Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

			By("waiting for Deployment to be ready")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("Getting initial pod UIDs")
			initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialUIDs).To(HaveLen(1))

			By("Creating the new ConfigMap (triggers reload)")
			configMapYAML := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  data: "initial-value"
`, configMapName, testNS)

			Expect(utils.ApplyYAML(configMapYAML)).To(Succeed())

			By("Waiting for pods to be reloaded")
			time.Sleep(10 * time.Second)

			By("Verifying that new pods were created")
			Eventually(func() bool {
				newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
				if err != nil {
					return false
				}
				return len(newUIDs) > 0 && !utils.StringSlicesEqual(initialUIDs, newUIDs)
			}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Pods should have been reloaded after ConfigMap creation")

			By("Verifying pods are running with the new ConfigMap")
			Eventually(func() error {
				return utils.WaitForPodsReady(testNS, "app="+deploymentName, 1, 30*time.Second)
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			// Cleanup resources on success
			CleanupResourcesOnSuccess(testNS, map[string][]string{
				"deployment":     {deploymentName},
				"configmap":      {configMapName},
				"reloaderconfig": {reloaderConfigName},
			})
		})
	})
})
