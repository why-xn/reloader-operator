# Reloader Operator Helm Chart

A Kubernetes operator that watches ConfigMaps and Secrets and automatically triggers rolling updates on workloads when they change.

## Overview

The Reloader Operator solves a common Kubernetes problem: workloads don't automatically restart when their ConfigMaps or Secrets are updated. This operator watches for changes and triggers reloads using various strategies.

## Features

- **Flexible Watching**: Watch specific resources by name or use auto-reload for all referenced resources
- **Multiple Reload Strategies**:
  - `rollout`: Modify pod template to trigger rolling update (uses reload strategy)
  - `restart`: Delete pods directly without template changes (GitOps-friendly)
- **Template Modification Strategies** (when rollout strategy is used):
  - `env-vars`: Update resource-specific environment variables
  - `annotations`: Update pod template annotations
- **Label-Based Filtering**: Filter resources by labels
- **Cross-Namespace Watching**: Watch resources across multiple namespaces
- **Targeted Reload**: Only reload workloads that actually reference changed resources
- **Pause Periods**: Prevent cascading reloads with configurable pause periods
- **Alert Integration**: Send alerts to Slack, Teams, Google Chat, or custom webhooks
- **GitOps Compatible**: Works seamlessly with GitOps workflows

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+

## Installation

### Add Helm Repository (if published)

```bash
helm repo add stakater https://stakater.github.io/charts
helm repo update
```

### Install from Local Chart

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace
```

### Install with Custom Values

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace \
  --values my-values.yaml
```

## Configuration

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controllerManager.replicas` | Number of controller replicas | `1` |
| `controllerManager.manager.image.repository` | Container image repository | `stakater/reloader-operator` |
| `controllerManager.manager.image.tag` | Container image tag | `v2.0.0` |
| `controllerManager.manager.image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Operator Arguments

Configure operator behavior through `controllerManager.manager.args`:

```yaml
controllerManager:
  manager:
    args:
      - --metrics-bind-address=:8443
      - --leader-elect
      - --reload-on-create=true
      - --reload-on-delete=false
      - --rollout-strategy=rollout
      - --reload-strategy=env-vars
```

| Argument | Description | Default |
|----------|-------------|---------|
| `--metrics-bind-address` | Metrics server address (`:8443` for HTTPS, `:8080` for HTTP, `0` to disable) | `:8443` |
| `--leader-elect` | Enable leader election for HA | `true` |
| `--health-probe-bind-address` | Health probe endpoint address | `:8081` |
| `--reload-on-create` | Reload when watched resources are created | `false` |
| `--reload-on-delete` | Reload when watched resources are deleted | `false` |
| `--rollout-strategy` | Global rollout strategy: `rollout` or `restart` | `rollout` |
| `--reload-strategy` | Global reload strategy: `env-vars` or `annotations` | `env-vars` |
| `--alert-on-reload` | Enable alerts when reloads occur | `false` |
| `--alert-sink` | Alert destination: `slack`, `teams`, `gchat`, `webhook` | `webhook` |
| `--alert-webhook-url` | Webhook URL for alerts | - |
| `--alert-additional-info` | Additional context for alert messages | - |
| `--resource-label-selector` | Label selector for resources (e.g., `app=myapp`) | - |
| `--namespace-selector` | Namespace label selector | - |
| `--namespaces-to-ignore` | Comma-separated list of namespaces to ignore | - |

### Resource Limits

```yaml
controllerManager:
  manager:
    resources:
      limits:
        cpu: 500m
        memory: 128Mi
      requests:
        cpu: 10m
        memory: 64Mi
```

### Security Configuration

```yaml
controllerManager:
  manager:
    containerSecurityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop:
          - ALL
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      runAsUser: 65532

  podSecurityContext:
    runAsNonRoot: true
    runAsUser: 65532
    fsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
```

### Service Account

```yaml
serviceAccount:
  # Create service account
  create: true
  # Service account annotations (e.g., for IRSA)
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/reloader-operator
  # Service account name
  name: ""
```

### RBAC

```yaml
rbac:
  # Create RBAC resources
  create: true
  annotations: {}
```

### Metrics Service

```yaml
metricsService:
  type: ClusterIP
  ports:
    - name: https
      port: 8443
      protocol: TCP
      targetPort: 8443
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "8443"
    prometheus.io/scheme: "https"
```

