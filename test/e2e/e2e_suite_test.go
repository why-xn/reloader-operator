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
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stakater/Reloader/test/utils"
)

var (
	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/reloader-operator:v0.0.1"

	// skipSetup skips the BeforeSuite setup (operator deployment)
	// Set E2E_SKIP_SETUP=true to skip setup (useful when operator is already deployed)
	skipSetup = os.Getenv("E2E_SKIP_SETUP") == "true"

	// skipCleanup skips the AfterSuite cleanup (operator undeployment)
	// Set E2E_SKIP_CLEANUP=true to skip cleanup (useful for troubleshooting)
	skipCleanup = os.Getenv("E2E_SKIP_CLEANUP") == "true"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind and builds/loads the Manager Docker image locally.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting reloader-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	if skipSetup {
		_, _ = fmt.Fprintf(GinkgoWriter, "⏩ Skipping setup (E2E_SKIP_SETUP=true)\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   Assuming operator is already deployed...\n")
		return
	}

	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("installing ReloaderConfig CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
})

var _ = AfterSuite(func() {
	if skipCleanup {
		_, _ = fmt.Fprintf(GinkgoWriter, "⏩ Skipping cleanup (E2E_SKIP_CLEANUP=true)\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   Resources left intact for troubleshooting...\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   Troubleshooting commands:\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   - Check operator logs:  make e2e-logs\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   - Check test resources: kubectl get all -n test-reloader\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   - Check environment:    make e2e-status\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   - Reset test namespace: make e2e-reset\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "   - Full cleanup:         make e2e-cleanup\n")
		_, _ = fmt.Fprintf(GinkgoWriter, "\n")
		return
	}

	By("undeploying the controller-manager")
	cmd := exec.Command("make", "undeploy")
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = utils.Run(cmd)
})
