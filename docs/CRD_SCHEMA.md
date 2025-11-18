# ReloaderConfig CRD Schema Documentation

## Overview

The `ReloaderConfig` Custom Resource Definition (CRD) provides a declarative way to configure automatic reloading of Kubernetes workloads when referenced ConfigMaps or Secrets change.

## API Version

- **Group**: `reloader.stakater.com`
- **Version**: `v1alpha1`
- **Kind**: `ReloaderConfig`
- **Short Names**: `rc`, `rlc`

## Schema Reference

### ReloaderConfigSpec

The `spec` section defines the desired behavior of the reloader configuration.

#### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `watchedResources` | [WatchedResources](#watchedresources) | No | - | Specifies which Secrets and ConfigMaps to monitor |
| `targets` | [][TargetWorkload](#targetworkload) | No | - | Workloads to reload when watched resources change |
| `rolloutStrategy` | string | No | `rollout` | How to deploy changes (`rollout` or `restart`) |
| `reloadStrategy` | string | No | `env-vars` | How to modify template when rollout (`env-vars` or `annotations`) |
| `autoReloadAll` | boolean | No | `false` | Automatically reload on any referenced resource change |
| `ignoreResources` | [][ResourceReference](#resourcereference) | No | - | Resources to ignore even if they match watch criteria |
| `matchLabels` | map[string]string | No | - | Label-based matching for resources |

**Note:** Alert configuration is done at the operator level using command-line flags (`--alert-on-reload`, `--alert-sink`, `--alert-webhook-url`), not in the CRD spec.

### WatchedResources

Defines which ConfigMaps and Secrets to monitor for changes.

| Field | Type | Description |
|-------|------|-------------|
| `secrets` | []string | List of Secret names to watch |
| `configMaps` | []string | List of ConfigMap names to watch |
| `enableTargetedReload` | boolean | Enable targeted reload mode (only reload targets with `requireReference=true` that actually reference the changed resource) |
| `namespaceSelector` | [LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#labelselector-v1-meta) | Watch resources across namespaces matching labels |
| `resourceSelector` | [LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#labelselector-v1-meta) | Filter resources by labels |

### TargetWorkload

Defines a workload that should be reloaded.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | Workload type: `Deployment`, `StatefulSet`, `DaemonSet` <br/>**Note:** `CronJob`, `Rollout` (Argo), and `DeploymentConfig` (OpenShift) are defined in the CRD but not yet implemented in the reload logic |
| `name` | string | Yes | Name of the workload |
| `namespace` | string | No | Namespace (defaults to ReloaderConfig's namespace) |
| `rolloutStrategy` | string | No | Override global rollout strategy for this workload (`rollout` or `restart`) |
| `reloadStrategy` | string | No | Override global reload strategy for this workload (`env-vars` or `annotations`) |
| `pausePeriod` | string | No | Duration to prevent multiple reloads (e.g., `5m`, `1h`) |
| `requireReference` | boolean | No | Only reload if workload references the changed resource (works with `enableTargetedReload` in watchedResources) |

### ResourceReference

Identifies a specific Kubernetes resource.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | `Secret` or `ConfigMap` |
| `name` | string | Yes | Resource name |
| `namespace` | string | No | Resource namespace |

## Alert Configuration

**Important:** Alerts are configured at the **operator level** using command-line flags, not in the ReloaderConfig CRD.

### Operator Alert Flags

| Flag | Description | Example |
|------|-------------|---------|
| `--alert-on-reload` | Enable alerts when reloads occur | `--alert-on-reload=true` |
| `--alert-sink` | Alert destination type | `--alert-sink=slack` (options: slack, teams, gchat, webhook) |
| `--alert-webhook-url` | Webhook URL for alerts | `--alert-webhook-url=https://hooks.slack.com/...` |
| `--alert-additional-info` | Additional context in alerts | `--alert-additional-info="Cluster: production"` |

### Example Alert Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: reloader-operator
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --alert-on-reload=true
        - --alert-sink=slack
        - --alert-webhook-url=https://hooks.slack.com/services/YOUR/WEBHOOK/URL
        - --alert-additional-info=Environment: Production
```

See the [Helm chart values](../charts/reloader-operator/values.yaml) for more alert configuration options.

## ReloaderConfigStatus

The `status` section reflects the observed state of the reloader.

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions |
| `lastReloadTime` | Time | Timestamp of most recent reload |
| `watchedResourceHashes` | map[string]string | Current hash of watched resources |
| `reloadCount` | int64 | Total number of reloads triggered |
| `targetStatus` | [][TargetWorkloadStatus](#targetworkloadstatus) | Per-workload reload status |
| `observedGeneration` | int64 | Generation last processed |

### TargetWorkloadStatus

Status of a specific target workload.

| Field | Type | Description |
|-------|------|-------------|
| `kind` | string | Workload kind |
| `name` | string | Workload name |
| `namespace` | string | Workload namespace |
| `lastReloadTime` | Time | When this workload was last reloaded |
| `reloadCount` | int64 | Number of times reloaded |
| `pausedUntil` | Time | When pause period ends |
| `lastError` | string | Error message if last reload failed |

## Strategy System

The operator uses a **two-level strategy system**:

### 1. Rollout Strategy (HOW to deploy)

Controls the deployment mechanism for changes.

#### `rollout` (Default)

Modifies the pod template to trigger a Kubernetes rolling update.

**Pros:**
- Standard Kubernetes behavior
- Changes visible in deployment spec
- Predictable rollout process

**Cons:**
- Modifies deployment template
- Can trigger drift detection in GitOps tools

**When to use:** When you want standard Kubernetes rolling updates and don't mind template modifications.

#### `restart`

Deletes pods directly without modifying the pod template.

**Pros:**
- **Most GitOps-friendly** - no template changes
- No drift detection in ArgoCD/Flux
- Equivalent to `kubectl rollout restart`

**Cons:**
- Pods deleted immediately
- Less visibility in deployment history

**When to use:** When using GitOps tools and you want to avoid template modifications entirely.

### 2. Reload Strategy (HOW to modify template)

Controls how the pod template is modified when using `rollout` rollout strategy. **Ignored when using `restart` rollout strategy.**

#### `env-vars` (Default)

Adds/updates an environment variable with a timestamp.

**Pros:**
- Works with all Kubernetes versions
- Simple and reliable
- Immediate effect

**Cons:**
- Adds metadata to pod spec
- Environment variable pollution

**When to use:** Default choice for standard deployments.

#### `annotations`

Updates pod template annotations instead of environment variables.

**Pros:**
- Cleaner pod spec - no environment variables
- ArgoCD/Flux can ignore annotation changes with proper config
- Better for declarative workflows

**Cons:**
- Requires annotation support
- Still modifies template (less GitOps-friendly than `restart` rollout strategy)

**When to use:** When you want cleaner pod specs but are okay with template modifications.

### Strategy Combinations

| Rollout Strategy | Reload Strategy | Result | GitOps Friendly |
|------------------|-----------------|--------|-----------------|
| `rollout` (default) | `env-vars` (default) | Template modified with env var | ⚠️ No |
| `rollout` | `annotations` | Template modified with annotation | ⚠️ Partial |
| `restart` | (ignored) | Pods deleted directly | ✅ Yes |

**Recommendation for GitOps:** Use `rolloutStrategy: restart` for maximum compatibility with ArgoCD/Flux.

## Mapping to Annotation-Based Configuration

For **100% backward compatibility**, the operator supports both CRD and annotation-based configuration.

### Annotation Format

| Annotation | CRD Equivalent |
|------------|----------------|
| `reloader.stakater.com/auto: "true"` | `spec.autoReloadAll: true` |
| `secret.reloader.stakater.com/reload: "name1,name2"` | `spec.watchedResources.secrets: [name1, name2]` |
| `configmap.reloader.stakater.com/reload: "name1"` | `spec.watchedResources.configMaps: [name1]` |
| `reloader.stakater.com/search: "true"` | Uses `spec.matchLabels` |
| `reloader.stakater.com/match: "true"` | Uses `spec.matchLabels` |
| `deployment.reloader.stakater.com/pause-period: "5m"` | `spec.targets[].pausePeriod: "5m"` |
| `reloader.stakater.com/ignore: "true"` | `spec.ignoreResources[]` |

### Example: Migration from Annotations to CRD

**Old (Annotations):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"
    secret.reloader.stakater.com/reload: "db-creds,api-keys"
    deployment.reloader.stakater.com/pause-period: "5m"
```

**New (CRD):**
```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
spec:
  autoReloadAll: true
  watchedResources:
    secrets:
      - db-creds
      - api-keys
  rolloutStrategy: rollout  # How to deploy (rollout or restart)
  reloadStrategy: env-vars  # How to modify template (env-vars or annotations)
  targets:
    - kind: Deployment
      name: my-app
      pausePeriod: 5m
```

**Note:** Both configurations work simultaneously. The operator merges them with CRD taking precedence.

## kubectl Usage

```bash
# List all ReloaderConfigs
kubectl get reloaderconfigs
kubectl get rc  # short name

# Get detailed info
kubectl get rc my-app-reloader -o yaml

# Check status
kubectl get rc my-app-reloader -o jsonpath='{.status}'

# Watch for changes
kubectl get rc -w

# Output columns show:
# NAME              STRATEGY      TARGETS   RELOADS   LAST RELOAD           AGE
# my-app-reloader   env-vars      2         5         2025-10-30T14:30:00Z  2d
```

## Examples

See the `config/samples/` directory for comprehensive examples:

- **reloader_v1alpha1_reloaderconfig.yaml** - Basic example
- **auto-reload-example.yaml** - Auto-reload mode
- **advanced-example.yaml** - Advanced features

## Best Practices

1. **Use CRDs for new deployments** - More declarative and easier to manage
2. **Keep annotations for backward compatibility** - Gradual migration
3. **Set pause periods** - Prevent cascading restarts in microservices
4. **Use annotations strategy for GitOps** - Better ArgoCD/Flux integration
5. **Leverage alerts** - Monitor reload activity in production
6. **Use label selectors** - Filter resources efficiently at scale
7. **Namespace isolation** - Create ReloaderConfig per namespace for clarity

## Validation

The CRD includes built-in validation:

- ✅ Enum validation for `kind` and `reloadStrategy`
- ✅ Pattern validation for `pausePeriod` (duration format)
- ✅ Required field validation
- ✅ Default values

Invalid configurations are rejected at admission time by the API server.
