# Annotation-Based vs CRD-Based Feature Comparison

## Feature Parity Analysis

### ✅ Features Available in BOTH Annotation and CRD

| Feature | Annotation | CRD Field | Status |
|---------|-----------|-----------|--------|
| **Watch specific Secrets** | `secret.reloader.stakater.com/reload: "secret-name"` | `spec.watchedResources.secrets: ["secret-name"]` | ✅ Both work |
| **Watch specific ConfigMaps** | `configmap.reloader.stakater.com/reload: "cm-name"` | `spec.watchedResources.configMaps: ["cm-name"]` | ✅ Both work |
| **Auto-reload all Secrets** | `secret.reloader.stakater.com/auto: "true"` | N/A (use AutoReloadAll) | ✅ Annotation only |
| **Auto-reload all ConfigMaps** | `configmap.reloader.stakater.com/auto: "true"` | N/A (use AutoReloadAll) | ✅ Annotation only |
| **Auto-reload everything** | `reloader.stakater.com/auto: "true"` | `spec.autoReloadAll: true` | ✅ Both work |
| **Reload strategy** | `reloader.stakater.com/rollout-strategy: "env-vars"` | `spec.reloadStrategy: "env-vars"` | ✅ Both work |
| **Per-target reload strategy** | N/A | `spec.targets[].reloadStrategy: "restart"` | ✅ CRD only |
| **Pause period** | `deployment.reloader.stakater.com/pause-period: "5m"` | `spec.targets[].pausePeriod: "5m"` | ✅ Both work |
| **Targeted reload (search+match)** | `reloader.stakater.com/search: "true"` + `reloader.stakater.com/match: "true"` | `spec.watchedResources.enableTargetedReload: true` + `spec.targets[].requireReference: true` | ✅ Both work |

---

### Features Available in CRD and Annotations (with different mechanisms)

| Feature | CRD Field | Annotation Support | Status |
|---------|-----------|-------------------|--------|
| **Reload on resource creation** | N/A (global flag: `--reload-on-create`) | Works with all annotation-based configs | ✅ Implemented |
| **Reload on resource deletion** | N/A (global flag: `--reload-on-delete`) | Works with all annotation-based configs | ✅ Implemented |

### ❌ Features ONLY Available in CRD (No Annotation Equivalent)

| Feature | CRD Field | Why No Annotation? | Status |
|---------|-----------|-------------------|--------|
| **Watch resources across namespaces** | `spec.watchedResources.namespaceSelector` | Annotations are namespace-scoped | ❌ Not implemented |
| **Label-based resource filtering** | `spec.watchedResources.resourceSelector` | Complex, needs CRD | ❌ Not implemented |
| **Ignore specific resources** | `spec.ignoreResources: [...]` | No annotation defined | ✅ Implemented |
| **Alert integrations** | `spec.alerts.slack/teams/googleChat` | Too complex for annotations | ✅ Implemented |
| **Match labels** | `spec.matchLabels: {...}` | No annotation defined | ❌ Not implemented |
| **Centralized configuration** | Single ReloaderConfig for multiple targets | Annotations per workload | ✅ Implemented |
| **Status tracking** | `status.reloadCount`, `status.targetStatus` | CRD-only feature | ✅ Implemented |

---

### ⚠️ Features ONLY Available in Annotations (No CRD Equivalent)

| Feature | Annotation | Why No CRD? |
|---------|-----------|-------------|
| **Type-specific auto reload** | `secret.reloader.stakater.com/auto: "true"` | Use `spec.autoReloadAll` instead |
| **Type-specific auto reload** | `configmap.reloader.stakater.com/auto: "true"` | Use `spec.autoReloadAll` instead |
| **Ignore workload** | `reloader.stakater.com/ignore: "true"` | Not implemented in CRD |

---

## Implementation Status Summary

### Working Features

#### Annotation-Based (100% working for defined features):
✅ Auto reload all (`auto: "true"`)
✅ Type-specific auto (`secret.auto`, `configmap.auto`)
✅ Named reload (`secret.reload`, `configmap.reload`)
✅ Targeted reload (`search + match`)
✅ Reload strategy (`rollout-strategy`)
✅ Pause period (`deployment/statefulset/daemonset.pause-period`)
✅ Ignore workload (`ignore: "true"`)

#### CRD-Based (100% of core features working):
✅ Watched resources (secrets, configMaps)
✅ Auto reload all (`autoReloadAll`)
✅ Reload strategy (global and per-target)
✅ Pause period (`targets[].pausePeriod`)
✅ Targeted reload (`enableTargetedReload` + `requireReference`)
✅ Ignore resources (`ignoreResources`)
✅ Alert integrations
✅ Status tracking
✅ ReloadOnCreate (global flag: `--reload-on-create`) - **✨ NEW**
✅ ReloadOnDelete (global flag: `--reload-on-delete`) - **✨ NEW**
❌ Namespace selector - **Not implemented** (advanced feature)
❌ Resource selector - **Not implemented** (advanced feature)
❌ Match labels - **Not implemented** (advanced feature)

---

## Missing Implementations

### Advanced Features (Low Priority):

1. **❌ Cross-namespace watching**
   - CRD field exists: `spec.watchedResources.namespaceSelector`
   - **Action needed:** Implement logic
   - **Use case:** Watch Secrets/ConfigMaps across multiple namespaces

2. **❌ Resource selector**
   - CRD field exists: `spec.watchedResources.resourceSelector`
   - **Action needed:** Implement logic
   - **Use case:** Label-based filtering of watched resources

3. **❌ Match labels**
   - CRD field exists: `spec.matchLabels`
   - **Action needed:** Implement logic
   - **Use case:** Target workloads based on label selectors

---

## Recommendations

### Implemented Features:

1. **✅ ReloadOnCreate/ReloadOnDelete** - **DONE!**
   - Implemented as global command-line flags
   - Works with both annotation-based and CRD-based configurations
   - Enable with: `--reload-on-create=true` and `--reload-on-delete=true`

### Advanced Features - Consider for Future:

These features are more complex and should remain CRD-only:
- Cross-namespace watching (`spec.watchedResources.namespaceSelector`)
- Label-based filtering (`spec.watchedResources.resourceSelector`)
- Match labels (`spec.matchLabels`)
- Alert integrations (already implemented)
- Status tracking (already implemented)

---

## Conclusion

**Feature Parity:** ~90%
**Core features:** ✅ Both approaches support all essential reload functionality
**Advanced features:** CRD has more (alerts, status, multi-target, resource-level ignore, namespace/resource selectors)
**Simplicity:** Annotations are simpler for basic use cases

**Recent improvements:**
- ✅ Pause period now works for both annotation and CRD-based configs
- ✅ Targeted reload (search+match) now available in CRD via `enableTargetedReload` + `requireReference`
- ✅ Ignore feature now fully implemented for both annotations (workload-level) and CRD (resource-level)
- **✨ NEW:** ReloadOnCreate and ReloadOnDelete implemented as global flags - works with both annotations and CRD!

**Implementation Status:**
- **Annotation-based:** 100% of defined features working
- **CRD-based:** 100% of core features working (3 advanced features pending)
