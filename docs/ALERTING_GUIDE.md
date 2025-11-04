# Alerting Guide

This guide explains how to configure alerting in Reloader Operator to receive notifications when workloads are reloaded.

## Table of Contents

- [Overview](#overview)
- [Supported Platforms](#supported-platforms)
- [Configuration](#configuration)
- [Webhook URL Management](#webhook-url-management)
- [Alert Message Format](#alert-message-format)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

## Overview

Reloader Operator can send alerts to multiple platforms when workloads are reloaded due to Secret or ConfigMap changes. Alerts are sent for both successful reloads and failures, allowing you to track all reload events across your cluster.

**Features:**
- ✅ Multiple alert channels simultaneously
- ✅ Success and error notifications
- ✅ Secure webhook URL storage via Secrets
- ✅ Rich message formatting with context
- ✅ Concurrent alert delivery
- ✅ Graceful error handling

## Supported Platforms

| Platform | Status | Message Format |
|----------|--------|---------------|
| **Slack** | ✅ Supported | Attachments with color coding |
| **Microsoft Teams** | ✅ Supported | MessageCard with facts |
| **Google Chat** | ✅ Supported | Card with key-value widgets |
| **Custom Webhook** | ✅ Supported | Slack-compatible format |

## Configuration

### Basic Alert Configuration

Add the `alerts` section to your ReloaderConfig:

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-reloader
spec:
  watchedResources:
    secrets:
      - my-secret

  targets:
    - kind: Deployment
      name: my-app

  # Alert configuration
  alerts:
    slack:
      url: "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
```

### Multiple Alert Channels

You can configure multiple alert platforms to receive notifications simultaneously:

```yaml
spec:
  alerts:
    slack:
      secretRef:
        name: webhooks
        key: slack-url

    teams:
      secretRef:
        name: webhooks
        key: teams-url

    googleChat:
      secretRef:
        name: webhooks
        key: gchat-url
```

## Webhook URL Management

### Option 1: Direct URL (Not Recommended)

**Use for:** Development and testing only

```yaml
alerts:
  slack:
    url: "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXX"
```

⚠️ **Warning:** Direct URLs are visible in manifests and should not be used in production.

### Option 2: Secret Reference (Recommended)

**Use for:** Production environments

```yaml
alerts:
  slack:
    secretRef:
      name: slack-webhook
      key: url  # Optional, defaults to "url"
      namespace: default  # Optional, defaults to ReloaderConfig namespace
```

**Create the secret:**

```bash
# Using kubectl
kubectl create secret generic slack-webhook \
  --from-literal=url="https://hooks.slack.com/services/YOUR/WEBHOOK" \
  -n default

# Or using YAML
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: slack-webhook
  namespace: default
type: Opaque
stringData:
  url: "https://hooks.slack.com/services/YOUR/WEBHOOK"
EOF
```

### Centralized Secret Management

Store all webhook URLs in a single secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alert-webhooks
type: Opaque
stringData:
  slack-url: "https://hooks.slack.com/services/..."
  teams-url: "https://outlook.office.com/webhook/..."
  gchat-url: "https://chat.googleapis.com/v1/spaces/..."
---
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-reloader
spec:
  # ...
  alerts:
    slack:
      secretRef:
        name: alert-webhooks
        key: slack-url
    teams:
      secretRef:
        name: alert-webhooks
        key: teams-url
    googleChat:
      secretRef:
        name: alert-webhooks
        key: gchat-url
```

## Alert Message Format

### Success Message

When a workload is successfully reloaded:

**Title:** 🔄 Workload Reloaded
**Color:** Green
**Fields:**
- Workload: `Deployment/my-app`
- Namespace: `production`
- Resource: `Secret/database-password`
- Strategy: `env-vars`
- Time: `2025-10-31T10:30:00Z`

### Error Message

When a reload fails:

**Title:** ❌ Reload Failed
**Color:** Red
**Fields:**
- Workload: `StatefulSet/redis`
- Namespace: `default`
- Resource: `ConfigMap/redis-config`
- Strategy: `annotations`
- Error: `failed to update StatefulSet: timeout waiting for rollout`
- Time: `2025-10-31T10:35:00Z`

## Examples

### Example 1: Slack Alerts Only

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: backend-reloader
  namespace: production
spec:
  watchedResources:
    secrets:
      - database-credentials
      - api-keys

  targets:
    - kind: Deployment
      name: backend-api

  alerts:
    slack:
      secretRef:
        name: slack-webhook
        key: url
```

### Example 2: Microsoft Teams with Pause Period

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: critical-services
  namespace: production
spec:
  watchedResources:
    secrets:
      - service-account

  targets:
    - kind: StatefulSet
      name: database
      pausePeriod: 10m  # Prevent alert spam

  alerts:
    teams:
      url: "https://outlook.office.com/webhook/YOUR/TEAMS/WEBHOOK"
```

### Example 3: Multi-Channel with Auto-Reload

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: auto-reload-with-alerts
  namespace: default
spec:
  # Automatically watch all referenced resources
  autoReloadAll: true

  targets:
    - kind: Deployment
      name: frontend
    - kind: Deployment
      name: backend

  # Send to multiple channels
  alerts:
    slack:
      secretRef:
        name: webhooks
        key: slack
    teams:
      secretRef:
        name: webhooks
        key: teams
    googleChat:
      secretRef:
        name: webhooks
        key: gchat
```

### Example 4: Custom Webhook Integration

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: monitoring-integration
spec:
  watchedResources:
    configMaps:
      - app-config

  targets:
    - kind: Deployment
      name: app

  alerts:
    # Custom webhook uses Slack-compatible format
    customWebhook:
      url: "https://my-monitoring-system.com/api/webhooks/reloader"
```

## Platform-Specific Setup

### Setting Up Slack

1. Go to your Slack workspace
2. Navigate to **Settings & administration** → **Manage apps**
3. Search for **Incoming Webhooks**
4. Click **Add to Slack**
5. Choose a channel and click **Add Incoming Webhooks Integration**
6. Copy the **Webhook URL**
7. Create a Kubernetes secret with the URL

```bash
kubectl create secret generic slack-webhook \
  --from-literal=url="<YOUR_WEBHOOK_URL>" \
  -n <namespace>
```

### Setting Up Microsoft Teams

1. Open Microsoft Teams
2. Navigate to the channel where you want to receive alerts
3. Click **…** (More options) → **Connectors**
4. Search for **Incoming Webhook**
5. Click **Configure**
6. Provide a name and click **Create**
7. Copy the **Webhook URL**
8. Create a Kubernetes secret with the URL

```bash
kubectl create secret generic teams-webhook \
  --from-literal=url="<YOUR_WEBHOOK_URL>" \
  -n <namespace>
```

### Setting Up Google Chat

1. Open Google Chat
2. Go to the space where you want to receive alerts
3. Click the space name → **Manage webhooks**
4. Click **Add webhook**
5. Provide a name and click **Save**
6. Copy the **Webhook URL**
7. Create a Kubernetes secret with the URL

```bash
kubectl create secret generic gchat-webhook \
  --from-literal=url="<YOUR_WEBHOOK_URL>" \
  -n <namespace>
```

## Troubleshooting

### Alerts Not Sent

**Check the operator logs:**

```bash
kubectl logs -n reloader-operator-system \
  deployment/reloader-operator-controller-manager -f
```

**Common issues:**

1. **Secret not found:**
   ```
   Error: Failed to resolve Slack webhook URL: failed to get secret
   ```
   - Verify the secret exists in the correct namespace
   - Check the secret name and key in your ReloaderConfig

2. **Invalid webhook URL:**
   ```
   Error: Slack webhook returned status 404
   ```
   - Verify the webhook URL is correct
   - Test the webhook using curl:
     ```bash
     curl -X POST -H 'Content-Type: application/json' \
       -d '{"text":"Test message"}' \
       <YOUR_WEBHOOK_URL>
     ```

3. **Timeout errors:**
   ```
   Error: failed to send Slack webhook: context deadline exceeded
   ```
   - Check network connectivity from the operator pod
   - Verify firewall rules allow outbound HTTPS traffic

### Partial Alert Delivery

If some alert channels work but others don't:

```bash
# Check logs for specific errors
kubectl logs -n reloader-operator-system \
  deployment/reloader-operator-controller-manager | grep "Failed to send alert"
```

The operator sends alerts concurrently to all channels. If one fails, others will still be delivered.

### Testing Alerts

To test alert configuration without triggering an actual reload:

```bash
# Create a test secret
kubectl create secret generic test-alert-secret \
  --from-literal=key=value1

# Create a test deployment
kubectl create deployment test-alert-app --image=nginx

# Create ReloaderConfig with alerts
kubectl apply -f - <<EOF
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: test-alerts
spec:
  watchedResources:
    secrets:
      - test-alert-secret
  targets:
    - kind: Deployment
      name: test-alert-app
  alerts:
    slack:
      secretRef:
        name: slack-webhook
EOF

# Update the secret to trigger a reload and alert
kubectl create secret generic test-alert-secret \
  --from-literal=key=value2 \
  --dry-run=client -o yaml | kubectl apply -f -

# Watch for reload and check logs
kubectl logs -n reloader-operator-system \
  deployment/reloader-operator-controller-manager -f | grep -i alert
```

Expected log output:
```
INFO Successfully sent alert  sender="Slack"
INFO Successfully triggered reload  kind="Deployment" name="test-alert-app"
```

### Debugging Secret Resolution

If webhook secrets are not being resolved correctly:

```bash
# Verify secret exists and has correct key
kubectl get secret <secret-name> -o jsonpath='{.data.url}' | base64 -d

# Check ReloaderConfig status
kubectl get reloaderconfig <name> -o yaml

# Look for conditions indicating secret resolution issues
```

## Security Best Practices

1. **Always use Secrets** for webhook URLs in production
2. **Use RBAC** to restrict access to webhook secrets
3. **Rotate webhook URLs** periodically
4. **Monitor alert logs** for suspicious activity
5. **Use separate webhooks** for different environments (dev/staging/prod)

## Performance Considerations

- Alerts are sent concurrently to all configured channels
- Each alert sender has a 10-second timeout
- Failed alerts are logged but don't block the reload process
- Use `pausePeriod` to prevent alert storms during frequent changes

## See Also

- [CRD Schema Documentation](CRD_SCHEMA.md) - Full API reference
- [Setup Guide](SETUP_GUIDE.md) - Getting started
- [Examples](../config/samples/) - More configuration examples
