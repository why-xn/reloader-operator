# Implementing Targeted Reload Feature (Search + Match)

## Overview

The targeted reload feature provides **fine-grained control** over which ConfigMaps/Secrets trigger workload reloads. It uses a **two-way opt-in system**:

1. **Workload** says: "I'm searching for marked resources" (`reloader.stakater.com/search: "true"`)
2. **Resource** says: "I'm marked and eligible for reload" (`reloader.stakater.com/match: "true"`)
3. **Condition**: The resource must also be **referenced** in the workload's pod spec

Only when **ALL three conditions** are met does a reload occur.

---

## Use Case

**Problem:** In multi-tenant clusters or shared environments, you may have:
- System-wide ConfigMaps/Secrets used by many workloads
- Some workloads should reload when these resources change
- Other workloads should NOT reload (e.g., monitoring, logging agents)

**Solution:** Targeted reload lets you:
- Mark specific ConfigMaps/Secrets with `match: "true"`
- Mark specific workloads with `search: "true"`
- Only marked workloads reload when marked resources change

**Example Scenario:**
```yaml
# Shared database config used by 10 microservices
apiVersion: v1
kind: ConfigMap
metadata:
  name: db-config
  annotations:
    reloader.stakater.com/match: "true"  # ← Marked for targeted reload
data:
  host: postgres.prod.svc.cluster.local
  port: "5432"
---
# API Service - wants to reload when db-config changes
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-service
  annotations:
    reloader.stakater.com/search: "true"  # ← Searching for marked resources
spec:
  template:
    spec:
      containers:
      - name: api
        envFrom:
        - configMapRef:
            name: db-config  # ← References the marked ConfigMap
---
# Monitoring Service - does NOT want to reload
apiVersion: apps/v1
kind: Deployment
metadata:
  name: monitoring
  # NO search annotation
spec:
  template:
    spec:
      containers:
      - name: monitor
        envFrom:
        - configMapRef:
            name: db-config  # ← Also references db-config, but won't reload
```

**Result:**
- When `db-config` changes:
  - ✅ `api-service` reloads (has search, references db-config, db-config has match)
  - ❌ `monitoring` does NOT reload (no search annotation)

---

## Behavior Rules

### 1. Precedence Rules

From Reloader README:
> `reloader.stakater.com/auto` and `reloader.stakater.com/search` **cannot be used together** — the `auto` annotation takes precedence.

**Priority order:**
1. `auto: "true"` → Reloads on ANY referenced resource change
2. `auto: "false"` → Explicitly disabled, no reload
3. `search: "true"` → Only reloads if resource has `match: "true"`
4. No annotations → No reload (unless global `--auto-reload-all` flag is set)

### 2. Interaction with Other Annotations

| Workload Annotation | Resource Annotation | Behavior |
|---------------------|---------------------|----------|
| `auto: "true"` | Any | Reloads regardless of `match` (auto takes precedence) |
| `search: "true"` | `match: "true"` | ✅ Reloads if resource is referenced |
| `search: "true"` | No `match` | ❌ Does NOT reload |
| `search: "true"` | `match: "false"` | ❌ Does NOT reload |
| `auto: "true"` + `search: "true"` | Any | `auto` takes precedence, `search` is ignored |
| `search: "true"` | `ignore: "true"` | ❌ `ignore` wins, no reload |

### 3. Reference Requirement

The resource **must be referenced** in the workload's pod spec via:
- `envFrom` (configMapRef or secretRef)
- `env` with `valueFrom` (configMapKeyRef or secretKeyRef)
- `volumes` with configMap or secret
- `imagePullSecrets`

---

## Current Implementation Status

### ✅ What Exists
- Constants defined: `AnnotationSearch`, `AnnotationMatch` (in `internal/pkg/util/helpers.go`)
- Documentation in `ANNOTATION_REFERENCE.md`

### ❌ What's Missing
- **No logic to check search annotation** on workloads
- **No logic to check match annotation** on ConfigMaps/Secrets
- **No handling of auto vs search precedence**
- **No tests for targeted reload**

---

## Implementation Plan

### Phase 1: Add Resource Match Checking (2 hours)

**Files to modify:**
1. `internal/pkg/workload/finder.go`

**Changes:**

```go
// Add new function to check if a resource has match annotation
func resourceHasMatchAnnotation(resource client.Object) bool {
    annotations := resource.GetAnnotations()
    if annotations == nil {
        return false
    }

    // Check for match annotation
    matchValue := annotations[util.AnnotationMatch]
    return matchValue == "true"
}
```

**Usage:** Called when checking if a resource change should trigger reload.

---

### Phase 2: Update FindWorkloadsWithAnnotations Logic (3 hours)

**Files to modify:**
1. `internal/pkg/workload/finder.go:shouldReloadFromAnnotations()`

**Current Logic:**
```go
func shouldReloadFromAnnotations(...) bool {
    // Check auto annotation
    if annotations[util.AnnotationAuto] == "true" {
        if podSpec != nil && workloadReferencesResource(...) {
            return true
        }
    }

    // Check type-specific auto
    // ...

    // Check named reload
    // ...

    return false
}
```

