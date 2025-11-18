# Reloader Operator - Feature Documentation

**Last Updated**: 2025-11-17

This document provides detailed documentation for all features implemented in the Reloader Operator.

**Supported Workload Types**: Deployment, StatefulSet, DaemonSet
**Note**: CronJob, Argo Rollout, and OpenShift DeploymentConfig are defined in the CRD but not yet implemented.

## Table of Contents

1. [Configuration Methods](#configuration-methods)
2. [Command-Line Flags](#command-line-flags)
3. [Reload Strategies](#reload-strategies)
4. [Filtering Features](#filtering-features)
5. [Reload Triggers](#reload-triggers)
6. [Ignore/Exclude Features](#ignoreexclude-features)
7. [Alert Integration](#alert-integration)
8. [Deployment Options](#deployment-options)
9. [Usage Examples](#usage-examples)

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
Adds/updates a resource-specific environment variable in the pod template:
```yaml
env:
# For Secret "db-credentials"
- name: STAKATER_DB_CREDENTIALS_SECRET
  value: "a1b2c3d4..."  # hash of the Secret

# For ConfigMap "app-config"
- name: STAKATER_APP_CONFIG_CONFIGMAP
  value: "e5f6g7h8..."  # hash of the ConfigMap
```

**Format:** `STAKATER_<RESOURCE_NAME>_<TYPE>=<hash>`
- Resource names are converted to valid env var names (uppercase, special chars → underscores)
- Value is the resource's hash, not a timestamp
- Each watched resource gets its own environment variable

**Pros:**
- Works with all Kubernetes versions
- Reliable pod restart trigger
- Clear audit trail (can see which specific resource triggered reload)
- Matches original Reloader behavior

**Cons:**
- Changes pod template spec
- May trigger ArgoCD/Flux drift detection
- Adds environment variables (one per watched resource)

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

## Ignore/Exclude Features

The operator supports ignoring specific resources even if they match watch criteria.

### Annotation-Based Ignore

Add the `reloader.stakater.com/ignore: "true"` annotation to a Secret or ConfigMap to exclude it from triggering reloads:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: internal-secret
  annotations:
    reloader.stakater.com/ignore: "true"  # This Secret won't trigger reloads
data:
  key: value
```

**Use Cases:**
- Exclude service account tokens
- Exclude CA certificates
- Exclude operator-managed resources
- Exclude temporary/system resources

### CRD-Based Ignore

Use `spec.ignoreResources` in ReloaderConfig for more complex ignore rules:

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
spec:
  watchedResources:
    secrets:
      - app-secret
      - db-secret
      - internal-secret  # Listed in watchedResources...

  ignoreResources:  # ...but explicitly ignored
    - kind: Secret
      name: internal-secret
      namespace: default
    - kind: ConfigMap
      name: kube-root-ca.crt
```

**Features:**
- Namespace-specific ignore rules
- Works with both `watchedResources` and `autoReloadAll` modes
- Can ignore resources even if they're explicitly watched

---

## Alert Integration

The operator can send alerts when workloads are reloaded. Alerts are configured at the **operator level** using command-line flags.

### Alert Configuration

| Flag | Description | Options |
|------|-------------|---------|
| `--alert-on-reload` | Enable alerts | `true` or `false` (default: `false`) |
| `--alert-sink` | Alert destination type | `slack`, `teams`, `gchat`, `webhook` (default: `webhook`) |
| `--alert-webhook-url` | Webhook URL | URL string |
| `--alert-additional-info` | Extra context in alerts | Any string |

### Supported Alert Sinks

#### 1. Slack

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --alert-on-reload=true
        - --alert-sink=slack
        - --alert-webhook-url=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
        - --alert-additional-info=Cluster: production
```

**Setup:**
1. Create an Incoming Webhook in Slack: https://api.slack.com/messaging/webhooks
2. Copy the webhook URL
3. Configure the operator with the URL

#### 2. Microsoft Teams

```yaml
args:
  - --alert-on-reload=true
  - --alert-sink=teams
  - --alert-webhook-url=https://outlook.office.com/webhook/...
  - --alert-additional-info=Team: Platform
```

**Setup:**
1. Go to your Teams channel
2. Configure Connectors → Incoming Webhook
3. Copy the webhook URL

#### 3. Google Chat

```yaml
args:
  - --alert-on-reload=true
  - --alert-sink=gchat
  - --alert-webhook-url=https://chat.googleapis.com/v1/spaces/.../messages?key=...
  - --alert-additional-info=Environment: Staging
```

**Setup:**
1. Open Google Chat space
2. Manage webhooks → Create webhook
3. Copy the webhook URL

#### 4. Custom Webhook

```yaml
args:
  - --alert-on-reload=true
  - --alert-sink=webhook
  - --alert-webhook-url=https://your-custom-endpoint.com/webhook
  - --alert-additional-info=Custom info
```

Send alerts to any HTTP endpoint that accepts POST requests.

### Alert Message Format

Alerts include:
- Workload kind, name, and namespace
- Resource kind and name that triggered the reload
- Timestamp of the reload
- Additional info (if configured)
- ReloaderConfig name (if applicable)

### Helm Chart Alert Configuration

```yaml
# values.yaml
controllerManager:
  manager:
    args:
      - --alert-on-reload=true
      - --alert-sink=slack
      - --alert-webhook-url=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
      - --alert-additional-info=Cluster: production | Team: Platform
```

---

## Deployment Options

The operator can be deployed using multiple methods.

### 1. Kubectl + Kustomize (Default)

```bash
# Install CRDs
make install

# Deploy operator
make deploy IMG=stakater/reloader-operator:v2.0.0

# Undeploy
make undeploy
```

### 2. Helm Chart (Recommended)

The operator includes a comprehensive Helm chart with production-ready defaults.

#### Install

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace
```

#### Install with Custom Values

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace \
  --values custom-values.yaml
```

#### Example Custom Values

```yaml
# custom-values.yaml
controllerManager:
  replicas: 3  # High availability

  manager:
    image:
      repository: stakater/reloader-operator
      tag: v2.0.0

    args:
      # Enable alerts
      - --alert-on-reload=true
      - --alert-sink=slack
      - --alert-webhook-url=https://hooks.slack.com/...

      # Filter namespaces
      - --namespace-selector=environment=production
      - --namespaces-to-ignore=kube-system,kube-public

      # Reload behavior
      - --reload-on-create=true
      - --reload-on-delete=false
      - --rollout-strategy=restart  # GitOps-friendly

      # Enable metrics
      - --metrics-bind-address=:8443
      - --leader-elect=true

    resources:
      limits:
        cpu: 1000m
        memory: 256Mi
      requests:
        cpu: 100m
        memory: 128Mi

  # Node scheduling
  nodeSelector:
    node-role.kubernetes.io/control-plane: ""

  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule

# Metrics configuration
metricsService:
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8443"
    prometheus.io/scheme: "https"
```

#### Upgrade

```bash
helm upgrade reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --values custom-values.yaml
```

#### Uninstall

```bash
helm uninstall reloader-operator --namespace reloader-operator-system

# Remove CRDs (optional)
kubectl delete crd reloaderconfigs.reloader.stakater.com
```

### 3. Direct Manifest Application

```bash
# Apply all manifests
kubectl apply -f config/crd/bases/reloader.stakater.com_reloaderconfigs.yaml
kubectl apply -f config/rbac/
kubectl apply -f config/manager/
```

### Deployment Comparison

| Method | Pros | Cons | Best For |
|--------|------|------|----------|
| **Helm** | Easy configuration, production-ready, version management | Requires Helm | Production deployments |
| **Kustomize** | Native to Kubebuilder, customizable overlays | Manual configuration | Development, CI/CD |
| **Direct** | Simple, no tools needed | Hard to manage, no templating | Testing, quick trials |

---

## Related Documentation

- [README.md](../README.md) - Getting started guide
- [ANNOTATION_REFERENCE.md](ANNOTATION_REFERENCE.md) - Complete annotation documentation
- [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) - Implementation status
- [CRD_SCHEMA.md](CRD_SCHEMA.md) - ReloaderConfig CRD reference
- [Helm Chart README](../charts/reloader-operator/README.md) - Helm chart documentation

---

**Version**: 2.0
**Maintained by**: Reloader Operator Team
**Last Updated**: 2025-11-17
