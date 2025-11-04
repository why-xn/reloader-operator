# E2E Test Implementation Roadmap

**Date:** 2025-10-31
**Current Status:** Planning Complete → Ready for Implementation

---

## 🎯 Quick Start Guide

### Step 1: Create Test Utilities (30 minutes)

**File:** `test/utils/reloader_helpers.go`

**Priority Functions:**
```go
// Essential for all tests
GetPodUIDs(namespace, workloadType, name string) ([]string, error)
WaitForPodsReady(namespace, labelSelector string, count int, timeout time.Duration) error
WaitForRolloutComplete(namespace, workloadType, name string, timeout time.Duration) error

// ReloaderConfig helpers
GetReloaderConfigStatus(namespace, name string) (*reloaderv1alpha1.ReloaderConfigStatus, error)
WaitForStatusUpdate(namespace, name string, checkFunc func(*reloaderv1alpha1.ReloaderConfigStatus) bool, timeout time.Duration) error

// Pod template inspection
GetPodTemplateEnvVars(namespace, workloadType, name string) (map[string]string, error)
GetPodTemplateAnnotations(namespace, workloadType, name string) (map[string]string, error)

// Apply/Delete helpers
ApplyYAML(yamlContent string) error
DeleteYAML(yamlContent string) error
ApplyFile(filepath string) error
```

---

### Step 2: Create Test Namespace Helper (15 minutes)

**File:** `test/e2e/helpers.go`

```go
package e2e

const testNamespace = "test-reloader"

// SetupTestNamespace creates and returns test namespace
func SetupTestNamespace() string {
    cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
    _, _ = utils.Run(cmd)
    return testNamespace
}

// CleanupTestNamespace deletes test namespace
func CleanupTestNamespace() {
    cmd := exec.Command("kubectl", "delete", "namespace", testNamespace, "--wait=false")
    _, _ = utils.Run(cmd)
}

// GenerateUniqueResourceName creates unique names for test resources
func GenerateUniqueResourceName(base string) string {
    return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}
```

---

### Step 3: Implement Core Test Scenario (1 hour)

**File:** `test/e2e/reloader_test.go`

Start with the most basic test:

```go
//go:build e2e
// +build e2e

package e2e

import (
    "time"
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("ReloaderConfig", Ordered, func() {
    var testNS string

    BeforeAll(func() {
        testNS = SetupTestNamespace()
    })

    AfterAll(func() {
        CleanupTestNamespace()
    })

    Context("CRD-based Configuration", func() {
        It("Should reload Deployment when Secret changes", func() {
            secretName := "test-secret"
            deploymentName := "test-app"
            reloaderConfigName := "test-config"

            By("Creating a Secret")
            secretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
stringData:
  password: initial-value
`, secretName, testNS)
            Expect(utils.ApplyYAML(secretYAML)).To(Succeed())

            By("Creating a Deployment that uses the Secret")
            deploymentYAML := fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: 2
  selector:
    matchLabels:
      app: test
  template:
    metadata:
      labels:
        app: test
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        env:
        - name: PASSWORD
          valueFrom:
            secretKeyRef:
              name: %s
              key: password
`, deploymentName, testNS, secretName)
            Expect(utils.ApplyYAML(deploymentYAML)).To(Succeed())

            By("Waiting for Deployment to be ready")
            Eventually(func() error {
                return utils.WaitForPodsReady(testNS, "app=test", 2, 30*time.Second)
            }, 2*time.Minute, 5*time.Second).Should(Succeed())

            By("Capturing initial pod UIDs")
            initialUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
            Expect(err).NotTo(HaveOccurred())
            Expect(initialUIDs).To(HaveLen(2))

            By("Creating a ReloaderConfig")
            reloaderConfigYAML := fmt.Sprintf(`