**New Logic:**
```go
func shouldReloadFromAnnotations(
    annotations map[string]string,
    podSpec *corev1.PodSpec,
    resourceKind, resourceName string,
    resourceAnnotations map[string]string,  // ← NEW parameter
) bool {
    // Rule 1: Check auto annotation (takes precedence over search)
    autoValue := annotations[util.AnnotationAuto]
    if autoValue == "true" {
        if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
            return true
        }
    } else if autoValue == "false" {
        // Explicitly disabled
        return false
    }

    // Rule 2: Check type-specific auto
    if resourceKind == util.KindSecret && annotations[util.AnnotationSecretAuto] == "true" {
        if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
            return true
        }
    }
    if resourceKind == util.KindConfigMap && annotations[util.AnnotationConfigMapAuto] == "true" {
        if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
            return true
        }
    }

    // Rule 3: Check named reload (specific resource name)
    var reloadList string
    if resourceKind == util.KindSecret {
        reloadList = annotations[util.AnnotationSecretReload]
    } else if resourceKind == util.KindConfigMap {
        reloadList = annotations[util.AnnotationConfigMapReload]
    }

    if reloadList != "" {
        names := util.ParseCommaSeparatedList(reloadList)
        return util.ContainsString(names, resourceName)
    }

    // Rule 4: Check search + match (NEW LOGIC)
    searchValue := annotations[util.AnnotationSearch]
    if searchValue == "true" {
        // Workload is in search mode
        // Check if resource has match annotation
        if resourceAnnotations != nil && resourceAnnotations[util.AnnotationMatch] == "true" {
            // Check if resource is referenced
            if podSpec != nil && workloadReferencesResource(podSpec, resourceKind, resourceName) {
                return true
            }
        }
    }

    return false
}
```

---

### Phase 3: Update FindWorkloadsWithAnnotations Calls (2 hours)

**Files to modify:**
1. `internal/pkg/workload/finder.go:FindWorkloadsWithAnnotations()`

**Current signature:**
```go
func (f *Finder) FindWorkloadsWithAnnotations(
    ctx context.Context,
    resourceKind string,
    resourceName string,
    resourceNamespace string,
) ([]Target, error)
```

**Changes needed:**
```go
func (f *Finder) FindWorkloadsWithAnnotations(
    ctx context.Context,
    resourceKind string,
    resourceName string,
    resourceNamespace string,
    resourceAnnotations map[string]string,  // ← NEW parameter
) ([]Target, error)
```

**Update all calls in:**
- `internal/controller/reloaderconfig_controller.go:discoverTargets()`

---

### Phase 4: Pass Resource Annotations to Finder (1 hour)

**Files to modify:**
1. `internal/controller/reloaderconfig_controller.go`

**Current:**
```go
annotatedWorkloads, err := r.WorkloadFinder.FindWorkloadsWithAnnotations(
    ctx, resourceKind, resourceName, resourceNamespace)
```

**New:**
```go
// Get the resource to access its annotations
var resourceAnnotations map[string]string
if resourceKind == util.KindSecret {
    secret := &corev1.Secret{}
    if err := r.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: resourceNamespace}, secret); err == nil {
        resourceAnnotations = secret.Annotations
    }
} else if resourceKind == util.KindConfigMap {
    cm := &corev1.ConfigMap{}
    if err := r.Get(ctx, client.ObjectKey{Name: resourceName, Namespace: resourceNamespace}, cm); err == nil {
        resourceAnnotations = cm.Annotations
    }
}

annotatedWorkloads, err := r.WorkloadFinder.FindWorkloadsWithAnnotations(
    ctx, resourceKind, resourceName, resourceNamespace, resourceAnnotations)
```

---

### Phase 5: Add Unit Tests (3 hours)

**Files to create/modify:**
1. `internal/pkg/workload/finder_test.go`

**Test cases:**

