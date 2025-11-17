# Reloader Operator - Manual Testing Guide

**Last Updated**: 2025-11-16

This guide provides step-by-step instructions for manually testing all features of the Reloader Operator.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Setup](#setup)
3. [Basic Functionality Tests](#basic-functionality-tests)
4. [Advanced Feature Tests](#advanced-feature-tests)
5. [Filtering Tests](#filtering-tests)
6. [Reload Strategy Tests](#reload-strategy-tests)
7. [Edge Case Tests](#edge-case-tests)
8. [Cleanup](#cleanup)

---

## Prerequisites

Before starting the tests, ensure you have:

1. **Kubernetes cluster** (Kind, Minikube, or real cluster)
2. **kubectl** configured and working
3. **Reloader Operator** deployed and running
4. **Basic tools**: `watch`, `kubectl`, `git`

### Verify Prerequisites

```bash
# Check cluster access
kubectl cluster-info

# Check if operator is running
kubectl get pods -n reloader-operator-system

# Expected output:
# NAME                                                  READY   STATUS    RESTARTS   AGE
# reloader-operator-controller-manager-xxxxxxxxx-xxxxx   2/2     Running   0          5m
```

---

## Setup

### Create Test Namespace

```bash
kubectl create namespace test-reloader
kubectl config set-context --current --namespace=test-reloader
```

### Helper Commands

Add these aliases for easier testing:

```bash
# Alias for watching pods
alias wpods='watch -n 2 kubectl get pods -o wide'

# Alias for getting pod UIDs
alias pod-uids='kubectl get pods -o jsonpath="{range .items[*]}{.metadata.name}: {.metadata.uid}{\"\\n\"}{end}"'

# Alias for checking operator logs
alias op-logs='kubectl logs -n reloader-operator-system -l control-plane=controller-manager -c manager --tail=50 -f'
```

---

## Basic Functionality Tests

### Test 1: Annotation-Based Reload with ConfigMap

**Feature**: `configmap.reloader.stakater.com/reload` annotation

**Steps**:

1. Create a ConfigMap:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: test-reloader
data:
  config.yaml: |
    version: 1.0
    environment: test
EOF
```

2. Create a Deployment with reload annotation:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-app
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "app-config"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-app
  template:
    metadata:
      labels:
        app: test-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: config
          mountPath: /etc/config
      volumes:
      - name: config
        configMap:
          name: app-config
EOF
```

3. Wait for deployment to be ready:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/test-app
```

4. Get initial pod UID:
```bash
INITIAL_UID=$(kubectl get pod -l app=test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Initial Pod UID: $INITIAL_UID"
```

5. Update the ConfigMap:
```bash
kubectl patch configmap app-config -p '{"data":{"config.yaml":"version: 2.0\nenvironment: production\n"}}'
```

6. Watch for pod restart (should happen within 10 seconds):
```bash
watch -n 1 'kubectl get pods -l app=test-app'
```

7. Verify new pod has different UID:
```bash
NEW_UID=$(kubectl get pod -l app=test-app -o jsonpath='{.items[0].metadata.uid}')
echo "New Pod UID: $NEW_UID"
echo "Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ Pod should restart with a new UID

**Cleanup**:
```bash
kubectl delete deployment test-app
kubectl delete configmap app-config
```

---

### Test 2: Annotation-Based Reload with Secret

**Feature**: `secret.reloader.stakater.com/reload` annotation

**Steps**:

1. Create a Secret:
```bash
kubectl create secret generic db-credentials \
  --from-literal=username=admin \
  --from-literal=password=secret123
```

2. Create a Deployment:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-secret-app
  namespace: test-reloader
  annotations:
    secret.reloader.stakater.com/reload: "db-credentials"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-secret-app
  template:
    metadata:
      labels:
        app: test-secret-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: DB_USER
          valueFrom:
            secretKeyRef:
              name: db-credentials
              key: username
        - name: DB_PASS
          valueFrom:
            secretKeyRef:
              name: db-credentials
              key: password
EOF
```

3. Get initial pod UID:
```bash
INITIAL_UID=$(kubectl get pod -l app=test-secret-app -o jsonpath='{.items[0].metadata.uid}')
echo "Initial Pod UID: $INITIAL_UID"
```

4. Update the Secret:
```bash
kubectl create secret generic db-credentials \
  --from-literal=username=admin \
  --from-literal=password=newsecret456 \
  --dry-run=client -o yaml | kubectl apply -f -
```

5. Wait and verify pod restart:
```bash
sleep 10
NEW_UID=$(kubectl get pod -l app=test-secret-app -o jsonpath='{.items[0].metadata.uid}')
echo "New Pod UID: $NEW_UID"
echo "Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ Pod should restart with a new UID

**Cleanup**:
```bash
kubectl delete deployment test-secret-app
kubectl delete secret db-credentials
```

---

### Test 3: CRD-Based Reload

**Feature**: ReloaderConfig custom resource

**Steps**:

1. Create resources:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: feature-flags
  namespace: test-reloader
data:
  enabled: "true"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crd-test-app
  namespace: test-reloader
spec:
  replicas: 1
  selector:
    matchLabels:
      app: crd-test-app
  template:
    metadata:
      labels:
        app: crd-test-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: feature-flags
---
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: crd-test-config
  namespace: test-reloader
spec:
  watchedResources:
    configMaps:
      - feature-flags
  targets:
    - kind: Deployment
      name: crd-test-app
  reloadStrategy: env-vars
EOF
```

2. Wait for deployment:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/crd-test-app
```

3. Get initial pod UID:
```bash
INITIAL_UID=$(kubectl get pod -l app=crd-test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Initial Pod UID: $INITIAL_UID"
```

4. Check ReloaderConfig status:
```bash
kubectl get reloaderconfig crd-test-config -o yaml | grep -A 10 "status:"
```

5. Update ConfigMap:
```bash
kubectl patch configmap feature-flags -p '{"data":{"enabled":"false"}}'
```

6. Verify reload:
```bash
sleep 10
NEW_UID=$(kubectl get pod -l app=crd-test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

7. Check updated ReloaderConfig status:
```bash
kubectl get reloaderconfig crd-test-config -o jsonpath='{.status.reloadCount}'
echo " reload(s)"
```

**Expected Result**:
- ✅ Pod should restart
- ✅ ReloaderConfig status should show reload count incremented

**Cleanup**:
```bash
kubectl delete reloaderconfig crd-test-config
kubectl delete deployment crd-test-app
kubectl delete configmap feature-flags
```

---

### Test 4: Auto-Reload Mode

**Feature**: `reloader.stakater.com/auto` annotation

**Steps**:

1. Create resources:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: auto-config
  namespace: test-reloader
data:
  app.conf: "setting=value1"
---
apiVersion: v1
kind: Secret
metadata:
  name: auto-secret
  namespace: test-reloader
stringData:
  password: "oldpass"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: auto-reload-app
  namespace: test-reloader
  annotations:
    reloader.stakater.com/auto: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: auto-reload-app
  template:
    metadata:
      labels:
        app: auto-reload-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: auto-config
        - secretRef:
            name: auto-secret
EOF
```

2. Wait for deployment:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/auto-reload-app
INITIAL_UID=$(kubectl get pod -l app=auto-reload-app -o jsonpath='{.items[0].metadata.uid}')
```

3. Test ConfigMap update:
```bash
kubectl patch configmap auto-config -p '{"data":{"app.conf":"setting=value2"}}'
sleep 10
NEW_UID=$(kubectl get pod -l app=auto-reload-app -o jsonpath='{.items[0].metadata.uid}')
echo "ConfigMap update - Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

4. Get new UID and test Secret update:
```bash
INITIAL_UID=$(kubectl get pod -l app=auto-reload-app -o jsonpath='{.items[0].metadata.uid}')
kubectl patch secret auto-secret -p '{"stringData":{"password":"newpass"}}'
sleep 10
NEW_UID=$(kubectl get pod -l app=auto-reload-app -o jsonpath='{.items[0].metadata.uid}')
echo "Secret update - Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**:
- ✅ Pod should reload when ConfigMap changes
- ✅ Pod should reload when Secret changes

**Cleanup**:
```bash
kubectl delete deployment auto-reload-app
kubectl delete configmap auto-config
kubectl delete secret auto-secret
```

---

## Advanced Feature Tests

### Test 5: Search & Match Mode

**Feature**: Selective reload using `reloader.stakater.com/search` and `reloader.stakater.com/match`

**Steps**:

1. Create resources:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: watched-config
  namespace: test-reloader
  annotations:
    reloader.stakater.com/match: "true"
data:
  config: "watched"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ignored-config
  namespace: test-reloader
data:
  config: "ignored"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: search-match-app
  namespace: test-reloader
  annotations:
    reloader.stakater.com/search: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: search-match-app
  template:
    metadata:
      labels:
        app: search-match-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: watched-config
        - configMapRef:
            name: ignored-config
EOF
```

2. Wait for deployment:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/search-match-app
INITIAL_UID=$(kubectl get pod -l app=search-match-app -o jsonpath='{.items[0].metadata.uid}')
```

3. Update watched ConfigMap (should trigger reload):
```bash
kubectl patch configmap watched-config -p '{"data":{"config":"watched-updated"}}'
sleep 10
NEW_UID=$(kubectl get pod -l app=search-match-app -o jsonpath='{.items[0].metadata.uid}')
echo "Watched ConfigMap update - Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

4. Get new UID and update ignored ConfigMap (should NOT trigger reload):
```bash
INITIAL_UID=$(kubectl get pod -l app=search-match-app -o jsonpath='{.items[0].metadata.uid}')
kubectl patch configmap ignored-config -p '{"data":{"config":"ignored-updated"}}'
sleep 10
NEW_UID=$(kubectl get pod -l app=search-match-app -o jsonpath='{.items[0].metadata.uid}')
echo "Ignored ConfigMap update - Pod NOT reloaded: $([[ "$INITIAL_UID" == "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**:
- ✅ Pod should reload when watched ConfigMap changes
- ✅ Pod should NOT reload when ignored ConfigMap changes

**Cleanup**:
```bash
kubectl delete deployment search-match-app
kubectl delete configmap watched-config ignored-config
```

---

## Filtering Tests

### Test 6: Resource Label Selector

**Feature**: `--resource-label-selector` flag

**Prerequisites**: Redeploy operator with label selector flag:

```bash
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--resource-label-selector=reload-enabled=true"}]'

# Wait for rollout
kubectl rollout status deployment/reloader-operator-controller-manager -n reloader-operator-system
```

**Steps**:

1. Create ConfigMaps with and without the label:
```bash
kubectl create configmap labeled-config --from-literal=data=value1
kubectl label configmap labeled-config reload-enabled=true

kubectl create configmap unlabeled-config --from-literal=data=value2
```

2. Create deployment:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: label-filter-app
  namespace: test-reloader
  annotations:
    reloader.stakater.com/auto: "true"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: label-filter-app
  template:
    metadata:
      labels:
        app: label-filter-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: labeled-config
        - configMapRef:
            name: unlabeled-config
EOF
```

3. Wait and get initial UID:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/label-filter-app
INITIAL_UID=$(kubectl get pod -l app=label-filter-app -o jsonpath='{.items[0].metadata.uid}')
```

4. Update labeled ConfigMap (should trigger reload):
```bash
kubectl patch configmap labeled-config -p '{"data":{"data":"value1-updated"}}'
sleep 10
NEW_UID=$(kubectl get pod -l app=label-filter-app -o jsonpath='{.items[0].metadata.uid}')
echo "Labeled ConfigMap - Pod reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

5. Update unlabeled ConfigMap (should NOT trigger reload):
```bash
INITIAL_UID=$(kubectl get pod -l app=label-filter-app -o jsonpath='{.items[0].metadata.uid}')
kubectl patch configmap unlabeled-config -p '{"data":{"data":"value2-updated"}}'
sleep 10
NEW_UID=$(kubectl get pod -l app=label-filter-app -o jsonpath='{.items[0].metadata.uid}')
echo "Unlabeled ConfigMap - Pod NOT reloaded: $([[ "$INITIAL_UID" == "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**:
- ✅ Pod should reload for labeled ConfigMap
- ✅ Pod should NOT reload for unlabeled ConfigMap

**Cleanup**:
```bash
kubectl delete deployment label-filter-app
kubectl delete configmap labeled-config unlabeled-config

# Remove the flag from operator
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/args/-"}]'
```

---

### Test 7: Namespace Selector

**Feature**: `--namespace-selector` flag

**Prerequisites**: Create namespaces with labels:

```bash
kubectl create namespace test-prod
kubectl label namespace test-prod environment=production

kubectl create namespace test-dev
kubectl label namespace test-dev environment=development
```

Redeploy operator with namespace selector:

```bash
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--namespace-selector=environment=production"}]'

kubectl rollout status deployment/reloader-operator-controller-manager -n reloader-operator-system
```

**Steps**:

1. Deploy to production namespace (should work):
```bash
kubectl create configmap prod-config --from-literal=data=value1 -n test-prod

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prod-app
  namespace: test-prod
  annotations:
    configmap.reloader.stakater.com/reload: "prod-config"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prod-app
  template:
    metadata:
      labels:
        app: prod-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: prod-config
EOF

kubectl wait --for=condition=available --timeout=60s deployment/prod-app -n test-prod
PROD_UID=$(kubectl get pod -l app=prod-app -n test-prod -o jsonpath='{.items[0].metadata.uid}')

kubectl patch configmap prod-config -n test-prod -p '{"data":{"data":"value1-updated"}}'
sleep 10
NEW_PROD_UID=$(kubectl get pod -l app=prod-app -n test-prod -o jsonpath='{.items[0].metadata.uid}')
echo "Production namespace - Pod reloaded: $([[ "$PROD_UID" != "$NEW_PROD_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

2. Deploy to development namespace (should NOT work):
```bash
kubectl create configmap dev-config --from-literal=data=value1 -n test-dev

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dev-app
  namespace: test-dev
  annotations:
    configmap.reloader.stakater.com/reload: "dev-config"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: dev-app
  template:
    metadata:
      labels:
        app: dev-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: dev-config
EOF

kubectl wait --for=condition=available --timeout=60s deployment/dev-app -n test-dev
DEV_UID=$(kubectl get pod -l app=dev-app -n test-dev -o jsonpath='{.items[0].metadata.uid}')

kubectl patch configmap dev-config -n test-dev -p '{"data":{"data":"value1-updated"}}'
sleep 10
NEW_DEV_UID=$(kubectl get pod -l app=dev-app -n test-dev -o jsonpath='{.items[0].metadata.uid}')
echo "Development namespace - Pod NOT reloaded: $([[ "$DEV_UID" == "$NEW_DEV_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**:
- ✅ Pod in production namespace should reload
- ✅ Pod in development namespace should NOT reload

**Cleanup**:
```bash
kubectl delete namespace test-prod test-dev

# Remove the flag
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/args/-"}]'
```

---

### Test 8: Reload on Create

**Feature**: `--reload-on-create` flag

**Prerequisites**: Enable reload-on-create:

```bash
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--reload-on-create=true"}]'

kubectl rollout status deployment/reloader-operator-controller-manager -n reloader-operator-system
```

**Steps**:

1. Create deployment BEFORE ConfigMap exists:
```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: create-test-app
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "future-config"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: create-test-app
  template:
    metadata:
      labels:
        app: create-test-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: CONFIG_DATA
          valueFrom:
            configMapKeyRef:
              name: future-config
              key: data
              optional: true
EOF
```

2. Wait and get initial UID:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/create-test-app
INITIAL_UID=$(kubectl get pod -l app=create-test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Initial Pod UID: $INITIAL_UID"
```

3. Create the ConfigMap:
```bash
kubectl create configmap future-config --from-literal=data=created
```

4. Verify reload:
```bash
sleep 10
NEW_UID=$(kubectl get pod -l app=create-test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Pod reloaded on ConfigMap creation: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ Pod should reload when ConfigMap is created

**Cleanup**:
```bash
kubectl delete deployment create-test-app
kubectl delete configmap future-config

# Remove flag
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/args/-"}]'
```

---

### Test 9: Reload on Delete

**Feature**: `--reload-on-delete` flag

**Prerequisites**: Enable reload-on-delete:

```bash
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--reload-on-delete=true"}]'

kubectl rollout status deployment/reloader-operator-controller-manager -n reloader-operator-system
```

**Steps**:

1. Create ConfigMap and deployment:
```bash
kubectl create configmap deletable-config --from-literal=data=value1

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: delete-test-app
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "deletable-config"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: delete-test-app
  template:
    metadata:
      labels:
        app: delete-test-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        env:
        - name: CONFIG_DATA
          valueFrom:
            configMapKeyRef:
              name: deletable-config
              key: data
              optional: true
EOF
```

2. Wait, update to track ConfigMap, then get UID:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/delete-test-app

# Update ConfigMap to ensure tracking
kubectl patch configmap deletable-config -p '{"data":{"data":"value2"}}'
sleep 10

INITIAL_UID=$(kubectl get pod -l app=delete-test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Initial Pod UID: $INITIAL_UID"
```

3. Delete the ConfigMap:
```bash
kubectl delete configmap deletable-config
```

4. Verify reload:
```bash
sleep 10
NEW_UID=$(kubectl get pod -l app=delete-test-app -o jsonpath='{.items[0].metadata.uid}')
echo "Pod reloaded on ConfigMap deletion: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ Pod should reload when ConfigMap is deleted

**Cleanup**:
```bash
kubectl delete deployment delete-test-app

# Remove flag
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/args/-"}]'
```

---

## Reload Strategy Tests

### Test 10: env-vars Strategy

**Feature**: Default reload strategy using environment variable injection

**Steps**:

1. Create resources:
```bash
kubectl create configmap strategy-test-config --from-literal=data=value1

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: envvars-strategy-app
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "strategy-test-config"
    reloader.stakater.com/rollout-strategy: "env-vars"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: envvars-strategy-app
  template:
    metadata:
      labels:
        app: envvars-strategy-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: strategy-test-config
EOF
```

2. Wait for deployment:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/envvars-strategy-app
```

3. Update ConfigMap:
```bash
kubectl patch configmap strategy-test-config -p '{"data":{"data":"value2"}}'
```

4. Check for RELOADER_TRIGGERED_AT environment variable:
```bash
sleep 10
kubectl get deployment envvars-strategy-app -o jsonpath='{.spec.template.spec.containers[0].env}' | grep RELOADER_TRIGGERED_AT
echo "env-vars strategy - RELOADER_TRIGGERED_AT found: $([[ $? -eq 0 ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ Should find RELOADER_TRIGGERED_AT environment variable

**Cleanup**:
```bash
kubectl delete deployment envvars-strategy-app
kubectl delete configmap strategy-test-config
```

---

### Test 11: annotations Strategy

**Feature**: GitOps-friendly reload using pod template annotations

**Steps**:

1. Create resources:
```bash
kubectl create configmap annotation-strategy-config --from-literal=data=value1

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: annotation-strategy-app
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "annotation-strategy-config"
    reloader.stakater.com/rollout-strategy: "annotations"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: annotation-strategy-app
  template:
    metadata:
      labels:
        app: annotation-strategy-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: annotation-strategy-config
EOF
```

2. Wait for deployment:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/annotation-strategy-app
```

3. Update ConfigMap:
```bash
kubectl patch configmap annotation-strategy-config -p '{"data":{"data":"value2"}}'
```

4. Check for reload annotation:
```bash
sleep 10
kubectl get deployment annotation-strategy-app -o jsonpath='{.spec.template.metadata.annotations}' | grep "reloader.stakater.com/last-reload"
echo "annotations strategy - last-reload annotation found: $([[ $? -eq 0 ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ Should find reloader.stakater.com/last-reload annotation

**Cleanup**:
```bash
kubectl delete deployment annotation-strategy-app
kubectl delete configmap annotation-strategy-config
```

---

### Test 12: restart Strategy

**Feature**: Pod restart without template changes

**Steps**:

1. Create resources:
```bash
kubectl create configmap restart-strategy-config --from-literal=data=value1

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: restart-strategy-app
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "restart-strategy-config"
    reloader.stakater.com/rollout-strategy: "restart"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: restart-strategy-app
  template:
    metadata:
      labels:
        app: restart-strategy-app
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: restart-strategy-config
EOF
```

2. Get initial state:
```bash
kubectl wait --for=condition=available --timeout=60s deployment/restart-strategy-app
INITIAL_UID=$(kubectl get pod -l app=restart-strategy-app -o jsonpath='{.items[0].metadata.uid}')
INITIAL_GEN=$(kubectl get deployment restart-strategy-app -o jsonpath='{.metadata.generation}')
```

3. Update ConfigMap:
```bash
kubectl patch configmap restart-strategy-config -p '{"data":{"data":"value2"}}'
sleep 10
```

4. Verify pod restarted but template unchanged:
```bash
NEW_UID=$(kubectl get pod -l app=restart-strategy-app -o jsonpath='{.items[0].metadata.uid}')
NEW_GEN=$(kubectl get deployment restart-strategy-app -o jsonpath='{.metadata.generation}')

echo "Pod restarted: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
echo "Template unchanged: $([[ "$INITIAL_GEN" == "$NEW_GEN" ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**:
- ✅ Pod should have new UID (restarted)
- ✅ Deployment generation should be same (template unchanged)

**Cleanup**:
```bash
kubectl delete deployment restart-strategy-app
kubectl delete configmap restart-strategy-config
```

---

## Edge Case Tests

### Test 13: Multiple Workloads Watching Same ConfigMap

**Steps**:

1. Create shared ConfigMap:
```bash
kubectl create configmap shared-config --from-literal=data=value1
```

2. Create multiple deployments:
```bash
for i in 1 2 3; do
  cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: shared-app-$i
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "shared-config"
spec:
  replicas: 1
  selector:
    matchLabels:
      app: shared-app-$i
  template:
    metadata:
      labels:
        app: shared-app-$i
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: shared-config
EOF
done
```

3. Wait for all deployments:
```bash
for i in 1 2 3; do
  kubectl wait --for=condition=available --timeout=60s deployment/shared-app-$i
done
```

4. Get initial UIDs:
```bash
for i in 1 2 3; do
  eval "INITIAL_UID_$i=$(kubectl get pod -l app=shared-app-$i -o jsonpath='{.items[0].metadata.uid}')"
  eval echo "App $i initial UID: \$INITIAL_UID_$i"
done
```

5. Update shared ConfigMap:
```bash
kubectl patch configmap shared-config -p '{"data":{"data":"value2"}}'
sleep 15
```

6. Verify all pods reloaded:
```bash
for i in 1 2 3; do
  NEW_UID=$(kubectl get pod -l app=shared-app-$i -o jsonpath='{.items[0].metadata.uid}')
  eval "INITIAL_UID=\$INITIAL_UID_$i"
  echo "App $i reloaded: $([[ "$INITIAL_UID" != "$NEW_UID" ]] && echo "✅ YES" || echo "❌ NO")"
done
```

**Expected Result**: ✅ All 3 deployments should reload

**Cleanup**:
```bash
for i in 1 2 3; do
  kubectl delete deployment shared-app-$i
done
kubectl delete configmap shared-config
```

---

### Test 14: StatefulSet Reload

**Steps**:

1. Create resources:
```bash
kubectl create configmap statefulset-config --from-literal=data=value1

cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: test-statefulset
  namespace: test-reloader
  annotations:
    configmap.reloader.stakater.com/reload: "statefulset-config"
spec:
  serviceName: test-svc
  replicas: 2
  selector:
    matchLabels:
      app: test-statefulset
  template:
    metadata:
      labels:
        app: test-statefulset
    spec:
      containers:
      - name: nginx
        image: nginxinc/nginx-unprivileged:alpine
        envFrom:
        - configMapRef:
            name: statefulset-config
EOF
```

2. Wait for StatefulSet:
```bash
kubectl wait --for=jsonpath='{.status.readyReplicas}'=2 statefulset/test-statefulset --timeout=120s
```

3. Get initial UIDs:
```bash
kubectl get pods -l app=test-statefulset -o jsonpath='{range .items[*]}{.metadata.name}: {.metadata.uid}{"\n"}{end}'
```

4. Update ConfigMap:
```bash
kubectl patch configmap statefulset-config -p '{"data":{"data":"value2"}}'
```

5. Verify StatefulSet rolling update:
```bash
kubectl rollout status statefulset/test-statefulset --timeout=120s
echo "StatefulSet reloaded: $([[ $? -eq 0 ]] && echo "✅ YES" || echo "❌ NO")"
```

**Expected Result**: ✅ StatefulSet should perform rolling update

**Cleanup**:
```bash
kubectl delete statefulset test-statefulset
kubectl delete configmap statefulset-config
```

---

## Cleanup

### Remove Test Namespace

```bash
kubectl delete namespace test-reloader
```

### Reset Operator to Default Configuration

```bash
kubectl patch deployment reloader-operator-controller-manager \
  -n reloader-operator-system \
  --type=json \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": ["--metrics-bind-address=0","--leader-elect","--health-probe-bind-address=:8081"]}]'

kubectl rollout status deployment/reloader-operator-controller-manager -n reloader-operator-system
```

---

## Summary Checklist

After completing all tests, you should have verified:

- ✅ Annotation-based reload (ConfigMap)
- ✅ Annotation-based reload (Secret)
- ✅ CRD-based reload
- ✅ Auto-reload mode
- ✅ Search & match mode
- ✅ Resource label selector
- ✅ Namespace selector
- ✅ Reload on create
- ✅ Reload on delete
- ✅ env-vars strategy
- ✅ annotations strategy
- ✅ restart strategy
- ✅ Multiple workloads watching same ConfigMap
- ✅ StatefulSet reload

---

## Troubleshooting

### Pods Not Reloading

1. Check operator logs:
```bash
kubectl logs -n reloader-operator-system -l control-plane=controller-manager -c manager --tail=100
```

2. Verify operator is running:
```bash
kubectl get pods -n reloader-operator-system
```

3. Check if namespace/resource is being filtered:
```bash
kubectl get deployment reloader-operator-controller-manager -n reloader-operator-system -o jsonpath='{.spec.template.spec.containers[0].args}'
```

4. Verify annotation syntax:
```bash
kubectl get deployment <name> -o jsonpath='{.metadata.annotations}'
```

### Checking Reload History

View ReloaderConfig status:
```bash
kubectl get reloaderconfig <name> -o yaml
```

Check pod template for reload annotations/env vars:
```bash
kubectl get deployment <name> -o jsonpath='{.spec.template.metadata.annotations}'
kubectl get deployment <name> -o jsonpath='{.spec.template.spec.containers[0].env}'
```

---

**Version**: 1.0
**Last Updated**: 2025-11-16
**Maintained by**: Reloader Operator Team
