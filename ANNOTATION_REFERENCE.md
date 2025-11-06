# Reloader Annotation Reference Guide

**Last Updated:** 2025-11-05
**Purpose:** Complete reference of all Reloader annotations with implementation status in Reloader-Operator

---

## Quick Status Legend

- ✅ **Implemented** - Fully working in Reloader-Operator
- ⚠️ **Partial** - Implemented with limitations or differences
- ❌ **Missing** - Not yet implemented
- 🐛 **Broken** - Implemented but has bugs

---

## Table of Contents

1. [Auto-Reload Annotations](#1-auto-reload-annotations)
2. [Named Resource Reload Annotations](#2-named-resource-reload-annotations)
3. [Search and Match Annotations](#3-search-and-match-annotations)
4. [Ignore and Exclusion Annotations](#4-ignore-and-exclusion-annotations)
5. [Reload Strategy Annotations](#5-reload-strategy-annotations)
6. [Pause Period Annotations](#6-pause-period-annotations)
7. [Internal/Tracking Annotations](#7-internaltracking-annotations)
8. [Summary Table](#8-summary-table)

---

## 1. Auto-Reload Annotations

These annotations enable automatic reload when ANY referenced ConfigMap or Secret changes.

### 1.1 `reloader.stakater.com/auto`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"` or `"false"`
**Status:** ⚠️ **Partial** - Requires ReloaderConfig to exist

**What it does:**
- Automatically detects ALL ConfigMaps and Secrets referenced in the workload
- Triggers reload when ANY of them change
- Searches in: volumes, envFrom, env (valueFrom)

**Original Reloader Behavior:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"  # That's it! No other config needed
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config        # ← Will trigger reload
        - secretRef:
            name: db-credentials    # ← Will trigger reload
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
      volumes:
      - name: tls
        secret:
          secretName: tls-cert      # ← Will trigger reload
```

**Reloader-Operator Behavior:**
```yaml
# REQUIRES a ReloaderConfig in the same namespace:
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-config
spec:
  autoReloadAll: true
  targets:
    - kind: Deployment
      name: my-app

---
# Then the annotation works:
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
```

**Key Difference:**
- Original: Just annotation is enough
- Operator: Requires ReloaderConfig CRD

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:28` - `AnnotationAuto`
- Check logic: `internal/pkg/workload/finder.go:343`

---

### 1.2 `secret.reloader.stakater.com/auto`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"` or `"false"`
**Status:** ⚠️ **Partial** - Requires ReloaderConfig to exist

**What it does:**
- Automatically detects ONLY Secrets referenced in the workload
- Ignores ConfigMap changes
- Useful when you want fine-grained control

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    secret.reloader.stakater.com/auto: "true"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config        # ← Will NOT trigger reload
        - secretRef:
            name: db-credentials    # ← Will trigger reload
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:38` - `AnnotationSecretAuto`
- Check logic: `internal/pkg/workload/finder.go:350`

---

### 1.3 `configmap.reloader.stakater.com/auto`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"` or `"false"`
**Status:** ⚠️ **Partial** - Requires ReloaderConfig to exist

**What it does:**
- Automatically detects ONLY ConfigMaps referenced in the workload
- Ignores Secret changes
- Useful when you want fine-grained control

**Example:**
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: my-app
  annotations:
    configmap.reloader.stakater.com/auto: "true"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config        # ← Will trigger reload
        - secretRef:
            name: db-credentials    # ← Will NOT trigger reload
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:40` - `AnnotationConfigMapAuto`
- Check logic: `internal/pkg/workload/finder.go:355`

---

## 2. Named Resource Reload Annotations

These annotations allow you to specify EXACT ConfigMaps or Secrets to watch, even if they're not referenced in the pod spec.

### 2.1 `secret.reloader.stakater.com/reload`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** Comma-separated list of Secret names or regex patterns
**Status:** ⚠️ **Partial** - Comma-separated works, regex NOT supported

**What it does:**
- Watches specific Secrets by name
- Can watch Secrets that aren't referenced in pod spec
- Original Reloader supports regex patterns like `"secret-.*"`

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    secret.reloader.stakater.com/reload: "db-creds,api-keys,oauth-token"
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-creds        # ← Will trigger reload
              key: password
        # Note: api-keys and oauth-token might not be in pod spec!
        # But changes will still trigger reload
```

**Regex Example (NOT SUPPORTED YET):**
```yaml
metadata:
  annotations:
    # This would match: secret-prod, secret-dev, secret-staging
    secret.reloader.stakater.com/reload: "secret-.*"
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:37` - `AnnotationSecretReload`
- Parse logic: `internal/pkg/util/helpers.go:89` - `ParseCommaSeparatedList()`
- Check logic: `internal/pkg/workload/finder.go:362-372`

**Missing:** Regex pattern matching

---

### 2.2 `configmap.reloader.stakater.com/reload`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** Comma-separated list of ConfigMap names or regex patterns
**Status:** ⚠️ **Partial** - Comma-separated works, regex NOT supported

**What it does:**
- Watches specific ConfigMaps by name
- Can watch ConfigMaps that aren't referenced in pod spec
- Original Reloader supports regex patterns like `"config-.*"`

**Example:**
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: logging-agent
  annotations:
    configmap.reloader.stakater.com/reload: "fluent-config,log-filters"
spec:
  template:
    spec:
      containers:
      - name: fluentd
        volumeMounts:
        - name: config
          mountPath: /fluentd/etc
      volumes:
      - name: config
        configMap:
          name: fluent-config    # ← Will trigger reload
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:39` - `AnnotationConfigMapReload`
- Parse logic: `internal/pkg/util/helpers.go:89` - `ParseCommaSeparatedList()`
- Check logic: `internal/pkg/workload/finder.go:362-372`

**Missing:** Regex pattern matching

---

## 3. Search and Match Annotations

These annotations provide an opt-in mechanism where workloads only reload for resources that are explicitly tagged.

### 3.1 `reloader.stakater.com/search`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"` or `"false"`
**Status:** ❌ **Missing**

**What it does:**
- Enables "search mode" for the workload
- In search mode, the workload ONLY reloads if the changed resource has `reloader.stakater.com/match: "true"`
- Useful for selective reloading in environments with many ConfigMaps/Secrets

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/search: "true"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config        # Only reloads if app-config has match annotation
        - configMapRef:
            name: static-config     # Ignores changes to static-config

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  annotations:
    reloader.stakater.com/match: "true"  # This will trigger reload
data:
  key: value

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: static-config
  # No match annotation - changes ignored even though it's referenced
data:
  key: value
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:29` - `AnnotationSearch`
- **Not implemented in finder.go**

---

### 3.2 `reloader.stakater.com/match`

**Applied to:** ConfigMap, Secret
**Value:** `"true"` or `"false"`
**Status:** ❌ **Missing**

**What it does:**
- Tags a ConfigMap or Secret as eligible for reload
- Only works when workload has `reloader.stakater.com/search: "true"`
- Provides explicit opt-in for resources

**Use Case:**
You have 10 ConfigMaps referenced in your deployment, but only 2 should trigger reload.

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:30` - `AnnotationMatch`
- **Not implemented in finder.go**

---

## 4. Ignore and Exclusion Annotations

These annotations prevent reloads from being triggered.

### 4.1 `reloader.stakater.com/ignore` (on ConfigMap/Secret)

**Applied to:** ConfigMap, Secret
**Value:** `"true"` or `"false"`
**Status:** ⚠️ **Partial** - Only checked on ReloaderConfig, not on resources

**What it does:**
- GLOBAL ignore flag - prevents ANY workload from reloading when this resource changes
- Useful for static ConfigMaps/Secrets that never require reload

**Example:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: static-readonly-config
  annotations:
    reloader.stakater.com/ignore: "true"  # No workload will reload when this changes
data:
  timezone: "UTC"
  locale: "en_US"
```

**Current Implementation:**
- Only checked on ReloaderConfig objects (line 71 in finder.go)
- NOT checked on ConfigMaps/Secrets themselves

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:31` - `AnnotationIgnore`
- Partial check: `internal/pkg/workload/finder.go:71` (only for ReloaderConfig)
- **Missing:** Check in `reconcileSecret()` and `reconcileConfigMap()`

---

### 4.2 `configmaps.exclude.reloader.stakater.com/reload`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** Comma-separated list of ConfigMap names
**Status:** ❌ **Missing**

**What it does:**
- Workload-specific exclusion list
- Even if ConfigMap is referenced, changes won't trigger reload
- Useful when using `auto: true` but want to exclude specific ConfigMaps

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    configmaps.exclude.reloader.stakater.com/reload: "static-config,readonly-config"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config        # ← Will trigger reload
        - configMapRef:
            name: static-config     # ← Will NOT trigger reload (excluded)
        - configMapRef:
            name: readonly-config   # ← Will NOT trigger reload (excluded)
```

**Code Location:**
- **Not defined in constants**
- **Not implemented**

**Recommended Constant:** `AnnotationConfigMapExclude = "configmaps.exclude.reloader.stakater.com/reload"`

---

### 4.3 `secrets.exclude.reloader.stakater.com/reload`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** Comma-separated list of Secret names
**Status:** ❌ **Missing**

**What it does:**
- Workload-specific exclusion list for Secrets
- Even if Secret is referenced, changes won't trigger reload
- Useful when using `auto: true` but want to exclude specific Secrets

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    secrets.exclude.reloader.stakater.com/reload: "ca-cert,service-account-token"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - secretRef:
            name: db-credentials    # ← Will trigger reload
        - secretRef:
            name: ca-cert           # ← Will NOT trigger reload (excluded)
```

**Code Location:**
- **Not defined in constants**
- **Not implemented**

**Recommended Constant:** `AnnotationSecretExclude = "secrets.exclude.reloader.stakater.com/reload"`

---

## 5. Reload Strategy Annotations

These annotations control HOW the reload is triggered.

### 5.1 `reloader.stakater.com/rollout-strategy`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"env-vars"`, `"annotations"`, `"restart"`, or `"rollout"` (backward compatibility)
**Status:** ✅ **Fully Implemented**

**Backward Compatibility:**
- Original Reloader uses `"rollout"` (default) and `"restart"`
- We support both original values PLUS enhanced options
- `"rollout"` maps to `"env-vars"` for backward compatibility
- `"restart"` works identically in both versions

**What it does:**
- Controls how Kubernetes rolling update is triggered

**Strategies:**

1. **`env-vars` (default)** - ✅ Supported
   - Adds/updates environment variable: `RELOADER_TRIGGERED_AT=<timestamp>`
   - Forces pod restart via spec change
   - Works with all Kubernetes versions
   - **Alias:** `"rollout"` (for backward compatibility with original Reloader)

2. **`annotations`** - ✅ Supported
   - Updates pod template annotation: `reloader.stakater.com/last-reload=<timestamp>`
   - GitOps-friendly (ArgoCD/Flux ignore annotation changes)
   - Cleaner pod spec
   - **Enhancement:** Not available in original Reloader

3. **`restart`** - ✅ Supported
   - Deletes pods without changing pod template
   - **Most GitOps-friendly** (no template changes at all)
   - Uses `kubectl rollout restart` equivalent
   - Kubernetes recreates pods with updated ConfigMap/Secret data
   - **Same as original Reloader**

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    reloader.stakater.com/rollout-strategy: "annotations"  # Use annotations strategy
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:32` - `AnnotationReloadStrategy`
- Constants: `internal/pkg/util/helpers.go:65-70` - Strategy values (including `ReloadStrategyRestart` and `ReloadStrategyRollout` alias)
- Normalization: `internal/pkg/util/helpers.go:82-93` - `NormalizeStrategy()` function
- Used in: `internal/pkg/workload/finder.go:238,267,296`
- Applied in: `internal/pkg/workload/updater.go:43-70` - TriggerReload method
- Restart implementation: `internal/pkg/workload/updater.go:282-345` - `restartWorkloadPods()` function

---

## 6. Pause Period Annotations

These annotations prevent reload storms when multiple resources change quickly.

### 6.1 `deployment.reloader.stakater.com/pause-period`

**Applied to:** Deployment
**Value:** Duration string (e.g., `"5m"`, `"1h"`, `"30s"`)
**Status:** 🐛 **Broken** - Set but never enforced

**What it does:**
- Prevents multiple reloads within the specified duration
- First change triggers reload immediately
- Subsequent changes within pause period are ignored
- After pause period expires, next change triggers reload

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    deployment.reloader.stakater.com/pause-period: "5m"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config
        - secretRef:
            name: db-credentials

# Timeline:
# 10:00 - app-config changes → Reload triggered, pause until 10:05
# 10:02 - db-credentials changes → Ignored (still paused)
# 10:04 - app-config changes again → Ignored (still paused)
# 10:06 - app-config changes → Reload triggered, pause until 10:11
```

**Bug Details:**
- `PausedUntil` timestamp IS set in `updateTargetStatus()` (controller.go:930, 956)
- `IsPaused()` check EXISTS in `executeReloads()` (controller.go:571)
- BUT: Logic has a bug - pause not enforced correctly
- **Test failing:** `test/e2e/edge_cases_test.go:293`

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:43` - `AnnotationDeploymentPausePeriod`
- Read: `internal/pkg/workload/finder.go:241`
- Set: `internal/controller/reloaderconfig_controller.go:926-931`
- Check: `internal/controller/reloaderconfig_controller.go:571-583`
- **Bug location:** `internal/pkg/workload/updater.go` - IsPaused() implementation

---

### 6.2 `statefulset.reloader.stakater.com/pause-period`

**Applied to:** StatefulSet
**Value:** Duration string (e.g., `"5m"`, `"1h"`, `"30s"`)
**Status:** 🐛 **Broken** - Same bug as Deployment

**What it does:**
- Same as `deployment.reloader.stakater.com/pause-period` but for StatefulSets

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:44` - `AnnotationStatefulSetPausePeriod`
- Read: `internal/pkg/workload/finder.go:270`

---

### 6.3 `daemonset.reloader.stakater.com/pause-period`

**Applied to:** DaemonSet
**Value:** Duration string (e.g., `"5m"`, `"1h"`, `"30s"`)
**Status:** 🐛 **Broken** - Same bug as Deployment

**What it does:**
- Same as `deployment.reloader.stakater.com/pause-period` but for DaemonSets

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:45` - `AnnotationDaemonSetPausePeriod`
- Read: `internal/pkg/workload/finder.go:299`

---

### 6.4 `deployment.reloader.stakater.com/paused-at`

**Applied to:** Deployment (auto-set by operator)
**Value:** RFC3339 timestamp
**Status:** ❌ **Missing** - Not used

**What it does:**
- Automatically set by Reloader to track when deployment was paused
- Used for observability and debugging
- Should NOT be set manually

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    deployment.reloader.stakater.com/pause-period: "5m"
    deployment.reloader.stakater.com/paused-at: "2025-11-05T10:00:00Z"  # Auto-set
```

**Code Location:**
- **Not defined in constants**
- **Not implemented**

---

## 7. Internal/Tracking Annotations

These annotations are set by Reloader for tracking and are not meant to be manually configured.

### 7.1 `reloader.stakater.com/last-hash`

**Applied to:** ConfigMap, Secret (auto-set by operator)
**Value:** SHA256 hash string
**Status:** ✅ **Implemented**

**What it does:**
- Stores the last known hash of the resource data
- Used for change detection (only data changes trigger reload, not metadata)
- Automatically managed by the operator

**Example:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  annotations:
    reloader.stakater.com/last-hash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
data:
  key: value
```

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:27` - `AnnotationLastHash`
- Get: `internal/controller/reloaderconfig_controller.go:474`
- Update: `internal/controller/reloaderconfig_controller.go:735-760`

**Note:** Original Reloader uses SHA1, Reloader-Operator uses SHA256

---

### 7.2 `reloader.stakater.com/last-reload`

**Applied to:** Deployment, StatefulSet, DaemonSet (auto-set by operator)
**Value:** RFC3339 timestamp
**Status:** ⚠️ **Partial** - Used in annotations strategy only

**What it does:**
- Tracks when the workload was last reloaded
- Used in `annotations` reload strategy
- Can be used for debugging and audit trail

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:33` - `AnnotationLastReload`
- **Used in updater but not documented**

---

### 7.3 `reloader.stakater.com/resource-hash`

**Applied to:** Deployment, StatefulSet, DaemonSet (auto-set by operator)
**Value:** SHA256 hash string
**Status:** ⚠️ **Partial** - Defined but usage unclear

**What it does:**
- Stores hash of the triggering resource
- Used for tracking which resource caused the reload

**Code Location:**
- Constant: `internal/pkg/util/helpers.go:34` - `AnnotationResourceHash`
- **Defined but not clearly used**

---

### 7.4 `reloader.stakater.com/last-reloaded-from`

**Applied to:** Deployment, StatefulSet, DaemonSet (auto-set by original Reloader)
**Value:** JSON metadata about the reload trigger
**Status:** ❌ **Missing**

**What it does:**
- Contains detailed JSON metadata about what triggered the reload
- Includes: resource kind, name, namespace, timestamp, strategy used

**Example:**
```json
{
  "kind": "Secret",
  "name": "db-credentials",
  "namespace": "production",
  "triggeredAt": "2025-11-05T10:30:00Z",
  "strategy": "annotations"
}
```

**Code Location:**
- **Not defined in constants**
- **Not implemented**

---

## 8. Summary Table

| Annotation | Applied To | Status | Priority to Fix |
|------------|-----------|--------|----------------|
| `reloader.stakater.com/auto` | Workload | ⚠️ Partial | **HIGH** |
| `secret.reloader.stakater.com/auto` | Workload | ⚠️ Partial | **HIGH** |
| `configmap.reloader.stakater.com/auto` | Workload | ⚠️ Partial | **HIGH** |
| `secret.reloader.stakater.com/reload` | Workload | ⚠️ Partial (no regex) | **MEDIUM** |
| `configmap.reloader.stakater.com/reload` | Workload | ⚠️ Partial (no regex) | **MEDIUM** |
| `reloader.stakater.com/search` | Workload | ❌ Missing | **MEDIUM** |
| `reloader.stakater.com/match` | ConfigMap/Secret | ❌ Missing | **MEDIUM** |
| `reloader.stakater.com/ignore` | ConfigMap/Secret | ⚠️ Partial | **HIGH** |
| `configmaps.exclude.reloader.stakater.com/reload` | Workload | ❌ Missing | **MEDIUM** |
| `secrets.exclude.reloader.stakater.com/reload` | Workload | ❌ Missing | **MEDIUM** |
| `reloader.stakater.com/rollout-strategy` | Workload | ✅ Implemented | **LOW** |
| `deployment.reloader.stakater.com/pause-period` | Deployment | 🐛 Broken | **CRITICAL** |
| `statefulset.reloader.stakater.com/pause-period` | StatefulSet | 🐛 Broken | **CRITICAL** |
| `daemonset.reloader.stakater.com/pause-period` | DaemonSet | 🐛 Broken | **CRITICAL** |
| `deployment.reloader.stakater.com/paused-at` | Deployment | ❌ Missing | **LOW** |
| `reloader.stakater.com/last-hash` | ConfigMap/Secret | ✅ Implemented | - |
| `reloader.stakater.com/last-reload` | Workload | ⚠️ Partial | **LOW** |
| `reloader.stakater.com/resource-hash` | Workload | ⚠️ Partial | **LOW** |
| `reloader.stakater.com/last-reloaded-from` | Workload | ❌ Missing | **LOW** |

---

## 9. Implementation Statistics

### By Status
- ✅ **Fully Implemented:** 2 annotations (10%)
- ⚠️ **Partially Implemented:** 8 annotations (42%)
- 🐛 **Broken:** 3 annotations (16%)
- ❌ **Missing:** 6 annotations (32%)

### By Priority
- **CRITICAL:** 3 annotations (pause period bug)
- **HIGH:** 4 annotations (auto annotations, ignore)
- **MEDIUM:** 5 annotations (named reload, search/match, exclusions)
- **LOW:** 7 annotations (tracking, metadata, restart strategy)

---

## 10. Quick Reference: Which Annotation Should I Use?

### Use Case: Auto-reload everything
```yaml
reloader.stakater.com/auto: "true"
```
**Status:** ⚠️ Requires ReloaderConfig

---

### Use Case: Reload only when specific Secrets change
```yaml
secret.reloader.stakater.com/reload: "db-creds,api-keys"
```
**Status:** ⚠️ Works, but no regex support

---

### Use Case: Auto-reload Secrets only, ignore ConfigMaps
```yaml
secret.reloader.stakater.com/auto: "true"
```
**Status:** ⚠️ Requires ReloaderConfig

---

### Use Case: Prevent reload storms
```yaml
deployment.reloader.stakater.com/pause-period: "5m"
```
**Status:** 🐛 Broken - being fixed

---

### Use Case: Exclude specific ConfigMaps from auto-reload
```yaml
reloader.stakater.com/auto: "true"
configmaps.exclude.reloader.stakater.com/reload: "static-config"
```
**Status:** ❌ Not implemented yet

---

### Use Case: Prevent a ConfigMap from ever triggering reload
On the ConfigMap itself:
```yaml
reloader.stakater.com/ignore: "true"
```
**Status:** ⚠️ Only works on ReloaderConfig, not on ConfigMap

---

### Use Case: GitOps-friendly reload (avoid ArgoCD drift detection)
```yaml
reloader.stakater.com/rollout-strategy: "annotations"
```
**Status:** ✅ Works

---

## 11. Migration Notes

### From Original Reloader to Reloader-Operator

**What works without changes:**
- ✅ Named reload annotations (`secret.reloader.stakater.com/reload`)
- ✅ Rollout strategy annotation - `"rollout"` value supported via alias
- ✅ Restart strategy - works identically (`"restart"` value)
- ✅ All reload strategies fully backward compatible
- ✅ Basic functionality

**What requires changes:**
- ⚠️ Auto annotations need ReloaderConfig CRD created (but work once CRD exists)
- ⚠️ Regex patterns in reload lists won't work (yet)
- ⚠️ Ignore annotation on ConfigMaps/Secrets won't work (yet)
- ⚠️ Pause periods won't work (bug in progress)

**Enhanced features (not in original Reloader):**
- ✅ `"annotations"` strategy - GitOps-friendly pod template annotation updates
- ✅ CRD-based configuration for advanced scenarios
- ✅ Enhanced status tracking and observability

**What to avoid:**
- 🐛 Don't rely on pause period annotations (broken)
- ❌ Don't use search/match annotations (not implemented)
- ❌ Don't use exclusion annotations (not implemented)

---

## 12. For Developers: Code Locations

All annotation constants are defined in:
- **File:** `internal/pkg/util/helpers.go`
- **Lines:** 26-48

Annotation checking logic:
- **File:** `internal/pkg/workload/finder.go`
- **Function:** `shouldReloadFromAnnotations()` (line 320)
- **Function:** `FindWorkloadsWithAnnotations()` (line 222)

CRD-based configuration:
- **File:** `api/v1alpha1/reloaderconfig_types.go`
- **Spec:** Lines 27-70

Controller logic:
- **File:** `internal/controller/reloaderconfig_controller.go`
- **Secret reconcile:** Line 366
- **ConfigMap reconcile:** Line 418

---

## 13. Contributing

If you're implementing missing annotations:

1. Add constant to `internal/pkg/util/helpers.go`
2. Add parsing logic if needed (e.g., comma-separated, regex)
3. Add check in `internal/pkg/workload/finder.go` - `shouldReloadFromAnnotations()`
4. Add tests in `internal/pkg/workload/finder_test.go`
5. Add E2E test in `test/e2e/annotation_test.go`
6. Update this document with ✅ status

---

**Document Version:** 1.0
**Maintainer:** Reloader-Operator Team
**Next Review:** After annotation implementation sprint
