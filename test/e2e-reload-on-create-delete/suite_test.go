//go:build e2e
// +build e2e

package e2e_reload_on_create_delete

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
	testNamespace = "test-reload-on-create-delete"
)

func TestE2EReloadOnCreateDelete(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ReloadOn Create/Delete E2E Suite")
}

var _ = BeforeSuite(func() {
	By("Setting up test namespace")
	cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
	output, err := cmd.CombinedOutput()
	if err != nil && !contains(string(output), "already exists") && !contains(string(output), "AlreadyExists") {
		Fail(fmt.Sprintf("Failed to create namespace: %v\n%s", err, string(output)))
	}

	By("Deploying operator with reload-on-create and reload-on-delete enabled")
	// Patch the deployment to add the flags
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
        - --reload-on-create=true
        - --reload-on-delete=true
`
	cmd = exec.Command("kubectl", "patch", "deployment", "reloader-operator-controller-manager",
		"-n", "reloader-operator-system",
		"--type", "strategic",
		"-p", patchYAML)
	output, err = cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to patch deployment: %s", string(output))

	By("Waiting for operator to restart with new configuration")
	cmd = exec.Command("kubectl", "rollout", "status", "deployment/reloader-operator-controller-manager",
		"-n", "reloader-operator-system", "--timeout=2m")
	output, err = cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to wait for rollout: %s", string(output))

	time.Sleep(10 * time.Second) // Give operator time to initialize
})

var _ = AfterSuite(func() {
	By("Cleaning up test namespace")
	cmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--ignore-not-found=true")
	_ = cmd.Run()

	By("Restoring operator to default configuration")
	// Remove the reload flags
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
	cmd = exec.Command("kubectl", "patch", "deployment", "reloader-operator-controller-manager",
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

// SetupTestNamespace returns the test namespace
func SetupTestNamespace() string {
	return testNamespace
}

// CleanupTestNamespace is a no-op since we clean up in AfterSuite
func CleanupTestNamespace() {
	// No-op - cleanup happens in AfterSuite
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
