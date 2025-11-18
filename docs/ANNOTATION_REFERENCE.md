# Reloader Annotation Reference Guide

**Last Updated:** 2025-11-17
**Version:** 2.0
**Purpose:** Complete reference of all Reloader annotations with current implementation status

**Supported Workload Types:** Deployment, StatefulSet, DaemonSet
**Note:** CronJob, Argo Rollout, and OpenShift DeploymentConfig are defined but not yet implemented

---

## Quick Reference

### Status Legend

- ✅ **Implemented** - Fully working and tested
- ⚠️ **Partial** - Works with limitations
- ❌ **Not Implemented** - Defined but not functional
- 📝 **Auto-Set** - Set automatically by operator (read-only)

### Annotation Priority (Reload Logic)

When multiple annotations are present, they are evaluated in this order:

1. **`reloader.stakater.com/auto: "false"`** → Explicitly disabled, skip all other checks
2. **`reloader.stakater.com/auto: "true"`** → Auto-reload all referenced resources
3. **Type-specific auto** (`secret.reloader.stakater.com/auto`, `configmap.reloader.stakater.com/auto`)
4. **Named reload** (`secret.reloader.stakater.com/reload`, `configmap.reloader.stakater.com/reload`)
5. **Search & Match** (`reloader.stakater.com/search` + `reloader.stakater.com/match`)

---

## Table of Contents