apiVersion: reloader.stakater.com/v1alpha1
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
`, reloaderConfigName, testNS, secretName, deploymentName)
            Expect(utils.ApplyYAML(reloaderConfigYAML)).To(Succeed())

            By("Waiting for ReloaderConfig status to be initialized")
            Eventually(func() bool {
                status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
                if err != nil {
                    return false
                }
                return len(status.WatchedResourceHashes) > 0
            }, 30*time.Second, 1*time.Second).Should(BeTrue())

            By("Updating the Secret")
            updatedSecretYAML := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: %s
stringData:
  password: updated-value
`, secretName, testNS)
            Expect(utils.ApplyYAML(updatedSecretYAML)).To(Succeed())

            By("Waiting for ReloaderConfig to trigger reload")
            Eventually(func() bool {
                status, err := utils.GetReloaderConfigStatus(testNS, reloaderConfigName)
                if err != nil {
                    return false
                }
                return status.ReloadCount > 0
            }, 1*time.Minute, 2*time.Second).Should(BeTrue())

            By("Waiting for Deployment rollout to complete")
            Eventually(func() error {
                return utils.WaitForRolloutComplete(testNS, "deployment", deploymentName, 30*time.Second)
            }, 2*time.Minute, 5*time.Second).Should(Succeed())

            By("Verifying new pods were created")
            newUIDs, err := utils.GetPodUIDs(testNS, "deployment", deploymentName)
            Expect(err).NotTo(HaveOccurred())
            Expect(newUIDs).To(HaveLen(2))
            Expect(newUIDs).NotTo(Equal(initialUIDs), "Pod UIDs should be different after reload")

            By("Verifying RELOADER_TRIGGERED_AT env var was added")
            envVars, err := utils.GetPodTemplateEnvVars(testNS, "deployment", deploymentName)
            Expect(err).NotTo(HaveOccurred())
            Expect(envVars).To(HaveKey("RELOADER_TRIGGERED_AT"))

            By("Verifying resource hash annotation was added")
            annotations, err := utils.GetPodTemplateAnnotations(testNS, "deployment", deploymentName)
            Expect(err).NotTo(HaveOccurred())
            Expect(annotations).To(HaveKey("reloader.stakater.com/resource-hash"))
        })
    })
})
```

---

### Step 4: Add More Test Scenarios (Iterative)

**Priority Order:**

1. **ConfigMap → Deployment** (30 min)
2. **Annotation-based reload** (30 min)
3. **Annotations strategy** (vs env-vars) (30 min)
4. **StatefulSet reload** (30 min)
5. **DaemonSet reload** (30 min)
6. **Multiple targets** (30 min)
7. **Pause periods** (45 min)
8. **Error handling** (45 min)

---

## 📁 File Structure After Implementation

```
test/
├── e2e/
│   ├── e2e_suite_test.go        # Suite setup (existing)
│   ├── e2e_test.go               # Basic manager tests (existing)
│   ├── reloader_test.go          # NEW: Core reload tests
│   ├── annotation_test.go        # NEW: Annotation-based tests
│   ├── strategy_test.go          # NEW: Strategy tests
│   ├── edge_cases_test.go        # NEW: Error handling
│   ├── alerting_test.go          # NEW: Alert integration (optional)
│   └── helpers.go                # NEW: Test helpers
└── utils/
    ├── kubectl.go                # Kubectl utilities (existing)
    └── reloader_helpers.go       # NEW: Reloader-specific helpers
```

---

## 🔧 Implementation Checklist

### Phase 1: Foundation ⏱️ ~1.5 hours

- [ ] Create `test/utils/reloader_helpers.go`
  - [ ] `GetPodUIDs()`
  - [ ] `WaitForPodsReady()`
  - [ ] `WaitForRolloutComplete()`
  - [ ] `GetReloaderConfigStatus()`
  - [ ] `ApplyYAML()` / `DeleteYAML()`
  - [ ] `GetPodTemplateEnvVars()`
  - [ ] `GetPodTemplateAnnotations()`

- [ ] Create `test/e2e/helpers.go`
  - [ ] `SetupTestNamespace()`
  - [ ] `CleanupTestNamespace()`
  - [ ] `GenerateUniqueResourceName()`

- [ ] Test the helpers work correctly

### Phase 2: Core Scenarios ⏱️ ~3 hours

- [ ] `test/e2e/reloader_test.go` - Core CRD tests
  - [ ] Secret → Deployment (env-vars)
  - [ ] ConfigMap → Deployment (env-vars)
  - [ ] ConfigMap → StatefulSet (annotations)
  - [ ] Multiple targets test

- [ ] `test/e2e/annotation_test.go` - Annotation tests
  - [ ] Explicit secret list
  - [ ] Auto-reload annotation
  - [ ] Ignore annotation

### Phase 3: Advanced Features ⏱️ ~2 hours

- [ ] `test/e2e/strategy_test.go` - Strategy tests
  - [ ] Env-vars strategy verification
  - [ ] Annotations strategy verification
  - [ ] Strategy switching

- [ ] Add pause period tests

### Phase 4: Edge Cases ⏱️ ~1.5 hours

- [ ] `test/e2e/edge_cases_test.go`
  - [ ] Missing target workload
  - [ ] Missing watched resource
  - [ ] Resource deletion
  - [ ] Status conditions

### Phase 5: Optional Enhancements ⏱️ ~2 hours

- [ ] `test/e2e/alerting_test.go`
  - [ ] Create mock webhook server
  - [ ] Test alert delivery

- [ ] Performance tests
  - [ ] Multiple resources
  - [ ] Large deployments

---

## 🚀 Running Tests During Development

### Test Individual Scenarios

```bash
# Run specific test
go test -tags=e2e ./test/e2e/ -v -ginkgo.focus "Should reload Deployment when Secret changes"

# Run all reloader tests
go test -tags=e2e ./test/e2e/ -v -ginkgo.focus "ReloaderConfig"

