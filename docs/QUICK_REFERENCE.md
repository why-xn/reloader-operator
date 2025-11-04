# Quick Reference - Reloader Operator Setup

## TL;DR - Recreate This Project

```bash
# Install Kubebuilder
curl -L -o kubebuilder "https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)"
chmod +x kubebuilder
mkdir -p ~/bin && mv kubebuilder ~/bin/
export PATH=$PATH:~/bin

# Initialize project
mkdir Reloader-Operator && cd Reloader-Operator
kubebuilder init --domain stakater.com --repo github.com/stakater/Reloader --project-name reloader-operator

# Create API
kubebuilder create api --group reloader --version v1alpha1 --kind ReloaderConfig --resource --controller

# Edit the CRD schema
# vi api/v1alpha1/reloaderconfig_types.go
# (Add all the spec and status fields)

# Generate code and manifests
make generate
make manifests

# Build
make build
```

## Essential Commands

| Command | Purpose |
|---------|---------|
| `make generate` | Generate DeepCopy methods |
| `make manifests` | Generate CRDs and RBAC |
| `make build` | Build the operator binary |
| `make test` | Run tests |
| `make install` | Install CRDs to cluster |
| `make run` | Run operator locally |
| `make deploy` | Deploy to cluster |

## File Locations

| File | Purpose |
|------|---------|
| `api/v1alpha1/reloaderconfig_types.go` | CRD definition |
| `internal/controller/reloaderconfig_controller.go` | Controller logic |
| `config/crd/bases/*.yaml` | Generated CRD manifests |
| `config/samples/*.yaml` | Example CRs |
| `cmd/main.go` | Entry point |

## kubectl Commands

```bash
# Install CRD
make install

# Create resource
kubectl apply -f config/samples/reloader_v1alpha1_reloaderconfig.yaml

# List resources
kubectl get reloaderconfig
kubectl get rc  # short name

# Describe
kubectl describe rc reloaderconfig-sample

# Get YAML
kubectl get rc reloaderconfig-sample -o yaml

# Delete
kubectl delete rc reloaderconfig-sample

# Uninstall CRD
make uninstall
```

## Project Structure

```
Reloader-Operator/
├── api/v1alpha1/              # CRD types
├── internal/controller/       # Reconciler logic
├── config/
│   ├── crd/bases/            # Generated CRDs
│   ├── samples/              # Example CRs
│   ├── rbac/                 # RBAC manifests
│   └── manager/              # Deployment
├── docs/                     # Documentation
└── cmd/main.go               # Entry point
```

## Kubebuilder Markers Cheat Sheet

```go
// Validation
// +kubebuilder:validation:Enum=value1;value2
// +kubebuilder:validation:Pattern=`regex`
// +kubebuilder:validation:Required
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=100

// Defaults
// +kubebuilder:default=defaultValue

// Print columns
// +kubebuilder:printcolumn:name="Column",type="string",JSONPath=".spec.field"

// Resource options
// +kubebuilder:resource:shortName=short
// +kubebuilder:subresource:status

// RBAC
// +kubebuilder:rbac:groups=group,resources=resources,verbs=get;list;watch
```

## Development Workflow

```bash
# 1. Modify types
vi api/v1alpha1/reloaderconfig_types.go

# 2. Regenerate
make generate && make manifests

# 3. Test locally
make install
make run

# 4. Test in another terminal
kubectl apply -f config/samples/reloader_v1alpha1_reloaderconfig.yaml

# 5. Build and deploy
make build
make docker-build IMG=myrepo/reloader-operator:v1
make docker-push IMG=myrepo/reloader-operator:v1
make deploy IMG=myrepo/reloader-operator:v1
```

## Common Issues

### Issue: `kubebuilder: command not found`
```bash
export PATH=$PATH:~/bin
```

### Issue: CRD changes not reflected
```bash
make manifests
make install
```

### Issue: Controller not starting
```bash
# Check logs
kubectl logs -n reloader-operator-system deployment/reloader-operator-controller-manager
```

## Git Workflow

```bash
git init
git add .
git commit -m "Initial Kubebuilder scaffold"
git remote add origin <your-repo-url>
git push -u origin main
```

## Next Implementation Steps

1. ✅ CRD Schema (DONE)
2. ⏳ Implement Reconcile() logic
3. ⏳ Add Secret/ConfigMap watchers
4. ⏳ Implement hash calculation
5. ⏳ Add workload update logic
6. ⏳ Backward compatibility layer
7. ⏳ Add tests
8. ⏳ Create Helm chart

---

For detailed information, see:
- **SETUP_GUIDE.md** - Complete setup instructions
- **CRD_SCHEMA.md** - API documentation
- **IMPLEMENTATION_STATUS.md** - Progress tracker