```go
func TestShouldReloadFromAnnotations_SearchAndMatch(t *testing.T) {
    tests := []struct {
        name                string
        workloadAnnotations map[string]string
        resourceAnnotations map[string]string
        resourceReferenced  bool
        expectedReload      bool
    }{
        {
            name:                "search true, match true, referenced",
            workloadAnnotations: map[string]string{"reloader.stakater.com/search": "true"},
            resourceAnnotations: map[string]string{"reloader.stakater.com/match": "true"},
            resourceReferenced:  true,
            expectedReload:      true,
        },
        {
            name:                "search true, match false, referenced",
            workloadAnnotations: map[string]string{"reloader.stakater.com/search": "true"},
            resourceAnnotations: map[string]string{"reloader.stakater.com/match": "false"},
            resourceReferenced:  true,
            expectedReload:      false,
        },
        {
            name:                "search true, no match, referenced",
            workloadAnnotations: map[string]string{"reloader.stakater.com/search": "true"},
            resourceAnnotations: map[string]string{},
            resourceReferenced:  true,
            expectedReload:      false,
        },
        {
            name:                "search true, match true, NOT referenced",
            workloadAnnotations: map[string]string{"reloader.stakater.com/search": "true"},
            resourceAnnotations: map[string]string{"reloader.stakater.com/match": "true"},
            resourceReferenced:  false,
            expectedReload:      false,
        },
        {
            name:                "auto true takes precedence over search",
            workloadAnnotations: map[string]string{
                "reloader.stakater.com/auto":   "true",
                "reloader.stakater.com/search": "true",
            },
            resourceAnnotations: map[string]string{},  // No match needed
            resourceReferenced:  true,
            expectedReload:      true,  // Auto wins
        },
        {
            name:                "auto false blocks search",
            workloadAnnotations: map[string]string{
                "reloader.stakater.com/auto":   "false",
                "reloader.stakater.com/search": "true",
            },
            resourceAnnotations: map[string]string{"reloader.stakater.com/match": "true"},
            resourceReferenced:  true,
            expectedReload:      false,  // Auto false blocks
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

---

### Phase 6: Add E2E Tests (4 hours)

**Files to modify:**
1. `test/e2e/annotation_test.go`

**Test scenario 1: Search + Match with referenced resource**
```go
It("should reload when workload has search and resource has match", func() {
    // Create ConfigMap with match annotation
    configMapYAML := GenerateConfigMap(configMapName, testNS, map[string]string{
        "config": "value",
    })
    // Add match annotation manually

    // Create Deployment with search annotation that references the ConfigMap
    deploymentYAML := GenerateDeployment(deploymentName, testNS, DeploymentOpts{
        Annotations: map[string]string{
            "reloader.stakater.com/search": "true",
        },
        ConfigMapName: configMapName,
    })

    // Update ConfigMap
    // Verify Deployment reloads
})
```

**Test scenario 2: Search without Match**
```go
It("should NOT reload when workload has search but resource lacks match", func() {
    // Create ConfigMap WITHOUT match annotation
    // Create Deployment with search annotation
    // Update ConfigMap
    // Verify Deployment does NOT reload
})
```

**Test scenario 3: Auto takes precedence**
```go
It("should use auto annotation when both auto and search are present", func() {
    // Create ConfigMap without match annotation
    // Create Deployment with both auto and search annotations
    // Update ConfigMap
    // Verify Deployment reloads (auto takes precedence)
})
```

---

### Phase 7: Update Documentation (1 hour)

**Files to modify:**
1. `ANNOTATION_REFERENCE.md` - Update status from ❌ Missing to ✅ Implemented
2. `README.md` - Add usage examples
3. `docs/EXAMPLES.md` - Add targeted reload examples

---

## Implementation Checklist

- [x] Phase 1: Add resource match checking helper function
- [x] Phase 2: Update shouldReloadFromAnnotations with search+match logic
- [x] Phase 3: Update FindWorkloadsWithAnnotations signature
- [x] Phase 4: Pass resource annotations from controller to finder
- [x] Phase 5: Add comprehensive unit tests
- [x] Phase 6: Add E2E tests
- [x] Phase 7: Update documentation

**Status:** ✅ **COMPLETED**
**Estimated Total Time:** 16 hours

---

## Key Implementation Notes

1. **Auto annotation always takes precedence** - If `auto: "true"`, search/match is ignored
2. **Auto: "false" blocks everything** - Explicit opt-out wins
3. **Three conditions must ALL be true** for search+match reload:
   - Workload has `search: "true"`
   - Resource has `match: "true"`
   - Resource is referenced in pod spec
4. **Ignore annotation still wins** - If resource has `ignore: "true"`, no reload happens

---

## Testing Strategy

### Unit Tests
- Test all precedence rules (auto > search)
- Test all combinations of search/match values
- Test with/without resource references
- Test ignore annotation blocking search+match

### E2E Tests
- Test basic search+match scenario
- Test auto precedence over search
- Test multiple workloads with different search settings
- Test mixed scenarios (some with auto, some with search)

---

## Backward Compatibility

✅ **Fully backward compatible** - This is a new feature that doesn't change existing behavior:
- Existing deployments without search annotation continue working
- Auto annotations continue to work as before
- Named reload continues to work
- No breaking changes to APIs or CRDs

---

## Migration Path

Users can gradually adopt targeted reload:

**Step 1:** Add `match: "true"` to ConfigMaps/Secrets you want to control
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    reloader.stakater.com/match: "true"
```

**Step 2:** Change workloads from `auto` to `search`
```yaml
# Before
annotations:
  reloader.stakater.com/auto: "true"

# After
annotations:
  reloader.stakater.com/search: "true"
```

**Step 3:** Gradually mark more resources and workloads as needed

---

## Success Criteria

Implementation is complete when:
1. ✅ All unit tests pass
2. ✅ All E2E tests pass
3. ✅ Documentation updated
4. ✅ Backward compatibility maintained
5. ✅ Code coverage > 80% for new code
