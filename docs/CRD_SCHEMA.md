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
| `reloadStrategy` | string | No | `env-vars` | How to trigger rolling updates (`env-vars` or `annotations`) |
| `autoReloadAll` | boolean | No | `false` | Automatically reload on any referenced resource change |
| `ignoreResources` | [][ResourceReference](#resourcereference) | No | - | Resources to ignore even if they match watch criteria |
| `alerts` | [AlertConfiguration](#alertconfiguration) | No | - | Alert settings for reload notifications |
| `matchLabels` | map[string]string | No | - | Label-based matching for resources |

### WatchedResources

Defines which ConfigMaps and Secrets to monitor for changes.

| Field | Type | Description |
|-------|------|-------------|
| `secrets` | []string | List of Secret names to watch |
| `configMaps` | []string | List of ConfigMap names to watch |
| `namespaceSelector` | [LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#labelselector-v1-meta) | Watch resources across namespaces matching labels |
| `resourceSelector` | [LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.28/#labelselector-v1-meta) | Filter resources by labels |

### TargetWorkload

Defines a workload that should be reloaded.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | Workload type: `Deployment`, `StatefulSet`, `DaemonSet`, `DeploymentConfig`, `Rollout`, `CronJob` |
| `name` | string | Yes | Name of the workload |
| `namespace` | string | No | Namespace (defaults to ReloaderConfig's namespace) |
| `reloadStrategy` | string | No | Override global reload strategy for this workload |
| `pausePeriod` | string | No | Duration to prevent multiple reloads (e.g., `5m`, `1h`) |

### ResourceReference

Identifies a specific Kubernetes resource.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | `Secret` or `ConfigMap` |
| `name` | string | Yes | Resource name |
| `namespace` | string | No | Resource namespace |

### AlertConfiguration

Configures alerting when reloads occur.

| Field | Type | Description |
|-------|------|-------------|
| `slack` | [WebhookConfig](#webhookconfig) | Slack webhook configuration |
| `teams` | [WebhookConfig](#webhookconfig) | Microsoft Teams webhook |
| `googleChat` | [WebhookConfig](#webhookconfig) | Google Chat webhook |
| `customWebhook` | [WebhookConfig](#webhookconfig) | Custom webhook endpoint |

### WebhookConfig

Webhook endpoint configuration.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes* | Webhook URL (*unless `secretRef` is used) |
| `secretRef` | [SecretReference](#secretreference) | No | Reference to Secret containing URL |

### SecretReference

Reference to a Secret containing sensitive data.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | Yes | - | Secret name |
| `key` | string | No | `url` | Key in the Secret |
| `namespace` | string | No | - | Secret namespace (defaults to ReloaderConfig's namespace) |

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

## Reload Strategies

### env-vars (Default)

Updates a dummy environment variable with the resource hash to trigger pod restart.

**Pros:**
- Works with all Kubernetes versions
- Simple and reliable
- Immediate effect

**Cons:**
- Modifies pod spec with metadata
- Less GitOps-friendly

### annotations

Updates pod template annotations instead of environment variables.

**Pros:**
- GitOps-friendly (ArgoCD, Flux ignore annotation changes)
- Cleaner pod spec
- Better for declarative workflows

**Cons:**
- Requires annotation support in workload controller

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
