# Reloader Operator - Implementation Status

## Project Overview

This is a Kubernetes Operator rewrite of the [Stakater Reloader](https://github.com/stakater/Reloader) project, built using **Kubebuilder 4.9.0** and **controller-runtime**. The goal is to maintain **100% backward compatibility** with the existing annotation-based configuration while providing a modern CRD-based declarative API.

## ✅ Completed Phase 1: CRD Schema Design

### What's Been Implemented

#### 1. Kubebuilder Project Setup
- ✅ Kubebuilder 4.9.0 installed and configured
- ✅ Project initialized with domain `stakater.com`
- ✅ Go module structure created
- ✅ Makefile with build targets
- ✅ Dockerfile for container builds
- ✅ GitHub workflows for CI/CD
- ✅ golangci-lint configuration

#### 2. ReloaderConfig CRD (v1alpha1)
- ✅ **Comprehensive API schema** designed with all features from original Reloader
- ✅ **OpenAPI v3 validation** via Kubebuilder markers
- ✅ **Short names** configured: `rc`, `rlc`
- ✅ **Custom columns** for `kubectl get` output
- ✅ **Status subresource** for tracking reload state
- ✅ **Generated CRDs** in `config/crd/bases/`

#### 3. CRD Features Implemented

##### Spec Fields
| Feature | Field | Status |
|---------|-------|--------|
| Watch specific resources | `watchedResources` | ✅ |
| Target workloads | `targets[]` | ✅ |
| Reload strategies | `reloadStrategy` | ✅ (enum: env-vars, annotations) |
| Auto-reload mode | `autoReloadAll` | ✅ |
| Reload on create/delete | `reloadOnCreate`, `reloadOnDelete` | ✅ |
| Ignore resources | `ignoreResources[]` | ✅ |
| Alerting | `alerts` | ✅ (Slack, Teams, Google Chat, Custom) |
| Label matching | `matchLabels` | ✅ |
| Namespace selectors | `namespaceSelector` | ✅ |
| Resource selectors | `resourceSelector` | ✅ |
| Pause periods | `pausePeriod` | ✅ (per-target) |

##### Status Fields
| Feature | Field | Status |
|---------|-------|--------|
| Conditions | `conditions[]` | ✅ |
| Last reload time | `lastReloadTime` | ✅ |
| Resource hashes | `watchedResourceHashes` | ✅ |
| Reload counter | `reloadCount` | ✅ |
| Per-target status | `targetStatus[]` | ✅ |
| Observed generation | `observedGeneration` | ✅ |

##### Validation & Defaults
- ✅ Enum validation for `kind`, `reloadStrategy`
- ✅ Pattern validation for `pausePeriod` (duration format)
- ✅ Required field enforcement
- ✅ Default value: `reloadStrategy: env-vars`

#### 4. Documentation
- ✅ **CRD_SCHEMA.md** - Comprehensive API reference
- ✅ **Example manifests** in `config/samples/`:
  - Basic example
  - Auto-reload example
  - Advanced features example
- ✅ **Mapping guide** - Annotation to CRD conversion

#### 5. Code Generation
- ✅ DeepCopy methods generated
- ✅ CRD manifests generated
- ✅ RBAC roles generated
- ✅ Build successfully compiles

### Project Structure

```
Reloader-Operator/
├── api/v1alpha1/
│   ├── reloaderconfig_types.go          # CRD definition (COMPLETED)
│   ├── groupversion_info.go
│   └── zz_generated.deepcopy.go          # Auto-generated
│
├── internal/controller/
│   ├── reloaderconfig_controller.go      # Reconciler (SCAFFOLD - TODO)
│   ├── reloaderconfig_controller_test.go
│   └── suite_test.go
│
├── config/
│   ├── crd/bases/
│   │   └── reloader.stakater.com_reloaderconfigs.yaml  # Generated CRD (COMPLETED)
│   ├── rbac/                             # RBAC manifests (GENERATED)
│   ├── manager/                          # Deployment manifests (GENERATED)
│   ├── samples/                          # Examples (COMPLETED)
│   │   ├── reloader_v1alpha1_reloaderconfig.yaml
│   │   ├── auto-reload-example.yaml
│   │   └── advanced-example.yaml
│   └── default/                          # Kustomize defaults
│
├── docs/
│   └── CRD_SCHEMA.md                     # API documentation (COMPLETED)
│
├── cmd/main.go                           # Entry point (SCAFFOLD)
├── Makefile                              # Build targets (GENERATED)
├── Dockerfile                            # Container image (GENERATED)
└── go.mod                                # Dependencies (GENERATED)
```

## 🚧 Pending Implementation

### Phase 2: Core Reconciliation Logic (NEXT)
- [ ] Secret watcher reconciler
- [ ] ConfigMap watcher reconciler
- [ ] Resource hash calculation (SHA256)
- [ ] Change detection logic
- [ ] Workload discovery (find deployments/statefulsets/etc.)

### Phase 3: Backward Compatibility
- [ ] Annotation parser
- [ ] Annotation → internal config converter
- [ ] Merge CRD + annotation configs
- [ ] Legacy annotation support validation

### Phase 4: Reload Strategies
- [ ] env-vars strategy implementation
- [ ] annotations strategy implementation
- [ ] Workload update executor
- [ ] Pause period enforcement

### Phase 5: Advanced Features
- [ ] Alerting integration (Slack, Teams, Google Chat)
- [ ] Metrics collection (Prometheus)
- [ ] Webhook support
- [ ] Leadership election for HA

### Phase 6: Testing
- [ ] Unit tests with envtest
- [ ] Integration tests
- [ ] E2E tests (kind/minikube)
- [ ] Backward compatibility tests

### Phase 7: Deployment
- [ ] Helm chart creation
- [ ] Kustomize overlays
- [ ] Migration guide
- [ ] CI/CD pipeline

## Technical Decisions

### Why Kubebuilder over Operator SDK?
- More actively maintained
- Simpler project structure
- Better documentation
- Native controller-runtime integration

### CRD Design Philosophy
1. **Declarative First** - CRD is the primary API
2. **Backward Compatible** - Annotations still work
3. **Extensible** - Easy to add new features
4. **Validated** - API server enforces correctness
5. **Observable** - Rich status for troubleshooting

### Status Fields Rationale
- `watchedResourceHashes` - Track current state, detect changes
- `targetStatus[]` - Per-workload reload tracking
- `reloadCount` - Audit trail
- `pausedUntil` - Prevent reload storms
- `observedGeneration` - Reconciliation tracking

## Backward Compatibility Strategy

### Both APIs Work Simultaneously

**Annotations (Legacy):**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    reloader.stakater.com/auto: "true"
```

**CRD (New):**
```yaml
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
spec:
  autoReloadAll: true
  targets:
    - kind: Deployment
      name: my-app
```

### Reconciler Logic (Planned)
```go
func (r *Reconciler) Reconcile(ctx, req) {
    // 1. Check for ReloaderConfig CRD
    crdConfig := r.getReloaderConfig(ctx, req)

    // 2. Check for annotation-based config
    annotationConfig := r.scanAnnotations(ctx, req)

    // 3. Merge (CRD takes precedence)
    config := merge(crdConfig, annotationConfig)

    // 4. Execute reload logic
    return r.executeReload(ctx, config)
}
```

## kubectl Examples

Once deployed, users can:

```bash
# Create a ReloaderConfig
kubectl apply -f config/samples/reloader_v1alpha1_reloaderconfig.yaml

# List configurations
kubectl get rc
kubectl get reloaderconfigs

# Output:
# NAME                  STRATEGY    TARGETS   RELOADS   LAST RELOAD           AGE
# my-app-reloader      env-vars    2         5         2025-10-30T14:30:00Z  2d

# Get detailed status
kubectl get rc my-app-reloader -o yaml

# Watch for changes
kubectl get rc -w

# Describe
kubectl describe rc my-app-reloader
```

## Next Steps

1. **Implement reconciliation controller** - Core logic
2. **Add Secret/ConfigMap watchers** - Trigger on resource changes
3. **Implement hash calculation** - Detect actual data changes
4. **Build workload updater** - Execute rolling updates
5. **Add backward compatibility layer** - Parse annotations

## Build & Test

```bash
# Generate code
make generate

# Generate manifests
make manifests

# Build
make build

# Run tests
make test

# Run locally (requires running k8s cluster)
make run

# Build container
make docker-build IMG=myrepo/reloader-operator:v2.0.0

# Deploy to cluster
make install  # Install CRDs
make deploy   # Deploy operator
```

## Dependencies

- **Go**: 1.24.6
- **Kubebuilder**: 4.9.0
- **controller-runtime**: v0.22.1
- **Kubernetes**: 1.34.0

## Migration Path for Users

### Option 1: No Changes Required
Keep using annotations - they continue to work!

### Option 2: Gradual Migration
```yaml
# Step 1: Create ReloaderConfig
apiVersion: reloader.stakater.com/v1alpha1
kind: ReloaderConfig
metadata:
  name: my-app-reloader
spec:
  # ... configuration

# Step 2: Remove annotations from Deployment
# (Optional - can keep both)
```

### Option 3: Full CRD Adoption
Use only CRDs for new deployments, centralized config management.

## Success Criteria

- ✅ CRD schema designed and validated
- ✅ Documentation complete
- ✅ Examples provided
- ✅ Build successful
- [ ] 100% feature parity with original Reloader
- [ ] 100% backward compatibility with annotations
- [ ] All tests passing
- [ ] Production-ready deployment manifests

---

**Current Status**: Phase 1 Complete ✅
**Next Milestone**: Implement reconciliation controller
**Target Completion**: TBD based on development pace
