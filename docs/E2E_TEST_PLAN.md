# End-to-End (E2E) Test Plan for Reloader Operator

**Date:** 2025-10-31
**Status:** Planning Phase
**Environment:** Kind cluster with deployed operator

---

## 📋 Overview

This document outlines comprehensive end-to-end test scenarios for the Reloader Operator. These tests will validate real-world functionality in a live Kubernetes cluster (Kind) to ensure the operator works correctly in production-like environments.

### Goals

- ✅ Verify actual pod restarts occur when resources change
- ✅ Test both CRD-based and annotation-based configurations
- ✅ Validate all reload strategies (env-vars, annotations)
- ✅ Test all workload types (Deployments, StatefulSets, DaemonSets)
- ✅ Verify status updates and condition tracking
- ✅ Test edge cases and error scenarios
- ✅ Validate alerting integrations (with mock webhooks)

---

## 🏗️ Test Architecture

### Current Structure

```
test/
├── e2e/
│   ├── e2e_suite_test.go    # Suite setup (existing)
│   ├── e2e_test.go           # Basic manager tests (existing)
│   └── reloader_test.go      # NEW: Reloader functionality tests
└── utils/
    ├── kubectl.go            # Kubectl utilities (existing)
    └── reloader_helpers.go   # NEW: Test helper functions
```

### Test Environment

- **Cluster:** Kind (already running)
- **Namespace:** `reloader-operator-system` (manager) + `test-reloader` (test resources)
- **Image:** Built and loaded into Kind
- **Timeout:** 2 minutes per test (with 1s polling)

---

## 🧪 Test Scenarios

### Category 1: CRD-Based Configuration

#### 1.1 Secret Change → Deployment Reload (env-vars strategy)
**Test:** `Should reload Deployment when Secret changes using CRD config`

**Steps:**
1. Create Secret `test-secret` with `password=initial`
2. Create Deployment `test-app` with 2 replicas referencing the Secret
3. Create ReloaderConfig watching the Secret, targeting the Deployment
4. Wait for Deployment to be ready and capture initial pod names/UIDs
5. Update Secret `password=updated`
6. Wait for ReloaderConfig status to show reload triggered
7. Verify Deployment rollout (new pods created, old pods terminated)
8. Verify pod template has updated `RELOADER_TRIGGERED_AT` env var
9. Verify pod template has resource hash annotation

**Expected:**
- ✅ New pods created with different names/UIDs
- ✅ Old pods terminated
- ✅ ReloaderConfig status updated (lastReloadTime, reloadCount++)
- ✅ ReloaderConfig condition `Available=True`

---

#### 1.2 ConfigMap Change → StatefulSet Reload (annotations strategy)
**Test:** `Should reload StatefulSet when ConfigMap changes using annotations strategy`

**Steps:**
1. Create ConfigMap `app-config` with `setting=value1`
2. Create StatefulSet `test-sts` with 2 replicas referencing the ConfigMap
3. Create ReloaderConfig with `reloadStrategy: annotations`
4. Wait for StatefulSet to be ready
5. Update ConfigMap `setting=value2`
6. Verify StatefulSet rolling update (pods restarted in order)
7. Verify pod template annotations updated (last-reload timestamp + hash)

**Expected:**
- ✅ Pods restarted in StatefulSet order (sts-0, sts-1)
- ✅ Pod template has updated annotations (not env vars)
- ✅ ReloaderConfig status updated

---

#### 1.3 Multiple Targets
**Test:** `Should reload multiple workloads when shared Secret changes`

**Steps:**
1. Create Secret `shared-secret`
2. Create Deployment `app-1`, StatefulSet `app-2`, DaemonSet `app-3` all using the Secret
3. Create ReloaderConfig with all 3 targets
4. Update Secret
5. Verify all 3 workloads reload simultaneously

**Expected:**
- ✅ All 3 workloads receive rollout
- ✅ ReloaderConfig shows 3 targets reloaded

---

