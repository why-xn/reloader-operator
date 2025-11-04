# Reloader Operator - Complete Setup Guide

This document contains all the commands and steps used to create the Reloader Operator project from scratch using Kubebuilder.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Kubebuilder Installation](#kubebuilder-installation)
3. [Project Initialization](#project-initialization)
4. [CRD Creation](#crd-creation)
5. [Schema Design](#schema-design)
6. [Code Generation](#code-generation)
7. [Verification](#verification)
8. [Project Structure](#project-structure)
9. [Next Steps](#next-steps)

---

## Prerequisites

Before starting, ensure you have:

- **Go** 1.20+ installed
- **kubectl** installed and configured
- **git** installed
- Access to a Kubernetes cluster (optional for initial development)

Check versions:
```bash
go version
kubectl version --client
git --version
```

---

## Kubebuilder Installation

### Step 1: Download Kubebuilder

```bash
# Download the latest Kubebuilder binary
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"

# Make it executable
chmod +x kubebuilder

# Move to user bin directory (no sudo required)
mkdir -p ~/bin
mv kubebuilder ~/bin/

# Add to PATH (add this to ~/.bashrc or ~/.zshrc for persistence)
export PATH=$PATH:~/bin
```

### Step 2: Verify Installation

```bash
kubebuilder version
```

**Expected output:**
```
Version: cmd.version{KubeBuilderVersion:"4.9.0", KubernetesVendor:"1.34.0", ...}
```

---

## Project Initialization

### Step 1: Create Project Directory

```bash
# Navigate to your workspace
cd /mnt/c/Workspace/Stakater/Assignment

# Create project directory
mkdir Reloader-Operator
cd Reloader-Operator
```

### Step 2: Initialize Kubebuilder Project

```bash
kubebuilder init \
  --domain stakater.com \
  --repo github.com/stakater/Reloader \
  --project-name reloader-operator
```

**What this creates:**
- `go.mod` and `go.sum` - Go module files
- `Makefile` - Build targets
- `PROJECT` - Kubebuilder metadata
- `cmd/main.go` - Entry point
- `config/` - Kubernetes manifests
- `hack/` - Build scripts
- `.gitignore` - Git ignore rules
- `Dockerfile` - Container image definition

**Expected output:**
```
INFO Writing kustomize manifests for you to edit...
INFO Writing scaffold for you to edit...
INFO Get controller runtime
INFO Update dependencies
Next: define a resource with:
$ kubebuilder create api
```

---

## CRD Creation

### Create the ReloaderConfig API

```bash
kubebuilder create api \
  --group reloader \
  --version v1alpha1 \
  --kind ReloaderConfig \
  --resource \
  --controller
```

**Flags explained:**
- `--group reloader` - API group name (becomes `reloader.stakater.com`)
- `--version v1alpha1` - API version
- `--kind ReloaderConfig` - Resource kind name
- `--resource` - Generate resource (CRD)
- `--controller` - Generate controller (reconciler)

**What this creates:**
- `api/v1alpha1/reloaderconfig_types.go` - CRD type definitions
- `api/v1alpha1/groupversion_info.go` - API group info
- `internal/controller/reloaderconfig_controller.go` - Controller scaffold
- `internal/controller/reloaderconfig_controller_test.go` - Controller tests
- `config/crd/` - CRD manifests directory
- `config/samples/` - Example CR

**Expected output:**
```
INFO Writing kustomize manifests for you to edit...
INFO Writing scaffold for you to edit...
INFO api/v1alpha1/reloaderconfig_types.go
INFO internal/controller/reloaderconfig_controller.go
INFO Update dependencies
INFO Running make
Next: implement your new API and generate the manifests (e.g. CRDs,CRs) with:
$ make manifests
```

---

## Schema Design

### Edit the CRD Types

Edit `api/v1alpha1/reloaderconfig_types.go` to define your CRD schema.

**Location:**
```bash
vi api/v1alpha1/reloaderconfig_types.go
# or use your preferred editor
```

**Key sections to modify:**

1. **ReloaderConfigSpec** - Define desired state fields
2. **ReloaderConfigStatus** - Define observed state fields
3. **Add Kubebuilder markers** for validation, defaults, print columns

**Example modifications:**
```go
// Add validation markers
// +kubebuilder:validation:Enum=env-vars;annotations
// +kubebuilder:default=env-vars
ReloadStrategy string `json:"reloadStrategy,omitempty"`

// Add print columns (above the type definition)
// +kubebuilder:printcolumn:name="Strategy",type="string",JSONPath=".spec.reloadStrategy"

// Add short names
// +kubebuilder:resource:shortName=rc;rlc
```

### Key Fields Added

**Spec:**
- `watchedResources` - Secrets/ConfigMaps to monitor
- `targets[]` - Workloads to reload
- `reloadStrategy` - env-vars or annotations
- `autoReloadAll` - Auto-detect mode
- `alerts` - Webhook configurations
- `pausePeriod` - Reload throttling

**Status:**
- `conditions[]` - Standard conditions
- `lastReloadTime` - Timestamp
- `watchedResourceHashes` - State tracking
- `reloadCount` - Counter
- `targetStatus[]` - Per-workload status

---

## Code Generation

### Generate DeepCopy Methods

```bash
make generate
```

**What this does:**
- Runs `controller-gen object`
- Generates `zz_generated.deepcopy.go`
- Creates DeepCopy methods for all types

**Expected output:**
```
/path/to/bin/controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."
```

### Generate CRD Manifests

```bash
make manifests
```

**What this does:**
- Runs `controller-gen crd rbac webhook`
- Generates CRD YAML in `config/crd/bases/`
- Generates RBAC manifests in `config/rbac/`
- Processes all Kubebuilder markers

**Expected output:**
```
/path/to/bin/controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
```

**Generated files:**
- `config/crd/bases/reloader.stakater.com_reloaderconfigs.yaml` - CRD manifest
- `config/rbac/role.yaml` - RBAC role with permissions
- Updated kustomization files

---

## Verification

### Build the Operator

```bash
make build
```

**What this does:**
1. Runs `make generate` and `make manifests`
2. Formats code with `go fmt`
3. Lints code with `go vet`
4. Builds binary to `bin/manager`

**Expected output:**
```
go fmt ./...
go vet ./...
go build -o bin/manager cmd/main.go
```

### Run Tests

```bash
make test
```

**What this does:**
- Runs all unit tests
- Uses envtest for controller tests

### Verify CRD Structure

```bash
# View generated CRD
cat config/crd/bases/reloader.stakater.com_reloaderconfigs.yaml

# Check spec fields
grep -A 50 "spec:" config/crd/bases/reloader.stakater.com_reloaderconfigs.yaml

# Check status fields
grep -A 30 "status:" config/crd/bases/reloader.stakater.com_reloaderconfigs.yaml
```

---

## Project Structure

After all steps, your project structure should look like:

```
Reloader-Operator/
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go
│       ├── reloaderconfig_types.go       # MODIFIED - CRD schema
│       └── zz_generated.deepcopy.go      # GENERATED
│
├── bin/
│   ├── controller-gen                     # DOWNLOADED
│   └── manager                            # BUILT
│
├── cmd/
│   └── main.go                            # GENERATED
│
├── config/
│   ├── crd/
│   │   ├── bases/
│   │   │   └── reloader.stakater.com_reloaderconfigs.yaml  # GENERATED
│   │   ├── kustomization.yaml
│   │   └── kustomizeconfig.yaml
│   │
│   ├── default/                           # Kustomize defaults
│   ├── manager/                           # Deployment manifests
│   ├── prometheus/                        # Prometheus monitoring
│   ├── rbac/                              # RBAC manifests (GENERATED)
│   ├── samples/
│   │   ├── reloader_v1alpha1_reloaderconfig.yaml        # MODIFIED
│   │   ├── auto-reload-example.yaml                     # CREATED
│   │   └── advanced-example.yaml                        # CREATED
│   └── network-policy/                    # Network policies
│
├── docs/
│   ├── CRD_SCHEMA.md                      # CREATED - API documentation
│   └── SETUP_GUIDE.md                     # THIS FILE
│
├── internal/
│   └── controller/
│       ├── reloaderconfig_controller.go        # GENERATED (TODO: implement)
│       ├── reloaderconfig_controller_test.go   # GENERATED
│       └── suite_test.go                       # GENERATED
│
├── test/
│   ├── e2e/                               # E2E tests
│   └── utils/                             # Test utilities
│
├── hack/
│   └── boilerplate.go.txt                 # License header
│
├── .devcontainer/                         # Dev container config
├── .github/                               # GitHub workflows
├── .gitignore                             # GENERATED
├── .golangci.yml                          # GENERATED
├── Dockerfile                             # GENERATED
├── go.mod                                 # GENERATED
├── go.sum                                 # GENERATED
├── IMPLEMENTATION_STATUS.md               # CREATED - Progress tracker
├── Makefile                               # GENERATED
├── PROJECT                                # GENERATED
└── README.md                              # GENERATED
```

---

## Common Makefile Targets

```bash
# Generate code (DeepCopy methods)
make generate

# Generate manifests (CRDs, RBAC)
make manifests

# Run tests
make test

# Build binary
make build

# Run locally (requires k8s cluster)
make run

# Install CRDs to cluster
make install

# Uninstall CRDs from cluster
make uninstall

# Deploy operator to cluster
make deploy

# Undeploy operator from cluster
make undeploy

# Build Docker image
make docker-build IMG=<registry>/reloader-operator:tag

# Push Docker image
make docker-push IMG=<registry>/reloader-operator:tag

# Run linter
make lint

# Format code
make fmt

# Run all checks (fmt, vet, lint)
make verify
```

---

## Sample CRD Examples Created

### Basic Example

**File:** `config/samples/reloader_v1alpha1_reloaderconfig.yaml`

```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: reloaderconfig-sample
  namespace: default
spec:
  watchedResources:
    secrets:
      - db-credentials
      - api-keys
    configMaps:
      - app-config
      - feature-flags

  targets:
    - kind: Deployment
      name: web-app
      reloadStrategy: env-vars
      pausePeriod: 5m

    - kind: StatefulSet
      name: database
      reloadStrategy: annotations

  reloadStrategy: env-vars

  alerts:
    slack:
      secretRef:
        name: slack-webhook
        key: url
```

### Auto-Reload Example

**File:** `config/samples/auto-reload-example.yaml`

Shows auto-reload mode with multiple targets and alert channels.

### Advanced Example

**File:** `config/samples/advanced-example.yaml`

Shows label selectors, ignore resources, pause periods, and cross-namespace targeting.

---

## Working with the CRD

### Install CRD to Cluster

```bash
# Install CRDs
make install

# Verify installation
kubectl get crd reloaderconfigs.reloader.stakater.com
```

### Apply Sample Configuration

```bash
# Apply the sample
kubectl apply -f config/samples/reloader_v1alpha1_reloaderconfig.yaml

# View the resource
kubectl get reloaderconfig
kubectl get rc  # short name

# Describe it
kubectl describe rc reloaderconfig-sample

# Get YAML
kubectl get rc reloaderconfig-sample -o yaml
```

### View with Custom Columns

```bash
kubectl get rc

# Output:
# NAME                  STRATEGY    TARGETS   RELOADS   LAST RELOAD           AGE
# reloaderconfig-sample env-vars    2         0                               1m
```

---

## Troubleshooting

### Error: `kubebuilder: command not found`

```bash
# Ensure ~/bin is in PATH
export PATH=$PATH:~/bin

# Or add to ~/.bashrc
echo 'export PATH=$PATH:~/bin' >> ~/.bashrc
source ~/.bashrc
```

### Error: `controller-gen not found`

```bash
# Download it manually
make controller-gen
```

### Build Errors

```bash
# Clean and rebuild
rm -rf bin/
make build
```

### CRD Not Updating

```bash
# Regenerate manifests
make manifests

# Reinstall CRDs
make uninstall
make install
```

---

## Git Setup (Optional)

```bash
# Initialize git repository
git init

# Add all files
git add .

# Initial commit
git commit -m "Initial commit: Kubebuilder scaffold with ReloaderConfig CRD"

# Add remote (replace with your repo)
git remote add origin https://github.com/stakater/Reloader.git

# Push
git push -u origin main
```

---

## Summary of Commands Used

### Complete Setup from Scratch

```bash
# 1. Install Kubebuilder
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder
mkdir -p ~/bin && mv kubebuilder ~/bin/
export PATH=$PATH:~/bin

# 2. Create project
mkdir Reloader-Operator && cd Reloader-Operator
kubebuilder init --domain stakater.com --repo github.com/stakater/Reloader --project-name reloader-operator

# 3. Create API
kubebuilder create api --group reloader --version v1alpha1 --kind ReloaderConfig --resource --controller

# 4. Edit types (manual step)
# vi api/v1alpha1/reloaderconfig_types.go

# 5. Generate code
make generate
make manifests

# 6. Build
make build

# 7. Test (optional)
make test

# 8. Install to cluster (optional)
make install

# 9. Run locally (optional)
make run
```

---

## File Modifications Summary

### Files Modified from Scaffold

1. **api/v1alpha1/reloaderconfig_types.go**
   - Added comprehensive `ReloaderConfigSpec` fields
   - Added detailed `ReloaderConfigStatus` fields
   - Added Kubebuilder markers for validation
   - Added print columns and short names

2. **config/samples/reloader_v1alpha1_reloaderconfig.yaml**
   - Updated with realistic example configuration

### Files Created

1. **config/samples/auto-reload-example.yaml** - Auto-reload example
2. **config/samples/advanced-example.yaml** - Advanced features
3. **docs/CRD_SCHEMA.md** - API documentation
4. **docs/SETUP_GUIDE.md** - This file
5. **IMPLEMENTATION_STATUS.md** - Progress tracker

### Files Auto-Generated

- `api/v1alpha1/zz_generated.deepcopy.go`
- `config/crd/bases/reloader.stakater.com_reloaderconfigs.yaml`
- `config/rbac/role.yaml`
- Various kustomization files

---

## Next Steps

1. **Implement Controller Logic**
   ```bash
   vi internal/controller/reloaderconfig_controller.go
   ```

2. **Add Watchers for Secrets and ConfigMaps**

3. **Implement Hash Calculation and Change Detection**

4. **Add Workload Update Logic**

5. **Implement Backward Compatibility for Annotations**

6. **Add Tests**

7. **Create Helm Chart**

8. **Deploy to Production**

---

## References

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- [Original Reloader](https://github.com/stakater/Reloader)

---

## Support

For issues or questions:
- Check `IMPLEMENTATION_STATUS.md` for current progress
- Review `docs/CRD_SCHEMA.md` for API details
- Refer to Kubebuilder documentation

---

**Document Version:** 1.0
**Last Updated:** 2025-10-30
**Kubebuilder Version:** 4.9.0
**Project Status:** Phase 1 Complete (CRD Schema Design)