1. [Auto-Reload Annotations](#1-auto-reload-annotations)
2. [Named Resource Reload Annotations](#2-named-resource-reload-annotations)
3. [Search and Match Annotations](#3-search-and-match-annotations)
4. [Ignore Annotations](#4-ignore-annotations)
5. [Strategy Annotations](#5-strategy-annotations)
6. [Pause Period Annotations](#6-pause-period-annotations)
7. [Tracking Annotations (Auto-Set)](#7-tracking-annotations-auto-set)
8. [Summary Table](#8-summary-table)
9. [Migration from Original Reloader](#9-migration-from-original-reloader)

---

## 1. Auto-Reload Annotations

### 1.1 `reloader.stakater.com/auto`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"` or `"false"`
**Status:** ✅ **Implemented**
**Priority:** Highest (checked first)

**What it does:**
- Automatically watches ALL ConfigMaps and Secrets referenced in the workload's pod spec
- Triggers reload when ANY of them change
- Searches in: volumes, volumeMounts, env, envFrom
- Can be explicitly disabled with `"false"`

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
spec:
  template:
    spec:
      containers:
      - name: app
        image: myapp:latest
        envFrom:
        - configMapRef:
            name: app-config  # Watched automatically
        - secretRef:
            name: db-creds    # Watched automatically
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
      volumes:
      - name: tls
        secret:
          secretName: tls-cert  # Watched automatically
```

**When to use:**
- ✅ Simple deployments with few ConfigMaps/Secrets
- ✅ When you want all referenced resources to trigger reloads
- ❌ When you only want specific resources to trigger reloads (use named reload instead)

**Implementation:**
- Code: `internal/pkg/workload/finder.go:318-326`
- Test: `internal/pkg/workload/finder_test.go`

---

### 1.2 `secret.reloader.stakater.com/auto`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"`
**Status:** ✅ **Implemented**
**Priority:** Second (after general auto)

**What it does:**
- Like `reloader.stakater.com/auto` but ONLY for Secrets
- Ignores ConfigMaps
- Useful when you want to reload on Secret changes but not ConfigMap changes

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
        - secretRef:
            name: db-creds      # ✅ Triggers reload
        - configMapRef:
            name: app-config    # ❌ Does NOT trigger reload
```

**Implementation:**
- Code: `internal/pkg/workload/finder.go:329-333`

---

### 1.3 `configmap.reloader.stakater.com/auto`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"`
**Status:** ✅ **Implemented**
**Priority:** Second (after general auto)

**What it does:**
- Like `reloader.stakater.com/auto` but ONLY for ConfigMaps
- Ignores Secrets
- Useful when you want to reload on ConfigMap changes but not Secret changes

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
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
            name: app-config    # ✅ Triggers reload
        - secretRef:
            name: db-creds      # ❌ Does NOT trigger reload
```

**Implementation:**
- Code: `internal/pkg/workload/finder.go:334-338`

---

## 2. Named Resource Reload Annotations

### 2.1 `secret.reloader.stakater.com/reload`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** Comma-separated Secret names (e.g., `"secret1,secret2,secret3"`)
**Status:** ✅ **Implemented** (exact match only, no regex)
**Priority:** Third

**What it does:**
- Watches ONLY the explicitly named Secrets
- Supports multiple Secrets (comma-separated)
- Exact name matching only (no wildcards or regex)
- Does NOT require the Secret to be referenced in pod spec

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    secret.reloader.stakater.com/reload: "db-credentials,api-keys,tls-cert"
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: db-credentials  # ✅ Triggers reload
              key: password
        # api-keys and tls-cert also trigger reload even if not referenced here
```

**Whitespace handling:**
- ✅ Spaces are trimmed: `"secret1, secret2"` works correctly
- ✅ Empty values ignored: `"secret1,,secret3"` works correctly

**Limitations:**
- ❌ No regex support: `"db-.*"` will NOT match `"db-credentials"`
- ❌ No wildcards: `"db-*"` will NOT work
- ❌ Case-sensitive exact match only

**Implementation:**
- Code: `internal/pkg/workload/finder.go:342-351`
- Parser: `internal/pkg/util/helpers.go:135` (ParseCommaSeparatedList)

---

### 2.2 `configmap.reloader.stakater.com/reload`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** Comma-separated ConfigMap names
**Status:** ✅ **Implemented** (exact match only, no regex)
**Priority:** Third

**What it does:**
- Watches ONLY the explicitly named ConfigMaps
- Same behavior as `secret.reloader.stakater.com/reload` but for ConfigMaps

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    configmap.reloader.stakater.com/reload: "app-config,feature-flags"
spec:
  template:
    spec:
      containers:
      - name: app
        envFrom:
        - configMapRef:
            name: app-config  # ✅ Triggers reload
```

**Implementation:**
- Code: `internal/pkg/workload/finder.go:342-351`

---

## 3. Search and Match Annotations

### 3.1 `reloader.stakater.com/search` (on Workload)

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"`
**Status:** ✅ **Implemented**
**Priority:** Fourth (lowest)

**What it does:**
- Enables "search mode" on the workload
- Workload will ONLY reload if:
  1. Resource has `reloader.stakater.com/match: "true"` annotation
  2. Resource is actually referenced in pod spec
  3. Both conditions must be met

**Use case:**
- Selective reloading in multi-tenant environments
- Only reload when specific ConfigMaps/Secrets are marked as important

**Example - Workload:**
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
            name: app-config
        - configMapRef:
            name: static-config
```

**Implementation:**
- Code: `internal/pkg/workload/finder.go:355-365`

---

### 3.2 `reloader.stakater.com/match` (on ConfigMap/Secret)

**Applied to:** ConfigMap, Secret
**Value:** `"true"`
**Status:** ✅ **Implemented**
**Works with:** `reloader.stakater.com/search`

**What it does:**
- Marks a ConfigMap/Secret as "important"
- Only triggers reload for workloads with `search: "true"`
- Resource must be referenced in the workload

**Example - Resources:**
```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  annotations:
    reloader.stakater.com/match: "true"  # ✅ Triggers reload
data:
  key: value
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: static-config
  # No match annotation - changes ignored even if referenced
data:
  key: value
```

**Result:**
- ✅ Changes to `app-config` → Reload triggered
- ❌ Changes to `static-config` → No reload (no match annotation)

**Implementation:**
- Code: `internal/pkg/workload/finder.go:359`

---

## 4. Ignore Annotations

### 4.1 `reloader.stakater.com/ignore` (on ConfigMap/Secret)

**Applied to:** ConfigMap, Secret
**Value:** `"true"`
**Status:** ✅ **Implemented**

**What it does:**
- GLOBAL ignore flag - prevents ANY workload from reloading when this resource changes
- Works for both CRD-based and annotation-based configurations
- Checked before any reload logic

**Example:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: service-account-token
  annotations:
    reloader.stakater.com/ignore: "true"  # Never triggers reloads
data:
  token: xyz...
```

**Use cases:**
- ✅ Service account tokens
- ✅ CA certificates
- ✅ System-managed resources
- ✅ Static configuration that never changes

**Implementation:**
- Annotation-based check: `internal/pkg/workload/finder.go:72`
- CRD-based check: `internal/controller/reconciler_events.go:44`
- Constant: `internal/pkg/util/helpers.go:32`

**Also supported in CRD:**
```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
spec:
  ignoreResources:
    - kind: Secret
      name: service-account-token
      namespace: default
```

---

### 4.2 `reloader.stakater.com/ignore` (on Workload)

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"true"`
**Status:** ❌ **Not Implemented**

**Note:** This annotation is NOT implemented for workloads. Use ignore on the ConfigMap/Secret instead.

---

## 5. Strategy Annotations

### 5.1 `reloader.stakater.com/rollout-strategy`

**Applied to:** Deployment, StatefulSet, DaemonSet
**Value:** `"rollout"` or `"restart"`
**Status:** ✅ **Implemented**
**Default:** `rollout`

**What it does:**
- Controls HOW the operator triggers pod restarts
- Two strategies:
  - `rollout`: Modify pod template (uses reload strategy)
  - `restart`: Delete pods directly (GitOps-friendly)

**Strategy: `rollout`** (Default)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    reloader.stakater.com/rollout-strategy: "rollout"  # Default
spec:
  # ...
```

**Behavior:**
- Modifies pod template to trigger Kubernetes rolling update
- Uses reload strategy (env-vars or annotations) to modify template
- Changes visible in deployment history
- Standard Kubernetes behavior

**Strategy: `restart`** (GitOps-friendly)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    reloader.stakater.com/rollout-strategy: "restart"
spec:
  # ...
```

**Behavior:**
- Deletes pods directly without modifying template
- Equivalent to `kubectl rollout restart`
- No template changes (ArgoCD/Flux won't detect drift)
- Most GitOps-friendly option

**Implementation:**
- Code: `internal/pkg/util/helpers.go:73-84` (GetDefaultRolloutStrategy)
- Constant: `internal/pkg/util/helpers.go:33`

---

### 5.2 Reload Strategy (env-vars vs annotations)

**Note:** Reload strategy is NOT configured via annotations in annotation-based mode.
It's only available in CRD-based configuration (`spec.reloadStrategy`).

In annotation mode, the operator uses `env-vars` strategy by default when `rolloutStrategy: "rollout"`.

**CRD Example:**
```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
spec:
  rolloutStrategy: rollout  # Use rollout
  reloadStrategy: annotations  # Modify template using annotations (not env vars)
  targets:
    - kind: Deployment
      name: my-app
```

---

## 6. Pause Period Annotations

### 6.1 `deployment.reloader.stakater.com/pause-period`

**Applied to:** Deployment
**Value:** Duration string (e.g., `"5m"`, `"1h"`, `"30s"`)
**Status:** ✅ **Implemented** (Fixed 2025-11-06)

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
  # ...
```

**Timeline example:**
```
10:00 - app-config changes → ✅ Reload triggered, paused until 10:05
10:02 - db-secret changes → ❌ Ignored (still paused)
10:04 - feature-flags changes → ❌ Ignored (still paused)
10:06 - tls-cert changes → ✅ Reload triggered, paused until 10:11
```

**Duration format:**
- Valid: `"5m"`, `"1h"`, `"30s"`, `"2h30m"`, `"90s"`
- Invalid: `"5 minutes"`, `"1 hour"`, `"5"`

**Use cases:**
- ✅ Prevent cascading restarts in microservices
- ✅ Batch multiple ConfigMap/Secret updates
- ✅ Reduce reload frequency in CI/CD pipelines

**Implementation:**
- Code: `internal/pkg/workload/updater.go:501-534` (IsPaused function)
- Uses annotation `reloader.stakater.com/last-reload` to track last reload time
- Constant: `internal/pkg/util/helpers.go:44`

---

### 6.2 `statefulset.reloader.stakater.com/pause-period`

**Applied to:** StatefulSet
**Value:** Duration string
**Status:** ✅ **Implemented** (Fixed 2025-11-06)

**What it does:**
- Same as `deployment.reloader.stakater.com/pause-period` but for StatefulSets

**Example:**
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis
  annotations:
    configmap.reloader.stakater.com/reload: "redis-config"
    statefulset.reloader.stakater.com/pause-period: "10m"
spec:
  # ...
```

**Implementation:**
- Code: `internal/pkg/workload/updater.go:501-534`
- Constant: `internal/pkg/util/helpers.go:45`

---

### 6.3 `daemonset.reloader.stakater.com/pause-period`

**Applied to:** DaemonSet
**Value:** Duration string
**Status:** ✅ **Implemented** (Fixed 2025-11-06)

**What it does:**
- Same as `deployment.reloader.stakater.com/pause-period` but for DaemonSets

**Example:**
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: log-collector
  annotations:
    secret.reloader.stakater.com/reload: "logging-credentials"
    daemonset.reloader.stakater.com/pause-period: "15m"
spec:
  # ...
```

**Implementation:**
- Code: `internal/pkg/workload/updater.go:501-534`
- Constant: `internal/pkg/util/helpers.go:46`

---

## 7. Tracking Annotations (Auto-Set)

These annotations are set automatically by the operator. Do NOT set them manually.

### 7.1 `reloader.stakater.com/last-reload`

**Applied to:** Deployment, StatefulSet, DaemonSet (auto-set)
**Value:** RFC3339 timestamp
**Status:** 📝 **Auto-Set by Operator**

**What it does:**
- Automatically set by operator when workload is reloaded
- Used for pause period calculations
- Shows when the last reload occurred

**Example (auto-set by operator):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    reloader.stakater.com/last-reload: "2025-11-17T10:30:45Z"  # Auto-set
spec:
  # ...
```

**Implementation:**
- Code: `internal/pkg/workload/updater.go:542-557` (setLastReloadAnnotation)
- Constant: `internal/pkg/util/helpers.go:34`

---

### 7.2 `reloader.stakater.com/last-reloaded-from`

**Applied to:** Deployment, StatefulSet, DaemonSet (auto-set)
**Value:** JSON string with reload source
**Status:** 📝 **Auto-Set by Operator**

**What it does:**
- Records which ConfigMap/Secret triggered the reload
- Useful for auditing and debugging

**Example (auto-set by operator):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    reloader.stakater.com/last-reloaded-from: '{"kind":"Secret","name":"db-credentials","namespace":"default","hash":"a1b2c3"}'
```

**Implementation:**
- Code: `internal/pkg/util/helpers.go:115-132` (CreateReloadSourceAnnotation)
- Constant: `internal/pkg/util/helpers.go:35`

---

### 7.3 `reloader.stakater.com/last-hash` (on ConfigMap/Secret)

**Applied to:** ConfigMap, Secret (auto-set)
**Value:** Hash of resource data
**Status:** 📝 **Auto-Set by Operator**

**What it does:**
- Operator sets this to track resource versions
- Used internally to detect changes
- You can see it but should not modify it

**Example (auto-set by operator):**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  annotations:
    reloader.stakater.com/last-hash: "a1b2c3d4e5f6"  # Auto-set by operator
data:
  key: value
```

**Implementation:**
- Constant: `internal/pkg/util/helpers.go:28`

---

## 8. Summary Table

### Workload Annotations

| Annotation | Applied To | Values | Status | Priority |
|------------|------------|--------|--------|----------|
| `reloader.stakater.com/auto` | Deployment/StatefulSet/DaemonSet | `"true"`, `"false"` | ✅ Implemented | Highest |
| `secret.reloader.stakater.com/auto` | Deployment/StatefulSet/DaemonSet | `"true"` | ✅ Implemented | High |
| `configmap.reloader.stakater.com/auto` | Deployment/StatefulSet/DaemonSet | `"true"` | ✅ Implemented | High |
| `secret.reloader.stakater.com/reload` | Deployment/StatefulSet/DaemonSet | Comma-separated names | ✅ Implemented (no regex) | Medium |
| `configmap.reloader.stakater.com/reload` | Deployment/StatefulSet/DaemonSet | Comma-separated names | ✅ Implemented (no regex) | Medium |
| `reloader.stakater.com/search` | Deployment/StatefulSet/DaemonSet | `"true"` | ✅ Implemented | Low |
| `reloader.stakater.com/rollout-strategy` | Deployment/StatefulSet/DaemonSet | `"rollout"`, `"restart"` | ✅ Implemented | - |
| `deployment.reloader.stakater.com/pause-period` | Deployment | Duration (e.g., `"5m"`) | ✅ Implemented | - |
| `statefulset.reloader.stakater.com/pause-period` | StatefulSet | Duration | ✅ Implemented | - |
| `daemonset.reloader.stakater.com/pause-period` | DaemonSet | Duration | ✅ Implemented | - |
| `reloader.stakater.com/last-reload` | Deployment/StatefulSet/DaemonSet | RFC3339 timestamp | 📝 Auto-set | - |
| `reloader.stakater.com/last-reloaded-from` | Deployment/StatefulSet/DaemonSet | JSON string | 📝 Auto-set | - |

### Resource Annotations

| Annotation | Applied To | Values | Status | Notes |
|------------|------------|--------|--------|-------|
| `reloader.stakater.com/match` | ConfigMap/Secret | `"true"` | ✅ Implemented | Works with search mode |
| `reloader.stakater.com/ignore` | ConfigMap/Secret | `"true"` | ✅ Implemented | Global ignore |
| `reloader.stakater.com/last-hash` | ConfigMap/Secret | Hash string | 📝 Auto-set | Internal tracking |

### Not Implemented

| Annotation | Status | Notes |
|------------|--------|-------|
| `configmaps.exclude.reloader.stakater.com/reload` | ❌ Not implemented | Use CRD `ignoreResources` instead |
| `secrets.exclude.reloader.stakater.com/reload` | ❌ Not implemented | Use CRD `ignoreResources` instead |
| Regex/wildcard patterns in reload annotations | ❌ Not implemented | Only exact string matching |

---

## 9. Migration from Original Reloader

### Compatibility Status

**100% backward compatible** with original Reloader annotation-based configuration.

All annotations from original Reloader work exactly the same way, with these exceptions:

**Differences:**

| Feature | Original Reloader | Reloader Operator | Migration |
|---------|------------------|-------------------|-----------|
| Regex patterns in reload lists | ✅ Supported | ❌ Not supported | Use exact names or switch to CRD |
| Exclusion annotations | ✅ Supported | ❌ Not supported | Use `reloader.stakater.com/ignore` or CRD `ignoreResources` |
| CronJob/Rollout/DeploymentConfig | ✅ Supported | ❌ Not supported | Not yet implemented |

**Migration Steps:**

1. **No changes needed** - Most deployments work as-is
2. **If using regex** - Replace with exact names or migrate to CRD
3. **If using exclusions** - Switch to ignore annotation or CRD

**Example migration:**

Original Reloader (with regex):
```yaml
annotations:
  secret.reloader.stakater.com/reload: "db-.*,api-.*"
```

Reloader Operator (exact names):
```yaml
annotations:
  secret.reloader.stakater.com/reload: "db-credentials,db-config,api-keys,api-tokens"
```

Or use CRD for better management:
```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
spec:
  watchedResources:
    secrets:
      - db-credentials
      - db-config
      - api-keys
      - api-tokens
  targets:
    - kind: Deployment
      name: my-app
```

---

## Best Practices

### 1. Use Auto-Reload for Simple Cases
```yaml
annotations:
  reloader.stakater.com/auto: "true"
```

### 2. Use Named Reload for Selective Watching
```yaml
annotations:
  secret.reloader.stakater.com/reload: "db-credentials"
  configmap.reloader.stakater.com/reload: "app-config"
```

### 3. Add Pause Periods to Prevent Reload Storms
```yaml
annotations:
  reloader.stakater.com/auto: "true"
  deployment.reloader.stakater.com/pause-period: "5m"
```

### 4. Use Restart Strategy for GitOps
```yaml
annotations:
  reloader.stakater.com/auto: "true"
  reloader.stakater.com/rollout-strategy: "restart"
```

### 5. Ignore System Resources
```yaml
# On ConfigMap/Secret
annotations:
  reloader.stakater.com/ignore: "true"
```

### 6. For Complex Scenarios, Use CRD
If you need:
- Multiple targets with different strategies
- Cross-namespace watching
- Label-based resource filtering
- Ignore lists with namespaces
- Alert configuration

→ Migrate to ReloaderConfig CRD instead of annotations

---

## Related Documentation

- [README.md](../README.md) - Getting started guide
- [CRD_SCHEMA.md](CRD_SCHEMA.md) - ReloaderConfig CRD reference
- [FEATURES.md](FEATURES.md) - Complete feature documentation
- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) - Implementation status

---

**Version**: 2.0
**Last Updated**: 2025-11-17
**Maintained by**: Reloader Operator Team
