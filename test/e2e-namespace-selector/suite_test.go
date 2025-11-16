//go:build e2e
// +build e2e

package e2e_namespace_selector

import (
	"fmt"
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stakater/Reloader/test/utils"
)

var (
	matchingNamespace    = "test-ns-matching"
	nonMatchingNamespace = "test-ns-non-matching"
	ignoredNamespace     = "test-ns-ignored"
)

func TestE2ENamespaceSelector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Namespace Selector E2E Suite")
}

var _ = BeforeSuite(func() {
	By("Setting up test namespaces")
	// Create namespace WITH matching labels
	matchingNsYAML := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    environment: production
    team: backend
`, matchingNamespace)
	Expect(utils.ApplyYAML(matchingNsYAML)).To(Succeed())

	// Create namespace WITHOUT matching labels
	nonMatchingNsYAML := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    environment: development
    team: frontend
`, nonMatchingNamespace)
	Expect(utils.ApplyYAML(nonMatchingNsYAML)).To(Succeed())

	// Create namespace to be ignored
	ignoredNsYAML := fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    environment: production
    team: backend
`, ignoredNamespace)
	Expect(utils.ApplyYAML(ignoredNsYAML)).To(Succeed())

	By("Deploying operator with namespace filtering enabled")
	// Patch the deployment to add namespace filtering flags
	patchYAML := `
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --metrics-bind-address=0
        - --leader-elect
        - --health-probe-bind-address=:8081
        - --namespace-selector=environment=production
        - --namespaces-to-ignore=test-ns-ignored
`
	cmd := exec.Command("kubectl", "patch", "deployment", "reloader-operator-controller-manager",
		"-n", "reloader-operator-system",
		"--type", "strategic",
		"-p", patchYAML)
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to patch deployment: %s", string(output))

	By("Waiting for operator to restart with new configuration")
	cmd = exec.Command("kubectl", "rollout", "status", "deployment/reloader-operator-controller-manager",
		"-n", "reloader-operator-system", "--timeout=2m")
	output, err = cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to wait for rollout: %s", string(output))

	time.Sleep(10 * time.Second) // Give operator time to initialize
})

var _ = AfterSuite(func() {
	By("Cleaning up test namespaces")
	for _, ns := range []string{matchingNamespace, nonMatchingNamespace, ignoredNamespace} {
		cmd := exec.Command("kubectl", "delete", "namespace", ns, "--ignore-not-found=true")
		_ = cmd.Run()
	}

	By("Restoring operator to default configuration")
	// Remove the namespace filtering flags
	patchYAML := `
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --metrics-bind-address=0
        - --leader-elect
        - --health-probe-bind-address=:8081
`
	cmd := exec.Command("kubectl", "patch", "deployment", "reloader-operator-controller-manager",
		"-n", "reloader-operator-system",
		"--type", "strategic",
		"-p", patchYAML)
	_ = cmd.Run()

	cmd = exec.Command("kubectl", "rollout", "status", "deployment/reloader-operator-controller-manager",
		"-n", "reloader-operator-system", "--timeout=2m")
	_ = cmd.Run()

	time.Sleep(5 * time.Second) // Give operator time to stabilize
})

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CleanupResourcesOnSuccess cleans up resources after successful test
func CleanupResourcesOnSuccess(namespace string, resources map[string][]string) {
	for resourceType, names := range resources {
		for _, name := range names {
			cmd := exec.Command("kubectl", "delete", resourceType, name,
				"-n", namespace, "--ignore-not-found=true", "--wait=false")
			_, _ = utils.Run(cmd)
		}
	}
}
