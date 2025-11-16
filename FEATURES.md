# Reloader Operator - Feature Documentation

**Last Updated**: 2025-11-16

This document provides detailed documentation for all features implemented in the Reloader Operator.

## Table of Contents

1. [Configuration Methods](#configuration-methods)
2. [Command-Line Flags](#command-line-flags)
3. [Reload Strategies](#reload-strategies)
4. [Filtering Features](#filtering-features)
5. [Reload Triggers](#reload-triggers)
6. [Usage Examples](#usage-examples)

---

## Configuration Methods

Reloader Operator supports two configuration methods that can be used independently or together:

### 1. Annotation-Based Configuration

The simplest way to configure reloading. Just add annotations to your Deployment, StatefulSet, or DaemonSet.

**Pros:**
- Simple and straightforward
- No additional resources needed
- 100% backward compatible with original Reloader

**Cons:**
- Limited to basic features
- Configuration scattered across multiple resources

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
spec:
  # ... deployment spec
```

### 2. CRD-Based Configuration (ReloaderConfig)

Advanced configuration using a custom resource definition.

**Pros:**
- Centralized configuration
- Advanced features (alerts, complex selectors)
- Better for GitOps workflows
- Declarative and version-controlled

**Cons:**
- Requires creating additional resources
- More complex for simple use cases

**Example:**
```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
spec:
  watchedResources:
    configMaps: [app-config]
    secrets: [db-credentials]
  targets:
    - kind: Deployment
      name: my-app
  reloadStrategy: env-vars
```

---

## Command-Line Flags

Configure the operator's global behavior using command-line flags.

### Resource Filtering

#### `--resource-label-selector`

**Type:** String (label selector expression)
**Default:** (none - watch all resources)
**Purpose:** Filter which ConfigMaps and Secrets to watch based on their labels

**Examples:**

Watch only resources with a specific label:
```bash
--resource-label-selector=managed-by=my-team
```

Watch resources matching multiple labels:
```bash
--resource-label-selector=environment=production,team=backend
```

Watch resources with label in a set:
```bash
--resource-label-selector='environment in (production,staging)'
```

**Use Case:**
In a multi-tenant cluster, only watch resources managed by your team:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --resource-label-selector=managed-by=platform-team
```

Now only ConfigMaps/Secrets with `managed-by=platform-team` label will trigger reloads.

---

### Namespace Filtering

#### `--namespace-selector`

**Type:** String (label selector expression)
**Default:** (none - watch all namespaces)
**Purpose:** Watch only namespaces that match the label selector

**Examples:**

Watch only production namespaces:
```bash
--namespace-selector=environment=production
```

Watch namespaces for specific teams:
```bash
--namespace-selector='team in (backend,frontend)'
```

**Use Case:**
Separate operators for different environments:
```yaml
# Production operator
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-production
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespace-selector=environment=production
---
# Development operator
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-development
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespace-selector=environment=development
```

#### `--namespaces-to-ignore`

**Type:** String (comma-separated namespace names)
**Default:** (none - don't ignore any namespaces)
**Purpose:** Explicitly ignore specific namespaces

**Examples:**

Ignore system namespaces:
```bash
--namespaces-to-ignore=kube-system,kube-public,kube-node-lease
```

**Use Case:**
Prevent reloading system workloads:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespaces-to-ignore=kube-system,kube-public,ingress-nginx
```

**Note:** This is checked AFTER `--namespace-selector`. If a namespace matches the selector but is in the ignore list, it will be ignored.

---

### Reload Triggers

#### `--reload-on-create`

**Type:** Boolean
**Default:** `false`
**Purpose:** Trigger reload when watched ConfigMaps/Secrets are created (not just updated)

**Use Case:**
You deploy a workload before the ConfigMap exists. When the ConfigMap is created, the workload should reload to pick up the new values.

**Example:**

1. Deploy with flag enabled:
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --reload-on-create=true
```

2. Deploy workload with missing ConfigMap:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    configmap.reloader.stakater.com/reload: "app-config"
spec:
  template:
    spec:
      containers:
      - name: app
        env:
        - name: CONFIG
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: data
              optional: true  # ConfigMap doesn't exist yet
```

3. Create the ConfigMap:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
data:
  data: "production-value"
```

Result: The deployment will automatically reload and pods will restart to pick up the new ConfigMap.

#### `--reload-on-delete`

**Type:** Boolean
**Default:** `false`
**Purpose:** Trigger reload when watched ConfigMaps/Secrets are deleted

**Use Case:**
When a ConfigMap is deleted, trigger a reload so the workload can fall back to default values or handle the absence gracefully.

**Example:**
```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --reload-on-delete=true
```

**Note:** Your workload must handle missing ConfigMaps/Secrets gracefully (use `optional: true` in references).

---

### Other Flags

#### `--metrics-bind-address`

**Type:** String (address:port)
**Default:** `:8080`
**Purpose:** Address for Prometheus metrics endpoint

**Example:**
```bash
--metrics-bind-address=:9090
```

Access metrics at `http://localhost:9090/metrics`

#### `--health-probe-bind-address`

**Type:** String (address:port)
**Default:** `:8081`
**Purpose:** Address for health probe endpoints (/healthz, /readyz)

**Example:**
```bash
--health-probe-bind-address=:9091
```

#### `--leader-elect`

**Type:** Boolean
**Default:** `false`
**Purpose:** Enable leader election for high availability

**Example:**
```bash
--leader-elect=true
```

**Use Case:**
Run multiple operator replicas for HA. Only the leader will reconcile resources.

---

## Reload Strategies

Control how the operator triggers pod restarts.

### `env-vars` (Default)

**How it works:**
Adds/updates an environment variable in the pod template:
```yaml
env:
- name: RELOADER_TRIGGERED_AT
  value: "2025-11-16T10:30:00Z"
```

**Pros:**
- Works with all Kubernetes versions
- Reliable pod restart trigger
- Clear audit trail (can see when reload happened)

**Cons:**
- Changes pod template spec
- May trigger ArgoCD/Flux drift detection

**Configuration:**
```yaml
metadata:
  annotations:
    reloader.stakater.com/rollout-strategy: "env-vars"
```

### `annotations`

**How it works:**
Updates a pod template annotation:
```yaml
template:
  metadata:
    annotations:
      reloader.stakater.com/last-reload: "2025-11-16T10:30:00Z"
```

**Pros:**
- GitOps-friendly (ArgoCD/Flux can ignore annotation changes)
- Cleaner pod spec
- Clear audit trail

**Cons:**
- Changes pod template (just annotations, not spec)

**Configuration:**
```yaml
metadata:
  annotations:
    reloader.stakater.com/rollout-strategy: "annotations"
```

**ArgoCD Configuration:**
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
spec:
  ignoreDifferences:
  - group: apps
    kind: Deployment
    jsonPointers:
    - /spec/template/metadata/annotations/reloader.stakater.com~1last-reload
```

### `restart`

**How it works:**
Deletes pods directly without changing the pod template (like `kubectl rollout restart`).

**Pros:**
- Most GitOps-friendly (no template changes)
- Clean approach

**Cons:**
- Kubernetes recreates pods with updated ConfigMap/Secret data
- No audit trail in pod template

**Configuration:**
```yaml
metadata:
  annotations:
    reloader.stakater.com/rollout-strategy: "restart"
```

---

## Filtering Features

### Combined Filtering Example

You can combine multiple filtering options:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-operator-controller-manager
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespace-selector=environment=production
        - --namespaces-to-ignore=production-system
        - --resource-label-selector=managed-by=my-team
```

**Result:**
- ✅ Watch namespaces with `environment=production` label
- ❌ Except `production-system` namespace (even if it has the label)
- ✅ Only watch ConfigMaps/Secrets with `managed-by=my-team` label

### Filtering Logic

```
For each ConfigMap/Secret change:
  1. Check if resource has required labels (--resource-label-selector)
     ❌ No match → Ignore

  2. Check if namespace is in ignore list (--namespaces-to-ignore)
     ❌ In list → Ignore

  3. Check if namespace matches selector (--namespace-selector)
     ❌ No match → Ignore

  ✅ All checks passed → Process the change
```

---

## Usage Examples

### Example 1: Multi-Environment Setup

Separate operators for different environments:

```yaml
# production-operator.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-production
  namespace: reloader-system
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespace-selector=environment=production
        - --resource-label-selector=managed-by=platform-team
        - --reload-on-create=false
        - --reload-on-delete=false
---
# staging-operator.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-staging
  namespace: reloader-system
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespace-selector=environment=staging
        - --resource-label-selector=managed-by=platform-team
        - --reload-on-create=true
        - --reload-on-delete=true
```

### Example 2: Team-Based Isolation

Each team manages their own resources:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-backend-team
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --namespace-selector=team=backend
        - --resource-label-selector=managed-by=backend-team
```

### Example 3: GitOps-Friendly Configuration

Use annotations strategy to avoid ArgoCD drift:

```yaml
# Deployment with annotation strategy
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    reloader.stakater.com/rollout-strategy: "annotations"
spec:
  template:
    spec:
      containers:
      - name: app
        image: myapp:v1
        envFrom:
        - configMapRef:
            name: app-config
```

### Example 4: Selective Reloading with Search & Match

Only reload when specific ConfigMaps change:

```yaml
# Workload with search mode
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
            name: app-config        # Only reloads if has match annotation
        - configMapRef:
            name: static-config     # Changes ignored
---
# ConfigMap that triggers reload
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  annotations:
    reloader.stakater.com/match: "true"  # This will trigger reload
data:
  key: value
---
# ConfigMap that doesn't trigger reload
apiVersion: v1
kind: ConfigMap
metadata:
  name: static-config
  # No match annotation - changes won't trigger reload
data:
  key: value
```

---

## Best Practices

### 1. Use Namespace Filtering in Multi-Tenant Clusters

```yaml
--namespace-selector=team=my-team
```

Prevents operators from different teams from interfering with each other.

### 2. Ignore System Namespaces

```yaml
--namespaces-to-ignore=kube-system,kube-public,kube-node-lease
```

Prevents unnecessary processing of system namespaces.

### 3. Use Label Selectors for Large Clusters

```yaml
--resource-label-selector=reload-enabled=true
```

Explicitly tag resources that should trigger reloads to reduce watch overhead.

### 4. Enable Reload on Create for Dynamic Environments

```yaml
--reload-on-create=true
```

Useful in CI/CD pipelines where ConfigMaps might be created after deployments.

### 5. Use Annotations Strategy for GitOps

```yaml
reloader.stakater.com/rollout-strategy: "annotations"
```

Prevents ArgoCD/Flux from detecting drift on every reload.

### 6. Combine Filtering for Fine-Grained Control

```yaml
--namespace-selector=environment=production
--namespaces-to-ignore=production-system
--resource-label-selector=team=backend
```

Maximum control over what triggers reloads.

---

## Troubleshooting

### Reload Not Triggered

**Check:**
1. Does the namespace match `--namespace-selector`?
2. Is the namespace in `--namespaces-to-ignore`?
3. Does the ConfigMap/Secret have required labels (`--resource-label-selector`)?
4. Is the annotation correct on the workload?
5. Is `--reload-on-create` enabled if you're creating new resources?

### Too Many Reloads

**Solutions:**
1. Use `--resource-label-selector` to filter resources
2. Use search & match mode for selective reloading
3. Add pause period (when bug is fixed)
4. Use `--namespaces-to-ignore` to exclude namespaces

### GitOps Drift Detection

**Solution:**
Use `annotations` reload strategy and configure GitOps tool to ignore the annotation:

```yaml
reloader.stakater.com/rollout-strategy: "annotations"
```

---

## Related Documentation

- [README.md](README.md) - Getting started guide
- [ANNOTATION_REFERENCE.md](ANNOTATION_REFERENCE.md) - Complete annotation documentation
- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) - Implementation status
- [CRD_SCHEMA.md](docs/CRD_SCHEMA.md) - ReloaderConfig CRD reference

---

**Version**: 1.0
**Maintained by**: Reloader Operator Team
**Last Updated**: 2025-11-16