# Dry run (see what tests will run)
go test -tags=e2e ./test/e2e/ -v -ginkgo.dry-run
```

### Debug Failed Tests

```bash
# Keep test namespace for inspection
# (Modify test to skip CleanupTestNamespace)

# Inspect resources
kubectl get all -n test-reloader
kubectl get reloaderconfigs -n test-reloader
kubectl describe deployment test-app -n test-reloader

# Check operator logs
kubectl logs -n reloader-operator-system deployment/reloader-operator-controller-manager --tail=100

# Check events
kubectl get events -n test-reloader --sort-by='.lastTimestamp'
```

---

## 📝 Code Examples

### Example: GetPodUIDs Implementation

```go
// GetPodUIDs returns UIDs of all pods matching a workload
func GetPodUIDs(namespace, workloadType, workloadName string) ([]string, error) {
    var labelSelector string

    // Get label selector based on workload type
    cmd := exec.Command("kubectl", "get", workloadType, workloadName,
        "-n", namespace,
        "-o", "jsonpath={.spec.selector.matchLabels}")
    output, err := Run(cmd)
    if err != nil {
        return nil, err
    }

    // Parse matchLabels JSON to create selector
    var labels map[string]string
    if err := json.Unmarshal([]byte(output), &labels); err != nil {
        return nil, err
    }

    // Build selector string
    var selectors []string
    for k, v := range labels {
        selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
    }
    labelSelector = strings.Join(selectors, ",")

    // Get pod UIDs
    cmd = exec.Command("kubectl", "get", "pods",
        "-n", namespace,
        "-l", labelSelector,
        "-o", "jsonpath={.items[*].metadata.uid}")
    output, err = Run(cmd)
    if err != nil {
        return nil, err
    }

    uids := strings.Fields(output)
    return uids, nil
}
```

### Example: WaitForRolloutComplete

```go
// WaitForRolloutComplete waits for a workload rollout to complete
func WaitForRolloutComplete(namespace, workloadType, name string, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    cmd := exec.CommandContext(ctx, "kubectl", "rollout", "status",
        fmt.Sprintf("%s/%s", workloadType, name),
        "-n", namespace,
        "--timeout", timeout.String())

    _, err := Run(cmd)
    return err
}
```

---

## 🎓 Best Practices

### 1. Use Unique Resource Names
```go
// Good: Prevents conflicts between parallel tests
secretName := fmt.Sprintf("test-secret-%d", time.Now().Unix())

// Bad: Can cause conflicts
secretName := "test-secret"
```

### 2. Always Clean Up
```go
defer func() {
    // Clean up even if test fails
    DeleteYAML(secretYAML)
    DeleteYAML(deploymentYAML)
}()
```

### 3. Use Eventually with Appropriate Timeouts
```go
// Good: Reasonable timeout for pod startup
Eventually(checkFunc, 2*time.Minute, 5*time.Second).Should(Succeed())

// Bad: Too short
Eventually(checkFunc, 5*time.Second, 1*time.Second).Should(Succeed())
```

### 4. Log Context for Debugging
```go
By("Creating a Secret with data: password=initial")
// This appears in Ginkgo output for debugging
```

### 5. Verify Both Success and Side Effects
```go
// Verify the main goal
Expect(newUIDs).NotTo(Equal(initialUIDs))

// Also verify expected side effects
Expect(status.ReloadCount).To(Equal(1))
Expect(status.LastReloadTime).NotTo(BeNil())
```

---

## 📊 Expected Test Duration

| Phase | Tests | Duration |
|-------|-------|----------|
| Suite Setup | 1 | ~2 min |
| Core CRD Tests | 4 | ~8 min |
| Annotation Tests | 3 | ~6 min |
| Strategy Tests | 2 | ~4 min |
| Edge Cases | 4 | ~6 min |
| **Total** | **14** | **~26 min** |

With parallelization (Ginkgo -p): **~12-15 minutes**

---

## ✅ Definition of Done

E2E test implementation is complete when:

1. ✅ All helper functions implemented and tested
2. ✅ At least 10 test scenarios passing
3. ✅ Tests run reliably on Kind cluster
4. ✅ Clear failure messages for debugging
5. ✅ Documentation updated
6. ✅ CI integration ready (Makefile targets work)
7. ✅ Code reviewed and merged

---

## 🔗 Next Actions

**Immediate (Today):**
1. Review and approve this plan
2. Create `test/utils/reloader_helpers.go` (start with 3-4 core functions)
3. Implement first test: "Should reload Deployment when Secret changes"

**This Week:**
1. Complete Phase 1 (Foundation)
2. Complete Phase 2 (Core Scenarios)
3. Start Phase 3 (Advanced Features)

**Next Week:**
1. Complete Phase 3 & 4
2. CI integration
3. Documentation

---

**Status:** ✅ Ready to Start Implementation
**First Task:** Create `test/utils/reloader_helpers.go` with core functions
**Estimated Time to Complete:** 8-10 hours over 2-3 days
