# Reloader Operator Helm Chart

A Kubernetes operator that watches ConfigMaps and Secrets and automatically triggers rolling updates on workloads when they change.

## Features

- 🔄 Automatic reload of workloads when ConfigMaps or Secrets change
- 🎯 Dual configuration support (CRD-based and annotation-based)
- 🔔 Multi-channel alerting (Slack, Microsoft Teams, Google Chat)
- 📊 Prometheus metrics support
- 🛡️ Secure by default with minimal RBAC permissions
- 🚀 High availability with leader election
- 📦 Support for Deployments, StatefulSets, DaemonSets, Argo Rollouts

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installation

### Quick Install

```bash
# Add the Helm repository (if published)
helm repo add stakater https://stakater.github.io/stakater-charts
helm repo update

# Install the chart
helm install reloader-operator stakater/reloader-operator --namespace reloader-system --create-namespace
```

### Install from Local Chart

```bash
# Install from local directory
helm install reloader-operator ./charts/reloader-operator --namespace reloader-system --create-namespace
```

### Install with Custom Values

```bash
# Production installation
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --create-namespace \
  --values ./charts/reloader-operator/values-production.yaml

# Development installation
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --create-namespace \
  --values ./charts/reloader-operator/values-development.yaml
```

### Install with Inline Values

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --create-namespace \
  --set metrics.enabled=true \
  --set serviceMonitor.enabled=true \
  --set operator.logLevel=debug
```

## Upgrading

```bash
# Upgrade to a new version
helm upgrade reloader-operator ./charts/reloader-operator \
  --namespace reloader-system

# Upgrade with new values
helm upgrade reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --values custom-values.yaml
```

## Uninstalling

```bash
# Uninstall the chart
helm uninstall reloader-operator --namespace reloader-system

# Note: CRDs are NOT removed by default
# To remove CRDs manually:
kubectl delete crd reloaderconfigs.reloader.stakater.com
```

## Configuration

### Key Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of operator replicas | `1` |
| `image.repository` | Container image repository | `stakater/reloader-operator` |
| `image.tag` | Container image tag | `""` (uses chart appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `operator.logLevel` | Log level (debug, info, error) | `info` |
| `operator.leaderElection.enabled` | Enable leader election for HA | `true` |
| `operator.watchNamespaces` | Namespaces to watch (empty = all) | `[]` |
| `operator.syncPeriod` | Reconciliation sync period | `10m` |
| `metrics.enabled` | Enable Prometheus metrics | `true` |
| `metrics.port` | Metrics port | `8080` |
| `serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### RBAC Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` (generated) |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `rbac.create` | Create RBAC resources | `true` |
| `rbac.annotations` | RBAC annotations | `{}` |

### Alerting Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `alerts.enabled` | Enable alerting | `false` |
| `alerts.slack.enabled` | Enable Slack alerts | `false` |
| `alerts.slack.webhookURL` | Slack webhook URL (direct) | `""` |
| `alerts.slack.webhookURLSecret.name` | Secret containing webhook URL | `""` |
| `alerts.slack.webhookURLSecret.key` | Key in secret | `""` |
| `alerts.slack.channel` | Slack channel override | `""` |
| `alerts.slack.username` | Slack username | `"Reloader Operator"` |
| `alerts.teams.enabled` | Enable Teams alerts | `false` |
| `alerts.teams.webhookURL` | Teams webhook URL | `""` |
| `alerts.gchat.enabled` | Enable Google Chat alerts | `false` |
| `alerts.gchat.webhookURL` | Google Chat webhook URL | `""` |

### High Availability Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podDisruptionBudget.enabled` | Create PodDisruptionBudget | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods | `1` |
| `autoscaling.enabled` | Enable HPA | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `1` |
| `autoscaling.maxReplicas` | Maximum replicas | `3` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization | `80` |

### Full Values Documentation

See [values.yaml](values.yaml) for all available configuration options.

## Usage Examples

### Example 1: CRD-Based Configuration

Create a ReloaderConfig resource:

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
  namespace: default
spec:
  watchedResources:
    secrets:
      - my-app-secret
    configMaps:
      - my-app-config
  targets:
    - kind: Deployment
      name: my-app
      reloadStrategy: env-vars
```

### Example 2: Annotation-Based Configuration

Annotate your workload:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    secret.reloader.stakater.com/reload: "my-app-secret"
    configmap.reloader.stakater.com/reload: "my-app-config"
spec:
  # ... deployment spec
```

### Example 3: Auto-Reload All Resources

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: namespace-auto-reload
  namespace: production