#### 1.4 AutoReloadAll
**Test:** `Should reload all workloads in namespace when autoReloadAll is enabled`

**Steps:**
1. Create Secret `auto-secret`
2. Create Deployments `app-1`, `app-2` (no explicit targets)
3. Create ReloaderConfig with `autoReloadAll: true` for Secrets
4. Update Secret
5. Verify workloads that reference the Secret get reloaded

**Expected:**
- ✅ Only workloads referencing the Secret are reloaded
- ✅ Other workloads not affected

---

### Category 2: Annotation-Based Configuration

#### 2.1 Explicit Secret List
**Test:** `Should reload Deployment with secret.reloader.stakater.com/reload annotation`

**Steps:**
1. Create Secret `my-secret`
2. Create Deployment with annotation: `secret.reloader.stakater.com/reload: "my-secret"`
3. Update Secret
4. Verify Deployment reloads without ReloaderConfig CRD

**Expected:**
- ✅ Backward compatibility works
- ✅ No ReloaderConfig CRD needed

---

#### 2.2 Auto-Reload Annotation
**Test:** `Should auto-reload when workload has reloader.stakater.com/auto: "true"`

**Steps:**
1. Create Secret `app-secret`
2. Create Deployment referencing Secret in env vars with annotation `reloader.stakater.com/auto: "true"`
3. Update Secret
4. Verify reload happens automatically

**Expected:**
- ✅ Operator detects resource reference
- ✅ Reload triggered without explicit list

---

#### 2.3 Ignore Annotation
**Test:** `Should NOT reload when reloader.stakater.com/ignore: "true"`

**Steps:**
1. Create Secret `ignored-secret`
2. Create Deployment with annotation `reloader.stakater.com/ignore: "true"`
3. Update Secret
4. Verify Deployment does NOT reload

**Expected:**
- ✅ No reload occurs
- ❌ Pod names/UIDs remain unchanged

---

### Category 3: Reload Strategies

#### 3.1 Env-Vars Strategy
**Test:** `Should add RELOADER_TRIGGERED_AT env var`

**Steps:**
1. Create resources with `reloadStrategy: env-vars`
2. Trigger reload
3. Verify pod spec has `RELOADER_TRIGGERED_AT` env var in container[0]
4. Verify timestamp is valid RFC3339 format

---

#### 3.2 Annotations Strategy
**Test:** `Should update pod template annotations`

**Steps:**
1. Create resources with `reloadStrategy: annotations`
2. Trigger reload
3. Verify pod template has:
   - `reloader.stakater.com/last-reload` = timestamp
   - `reloader.stakater.com/resource-hash` = SHA256 hash

---

### Category 4: Status and Conditions

#### 4.1 Status Updates
**Test:** `Should update ReloaderConfig status after successful reload`

**Verify:**
- ✅ `status.watchedResourceHashes` updated
- ✅ `status.lastReloadTime` set
- ✅ `status.reloadCount` incremented
- ✅ `status.targetStatus[]` contains per-target info

---

#### 4.2 Conditions
**Test:** `Should set conditions correctly`

**Verify:**
- ✅ Initial: `Available=False` (no targets)
- ✅ After target found: `Available=True`
- ✅ After error: `Error=True` with message

---

### Category 5: Pause Periods

#### 5.1 Pause Period Enforcement
**Test:** `Should NOT reload during pause period`

**Steps:**
1. Create ReloaderConfig with target having `pausePeriod: 1m`
2. Trigger first reload → succeeds
3. Immediately update Secret again
4. Verify second reload is skipped
5. Wait 1 minute
6. Update Secret again → reload succeeds

**Expected:**
- ✅ Second reload skipped with log message
- ✅ Third reload succeeds after pause expires

---

### Category 6: Edge Cases and Error Handling

#### 6.1 Missing Target
**Test:** `Should handle missing target workload gracefully`

**Steps:**
1. Create ReloaderConfig targeting non-existent Deployment
2. Verify status shows error condition
3. Create the Deployment
4. Verify operator recovers and watches it

---