### Node Scheduling

```yaml
controllerManager:
  # Node selector
  nodeSelector:
    kubernetes.io/os: linux

  # Tolerations
  tolerations:
    - key: "node-role.kubernetes.io/control-plane"
      operator: "Exists"
      effect: "NoSchedule"

  # Affinity
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                app.kubernetes.io/name: reloader-operator
            topologyKey: kubernetes.io/hostname
```

## Usage Examples

### Example 1: Basic ReloaderConfig

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
  namespace: default
spec:
  watchedResources:
    secrets:
      - db-credentials
    configMaps:
      - app-config

  targets:
    - kind: Deployment
      name: web-app
```

### Example 2: Auto-Reload All Referenced Resources

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: auto-reload
  namespace: production
spec:
  autoReloadAll: true
  rolloutStrategy: restart  # GitOps-friendly

  targets:
    - kind: Deployment
      name: frontend
    - kind: StatefulSet
      name: backend
```

### Example 3: Advanced Configuration

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: advanced
  namespace: staging
spec:
  watchedResources:
    secrets:
      - database-creds
    configMaps:
      - app-config
    resourceSelector:
      matchLabels:
        managed-by: reloader
    enableTargetedReload: true

  rolloutStrategy: rollout
  reloadStrategy: annotations

  targets:
    - kind: Deployment
      name: web-server
      reloadStrategy: annotations
      requireReference: true
      pausePeriod: 5m

    - kind: StatefulSet
      name: cache
      rolloutStrategy: restart
      pausePeriod: 10m

  ignoreResources:
    - kind: Secret
      name: default-token-xyz
```

## Installation Examples

### Production Setup with Alerts

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace \
  --set controllerManager.manager.args[6]="--alert-on-reload=true" \
  --set controllerManager.manager.args[7]="--alert-sink=slack" \
  --set controllerManager.manager.args[8]="--alert-webhook-url=https://hooks.slack.com/..." \
  --set controllerManager.manager.args[9]="--alert-additional-info=Cluster: production"
```

### GitOps-Friendly Setup

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace \
  --set controllerManager.manager.args[6]="--rollout-strategy=restart" \
  --set controllerManager.manager.args[7]="--reload-on-create=true"
```

### High Availability Setup

```bash
helm install reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --create-namespace \
  --set controllerManager.replicas=3 \
  --set controllerManager.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[0].labelSelector.matchLabels.app\.kubernetes\.io/name=reloader-operator \
  --set controllerManager.affinity.podAntiAffinity.requiredDuringSchedulingIgnoredDuringExecution[0].topologyKey=kubernetes.io/hostname
```

## Upgrade

```bash
helm upgrade reloader-operator ./charts/reloader-operator \
  --namespace reloader-operator-system \
  --values my-values.yaml
```

## Uninstall

```bash
helm uninstall reloader-operator --namespace reloader-operator-system
```

**Note**: This will not delete ReloaderConfig CRDs. To remove them:

```bash
kubectl delete crd reloaderconfigs.reloader.stakater.com
```

## Monitoring

The operator exposes Prometheus metrics on the configured metrics endpoint (default: `:8443/metrics`).

### ServiceMonitor Example (Prometheus Operator)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: reloader-operator
  namespace: reloader-operator-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
    - port: https
      scheme: https
      tlsConfig:
        insecureSkipVerify: true
```

## Troubleshooting

### Check Operator Logs

```bash
kubectl logs -n reloader-operator-system -l control-plane=controller-manager -f
```

### Check ReloaderConfig Status

```bash
kubectl get reloaderconfig -A
kubectl describe reloaderconfig <name> -n <namespace>
```

### Verify RBAC Permissions

```bash
kubectl auth can-i get secrets --as=system:serviceaccount:reloader-operator-system:reloader-operator-controller-manager
kubectl auth can-i get configmaps --as=system:serviceaccount:reloader-operator-system:reloader-operator-controller-manager
```

### Check Metrics

```bash
kubectl port-forward -n reloader-operator-system svc/reloader-operator-controller-manager-metrics-service 8443:8443
curl -k https://localhost:8443/metrics
```

## Support

- **Issues**: https://github.com/stakater/Reloader/issues
- **Documentation**: https://github.com/stakater/Reloader
- **Email**: hello@stakater.com

## License

Apache License 2.0 - see LICENSE file for details