spec:
  autoReloadAll: true
  targets:
    - kind: Deployment
      selector:
        matchLabels:
          auto-reload: "true"
```

### Example 4: With Alerting

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: critical-app-reloader
  namespace: production
spec:
  watchedResources:
    secrets:
      - database-credentials
  targets:
    - kind: StatefulSet
      name: database
  alerts:
    - type: slack
      webhookURLFrom:
        secretKeyRef:
          name: slack-webhook
          key: url
      channel: "#production-alerts"
```

## Production Deployment

For production deployments, use the production values file:

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --create-namespace \
  --values ./charts/reloader-operator/values-production.yaml \
  --set alerts.slack.webhookURLSecret.name=slack-webhook \
  --set alerts.slack.webhookURLSecret.key=webhook-url
```

### Production Checklist

- [ ] Enable metrics and ServiceMonitor
- [ ] Configure PodDisruptionBudget
- [ ] Set appropriate resource limits
- [ ] Enable leader election (HA)
- [ ] Configure alerting webhooks
- [ ] Set up monitoring and alerts
- [ ] Use specific namespace watching if possible
- [ ] Configure network policies
- [ ] Set priority class for critical workloads

## Monitoring

### Prometheus Metrics

The operator exposes Prometheus metrics at `/metrics`:

```yaml
# Enable metrics
metrics:
  enabled: true
  port: 8080

# Enable ServiceMonitor for Prometheus Operator
serviceMonitor:
  enabled: true
  additionalLabels:
    prometheus: kube-prometheus
  interval: 30s
```

### Available Metrics

- `reloader_reloads_total` - Total number of reloads triggered
- `reloader_reload_errors_total` - Total number of reload errors
- `reloader_watched_resources` - Number of watched resources
- `reloader_last_reload_timestamp` - Timestamp of last reload

## Troubleshooting

### Operator Not Starting

```bash
# Check pod status
kubectl get pods -n reloader-system

# Check logs
kubectl logs -n reloader-system deployment/reloader-operator-controller-manager

# Check events
kubectl get events -n reloader-system
```

### Reloads Not Triggering

1. Check operator logs for errors
2. Verify RBAC permissions
3. Ensure workload is in the same namespace as watched resources
4. Check ReloaderConfig status:
   ```bash
   kubectl get reloaderconfig -o yaml
   ```

### CRD Issues

```bash
# Check if CRD is installed
kubectl get crd reloaderconfigs.reloader.stakater.com

# Reinstall CRDs
kubectl apply -f charts/reloader-operator/crds/
```

## Development

### Testing the Chart

```bash
# Lint the chart
helm lint ./charts/reloader-operator

# Dry run installation
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --dry-run --debug

# Template and review manifests
helm template reloader-operator ./charts/reloader-operator \
  --namespace reloader-system > output.yaml
```

### Local Development

```bash
# Install with development values
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-system \
  --create-namespace \
  --values ./charts/reloader-operator/values-development.yaml

# Watch logs
kubectl logs -n reloader-system deployment/reloader-operator-controller-manager -f
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Reloader Operator                     │
│                                                         │
│  ┌──────────────┐    ┌──────────────┐                 │
│  │   Secret     │───▶│  Controller  │                 │
│  │   Watcher    │    │              │                 │
│  └──────────────┘    │  - Detects   │                 │
│                      │    changes   │                 │
│  ┌──────────────┐    │  - Finds     │                 │
│  │  ConfigMap   │───▶│    targets   │                 │
│  │   Watcher    │    │  - Triggers  │                 │
│  └──────────────┘    │    reloads   │                 │
│                      └──────┬───────┘                 │
│                             │                          │
│                             ▼                          │
│                      ┌──────────────┐                 │
│                      │  Workload    │                 │
│                      │  Updater     │                 │
│                      └──────┬───────┘                 │
└─────────────────────────────┼────────────────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │   Deployments   │
                    │  StatefulSets   │
                    │   DaemonSets    │
                    │  Argo Rollouts  │
                    └─────────────────┘
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](../../CONTRIBUTING.md) for details.

## License

Apache 2.0 License - see [LICENSE](../../LICENSE) for details.

## Support

- GitHub Issues: https://github.com/stakater/Reloader/issues
- Documentation: https://github.com/stakater/Reloader/tree/master/docs
- Website: https://www.stakater.com

## Links

- [Source Code](https://github.com/stakater/Reloader)
- [CRD Schema Documentation](../../docs/CRD_SCHEMA.md)
- [Setup Guide](../../docs/SETUP_GUIDE.md)
- [Alerting Guide](../../docs/ALERTING_GUIDE.md)