#### 6.2 Missing Resource
**Test:** `Should handle missing watched resource`

**Steps:**
1. Create ReloaderConfig watching non-existent Secret
2. Verify status updated but no errors
3. Create the Secret
4. Verify operator starts watching

---

#### 6.3 Resource Deletion
**Test:** `Should handle watched resource deletion`

**Steps:**
1. Setup working reload scenario
2. Delete the watched Secret
3. Verify status shows resource missing
4. Recreate Secret
5. Verify functionality restored

---

#### 6.4 Invalid Configuration
**Test:** `Should reject invalid ReloaderConfig`

**Steps:**
1. Try to create ReloaderConfig with invalid strategy
2. Verify CRD validation rejects it

---

### Category 7: Alerting Integration

#### 7.1 Slack Webhook (Mock)
**Test:** `Should send alert to Slack webhook on reload`

**Steps:**
1. Deploy mock webhook server (simple HTTP server)
2. Create Secret with Slack webhook URL
3. Create ReloaderConfig with Slack alert config
4. Trigger reload
5. Verify mock webhook received POST with expected payload

**Expected:**
- ✅ Webhook called with correct JSON
- ✅ Message contains workload info, resource name, timestamp

---

#### 7.2 Multiple Alert Channels
**Test:** `Should send alerts to multiple channels`

**Steps:**
1. Deploy mock webhook servers for Slack, Teams, Google Chat
2. Configure ReloaderConfig with all 3 channels
3. Trigger reload
4. Verify all 3 webhooks received alerts

---

#### 7.3 Alert on Error
**Test:** `Should send error alert when reload fails`

**Steps:**
1. Configure alerts
2. Create scenario that causes reload failure (e.g., invalid workload)
3. Verify error alert sent with error message

---

### Category 8: Performance and Scale

#### 8.1 Multiple Resources
**Test:** `Should handle multiple Secrets/ConfigMaps changing simultaneously`

**Steps:**
1. Create 10 Secrets and 10 Deployments
2. Create ReloaderConfigs for all
3. Update all 10 Secrets at once
4. Verify all Deployments reload successfully

---

#### 8.2 Large Deployment
**Test:** `Should handle deployment with many replicas`

**Steps:**
1. Create Deployment with 20 replicas
2. Trigger reload
3. Verify rolling update completes successfully

---

## 🛠️ Test Utilities Needed

### Helper Functions (test/utils/reloader_helpers.go)

```go
// GetPodUIDs returns UIDs of all pods for a workload
func GetPodUIDs(namespace, workloadType, workloadName string) ([]string, error)

// WaitForRollout waits for a workload to complete rollout
func WaitForRollout(namespace, workloadType, workloadName string, timeout time.Duration) error

// GetReloaderConfigStatus retrieves status of a ReloaderConfig
func GetReloaderConfigStatus(namespace, name string) (*ReloaderConfigStatus, error)

// VerifyPodTemplateHasEnvVar checks for env var in pod template
func VerifyPodTemplateHasEnvVar(namespace, workloadType, workloadName, envName string) (bool, error)

// VerifyPodTemplateAnnotation checks for annotation in pod template
func VerifyPodTemplateAnnotation(namespace, workloadType, workloadName, key string) (string, error)

// CreateMockWebhookServer deploys a simple HTTP server for webhook testing
func CreateMockWebhookServer(namespace string) (string, error) // returns URL

// GetWebhookCalls retrieves calls made to mock webhook
func GetWebhookCalls(namespace, serverName string) ([]WebhookCall, error)
```

### YAML Generators

```go
// GenerateSecret creates a Secret YAML string
func GenerateSecret(name, namespace string, data map[string]string) string

// GenerateDeployment creates a Deployment YAML string
func GenerateDeployment(name, namespace string, opts DeploymentOpts) string

// GenerateReloaderConfig creates a ReloaderConfig YAML string
func GenerateReloaderConfig(name, namespace string, spec ReloaderConfigSpec) string
```

---

## 📊 Test Execution Plan

