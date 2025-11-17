# Reloader Operator

A Kubernetes Operator that automatically reloads Deployments, StatefulSets, and DaemonSets when their referenced ConfigMaps or Secrets change.

## Description

Reloader Operator is a modern rewrite of [Stakater Reloader](https://github.com/stakater/Reloader) built using Kubebuilder 4.9.0 and controller-runtime. It provides **100% backward compatibility** with annotation-based configuration while offering a new CRD-based declarative API for advanced use cases.

### Key Features

- **Automatic Reload Detection**: Watches ConfigMaps and Secrets and triggers rolling updates when they change
- **Multiple Configuration Methods**:
  - Annotation-based (backward compatible with original Reloader)
  - CRD-based (ReloaderConfig custom resource)
- **Flexible Reload Strategies**:
  - `env-vars`: Inject environment variable to trigger reload
  - `annotations`: Update pod template annotations (GitOps-friendly)
  - `restart`: Delete pods without template changes
- **Namespace Filtering**: Filter which namespaces to watch using label selectors or ignore lists
- **Resource Filtering**: Filter ConfigMaps/Secrets using label selectors
- **Auto-Discovery**: Automatically detect all ConfigMaps/Secrets referenced in workloads
- **Reload on Create/Delete**: Optionally trigger reloads when watched resources are created or deleted
- **Search & Match Mode**: Selective reloading based on resource annotations

## Quick Start

### Using Annotations (Simple)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    reloader.stakater.com/auto: "true"  # Auto-reload when any referenced ConfigMap/Secret changes
spec:
  template:
    spec:
      containers:
      - name: app
        image: myapp:latest
        envFrom:
        - configMapRef:
            name: app-config
        - secretRef:
            name: db-credentials
```

### Using ReloaderConfig CRD (Advanced)

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
  namespace: default
spec:
  watchedResources:
    configMaps:
      - app-config
      - feature-flags
    secrets:
      - db-credentials
  targets:
    - kind: Deployment
      name: my-app
  reloadStrategy: env-vars
```

## Configuration

### Command-Line Flags

Configure the operator behavior using these flags:

| Flag | Description | Default | Example |
|------|-------------|---------|---------|
| `--resource-label-selector` | Only watch ConfigMaps/Secrets with matching labels | (none) | `environment=production` |
| `--namespace-selector` | Only watch namespaces with matching labels | (none) | `team=backend` |
| `--namespaces-to-ignore` | Comma-separated list of namespaces to ignore | (none) | `kube-system,kube-public` |
| `--reload-on-create` | Trigger reload when watched resources are created | `false` | `true` |
| `--reload-on-delete` | Trigger reload when watched resources are deleted | `false` | `true` |
| `--metrics-bind-address` | Address for metrics endpoint | `:8080` | `:9090` |
| `--health-probe-bind-address` | Address for health probes | `:8081` | `:9091` |
| `--leader-elect` | Enable leader election for HA | `false` | `true` |

#### Example Deployment Configuration

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
        - --metrics-bind-address=0
        - --leader-elect
        - --health-probe-bind-address=:8081
        - --resource-label-selector=managed-by=my-team
        - --namespace-selector=environment=production
        - --namespaces-to-ignore=kube-system,kube-public
        - --reload-on-create=true
        - --reload-on-delete=true
```

### Documentation

- **[docs/FEATURES.md](docs/FEATURES.md)** - Comprehensive feature documentation including command-line flags, filtering, and reload strategies
- **[docs/ANNOTATION_REFERENCE.md](docs/ANNOTATION_REFERENCE.md)** - Complete annotation reference guide
- **[docs/IMPLEMENTATION_STATUS.md](docs/IMPLEMENTATION_STATUS.md)** - Current implementation status and feature comparison
- **[docs/CRD_SCHEMA.md](docs/CRD_SCHEMA.md)** - ReloaderConfig CRD schema reference
- **[docs/MANUAL_TESTING_GUIDE.md](docs/MANUAL_TESTING_GUIDE.md)** - Step-by-step manual testing guide for all features

### Common Annotations

Quick reference for most-used annotations:

- `reloader.stakater.com/auto: "true"` - Auto-reload all referenced resources
- `secret.reloader.stakater.com/reload: "secret1,secret2"` - Reload specific Secrets
- `configmap.reloader.stakater.com/reload: "cm1,cm2"` - Reload specific ConfigMaps
- `reloader.stakater.com/rollout-strategy: "annotations"` - Set reload strategy (env-vars, annotations, restart)

## Getting Started

### Prerequisites
- go version v1.24.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/reloader-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/reloader-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/reloader-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/reloader-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v1-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Testing

The project includes comprehensive E2E tests organized into separate suites for different features.

### Running Tests

#### All E2E Tests

```sh
make e2e-test
```

This runs the main E2E test suite which includes:
- Basic reload functionality
- Annotation-based reload tests
- CRD-based reload tests
- Auto-reload tests
- Multiple reload strategy tests
- Search & match mode tests

#### Label Selector Tests

```sh
make e2e-test-label-selector
```

Tests the `--resource-label-selector` flag functionality:
- Filtering ConfigMaps by labels
- Filtering Secrets by labels
- Annotation-based reload with label filtering
- CRD-based reload with label filtering

#### Namespace Selector Tests

```sh
make e2e-test-namespace-selector
```

Tests namespace filtering functionality:
- `--namespace-selector` with label selectors
- `--namespaces-to-ignore` flag
- Both annotation-based and CRD-based reload with namespace filtering

#### Reload on Create/Delete Tests

```sh
make e2e-test-reload-on-create-delete
```

Tests the `--reload-on-create` and `--reload-on-delete` flags:
- Reload when Secrets are created/deleted (annotation-based)
- Reload when ConfigMaps are created/deleted (CRD-based)
- StatefulSet reload on resource deletion

### Test Organization

Tests are organized into separate directories under `test/`:

```
test/
├── e2e/                              # Main E2E tests
│   ├── e2e_suite_test.go            # Suite setup
│   ├── annotation_test.go            # Annotation-based reload tests
│   ├── e2e_test.go                   # Basic reload tests
│   ├── reloader_test.go              # Core reloader functionality
│   ├── targeted_reload_test.go       # Targeted reload tests
│   ├── volume_mount_test.go          # Volume mount reload tests
│   ├── workload_types_test.go        # Different workload types
│   ├── edge_cases_test.go            # Edge case scenarios
│   └── ignore_test.go                # Ignore annotation tests
├── e2e-label-selector/               # Label selector feature tests
│   ├── suite_test.go
│   └── label_selector_test.go
├── e2e-namespace-selector/           # Namespace filtering tests
│   ├── suite_test.go
│   └── namespace_selector_test.go
├── e2e-reload-on-create-delete/      # Create/delete reload tests
│   ├── suite_test.go
│   ├── reload_on_create_test.go
│   └── reload_on_delete_test.go
└── utils/                            # Shared test utilities
    └── utils.go
```

Each test suite:
- Has its own `suite_test.go` with BeforeSuite/AfterSuite setup
- Configures the operator with specific flags for that feature
- Cleans up after itself
- Can run independently or as part of the full test suite

### Running Specific Tests

Use Ginkgo focus to run specific tests:

```sh
# Run only auto-reload tests
make e2e-test GINKGO_ARGS="-ginkgo.focus=auto"

# Run only annotation tests
make e2e-test GINKGO_ARGS="-ginkgo.focus=annotation"

# Run only env-vars strategy tests
make e2e-test GINKGO_ARGS="-ginkgo.focus=env-vars"
```

### Test Prerequisites

Before running tests:

1. A Kubernetes cluster (Kind, Minikube, or real cluster)
2. Operator deployed and running
3. CRDs installed

The test framework automatically:
- Creates test namespaces
- Deploys test resources
- Patches operator configuration for specific tests
- Cleans up resources after tests

## Development

### Building and Running Locally

```sh
# Install CRDs
make install

# Run controller locally (against configured kubectl cluster)
make run

# Build binary
make build

# Run tests
make test

# Generate code and manifests
make generate manifests
```

### Building Container Image

```sh
# Build and push container image
make docker-build docker-push IMG=<your-registry>/reloader-operator:tag

# Deploy to cluster
make deploy IMG=<your-registry>/reloader-operator:tag
```

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