### Phase 1: Core Functionality (Priority 1)
- ✅ Secret → Deployment reload (CRD + env-vars)
- ✅ ConfigMap → Deployment reload (CRD + env-vars)
- ✅ Annotation-based reload
- ✅ Status updates

**Estimated Time:** 2 hours

### Phase 2: Advanced Features (Priority 2)
- ✅ Both reload strategies
- ✅ All workload types
- ✅ Multiple targets
- ✅ Pause periods

**Estimated Time:** 3 hours

### Phase 3: Edge Cases (Priority 3)
- ✅ Error handling
- ✅ Missing resources
- ✅ Invalid configs

**Estimated Time:** 1 hour

### Phase 4: Integration (Priority 4)
- ✅ Alerting (mock webhooks)
- ✅ Performance tests

**Estimated Time:** 2 hours

---

## 🚀 How to Run E2E Tests

### Prerequisites

```bash
# Ensure Kind is installed
kind version

# Ensure Docker is running
docker ps
```

### Run Tests

```bash
# Run all e2e tests (creates temporary Kind cluster)
make test-e2e

# Run e2e tests on existing cluster
KIND_CLUSTER=my-cluster make test-e2e

# Run specific test
go test -tags=e2e ./test/e2e/ -v -ginkgo.focus "Should reload Deployment"

# Skip cleanup (for debugging)
# Modify Makefile to not call cleanup-test-e2e
```

### Debug Failed Tests

```bash
# Get operator logs
kubectl logs -n reloader-operator-system deployment/reloader-operator-controller-manager

# Get test resources
kubectl get reloaderconfigs -A
kubectl get deployments -n test-reloader

# Check events
kubectl get events -n test-reloader --sort-by='.lastTimestamp'
```

---

## 📝 Test Coverage Matrix

| Feature | Unit Tests | Integration Tests | E2E Tests |
|---------|-----------|-------------------|-----------|
| Secret watching | ✅ | ✅ | 🔲 Planned |
| ConfigMap watching | ✅ | ✅ | 🔲 Planned |
| CRD-based config | ✅ | ✅ | 🔲 Planned |
| Annotation-based | ✅ | ✅ | 🔲 Planned |
| Env-vars strategy | ✅ | ✅ | 🔲 Planned |
| Annotations strategy | ✅ | ✅ | 🔲 Planned |
| Deployment reload | ✅ | ✅ | 🔲 Planned |
| StatefulSet reload | ✅ | ✅ | 🔲 Planned |
| DaemonSet reload | ✅ | ✅ | 🔲 Planned |
| Status updates | ✅ | ✅ | 🔲 Planned |
| Conditions | ✅ | ✅ | 🔲 Planned |
| Pause periods | ✅ | ❌ | 🔲 Planned |
| Alerting | ✅ | ❌ | 🔲 Planned |
| AutoReloadAll | ✅ | ❌ | 🔲 Planned |

---

## 🎯 Success Criteria

E2E tests are considered complete when:

- ✅ All core scenarios pass (Category 1-2)
- ✅ Both reload strategies verified (Category 3)
- ✅ Status tracking confirmed (Category 4)
- ✅ Error handling validated (Category 6)
- ✅ At least 1 alerting test passes (Category 7)
- ✅ Tests run reliably in CI environment
- ✅ Total test execution < 10 minutes

---

## 📌 Next Steps

1. **Implement test utilities** (`test/utils/reloader_helpers.go`)
2. **Create mock webhook server** (for alerting tests)
3. **Write core test scenarios** (Category 1-2 in `test/e2e/reloader_test.go`)
4. **Add advanced scenarios** (Category 3-5)
5. **Integrate into CI/CD** (GitHub Actions or similar)
6. **Document test maintenance** (how to add new tests)

---

## 📚 References

- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [Kind - Kubernetes in Docker](https://kind.sigs.k8s.io/)
- [Kubebuilder E2E Testing](https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html)

---

**Last Updated:** 2025-10-31
**Status:** ✅ Plan Complete - Ready for Implementation
