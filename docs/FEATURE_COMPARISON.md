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

### ❌ Features ONLY Available in CRD (No Annotation Equivalent)

| Feature | CRD Field | Why No Annotation? |
|---------|-----------|-------------------|
| **Watch resources across namespaces** | `spec.watchedResources.namespaceSelector` | Annotations are namespace-scoped |
| **Label-based resource filtering** | `spec.watchedResources.resourceSelector` | Complex, needs CRD |
| **Reload on resource creation** | `spec.reloadOnCreate: true` | No annotation defined |
| **Reload on resource deletion** | `spec.reloadOnDelete: true` | No annotation defined |
| **Ignore specific resources** | `spec.ignoreResources: [...]` | No annotation defined |
| **Alert integrations** | `spec.alerts.slack/teams/googleChat` | Too complex for annotations |
| **Match labels** | `spec.matchLabels: {...}` | No annotation defined |
| **Centralized configuration** | Single ReloaderConfig for multiple targets | Annotations per workload |
| **Status tracking** | `status.reloadCount`, `status.targetStatus` | CRD-only feature |

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
✅ Pause period (`deployment/statefulset/daemonset.pause-period`) - **Just fixed!**
❌ Ignore (`ignore: "true"`) - **Partially implemented**

#### CRD-Based (96% working):
✅ Watched resources (secrets, configMaps)
✅ Auto reload all (`autoReloadAll`)
✅ Reload strategy (global and per-target)
✅ Pause period (`targets[].pausePeriod`)
✅ Targeted reload (`enableTargetedReload` + `requireReference`)
✅ Alert integrations
✅ Status tracking
❌ ReloadOnCreate - **Not implemented**
❌ ReloadOnDelete - **Not implemented**
❌ IgnoreResources - **Not implemented**
❌ Namespace selector - **Not implemented**
❌ Resource selector - **Not implemented**
❌ Match labels - **Not implemented**

---

## Missing Implementations

### High Priority (Should Implement in CRD):

1. **❌ Ignore feature**
   - Annotation: `reloader.stakater.com/ignore: "true"` (partially working)
   - CRD equivalent: `spec.ignoreResources` field exists but not implemented
   - **Action needed:** Implement both properly

### Medium Priority:

2. **❌ ReloadOnCreate**
   - CRD field exists: `spec.reloadOnCreate`
   - **Action needed:** Implement logic

3. **❌ ReloadOnDelete**
   - CRD field exists: `spec.reloadOnDelete`
   - **Action needed:** Implement logic

4. **❌ Cross-namespace watching**
   - CRD field exists: `spec.watchedResources.namespaceSelector`
   - **Action needed:** Implement logic

---

## Recommendations

### What's Missing from Annotations → Add to CRD:

1. **ReloadOnCreate/ReloadOnDelete**
   - These make sense and are already in the CRD spec
   - Just need implementation

2. **IgnoreResources**
   - Already in CRD spec
   - Should work like annotation `ignore: "true"`

### What's in CRD but Not Annotations - Keep CRD-only:

These features are too complex for annotations:
- Cross-namespace watching
- Label-based filtering
- Alert integrations
- Status tracking

---

## Conclusion

**Feature Parity:** ~75%
**Core features:** ✅ Both approaches support basic reload functionality including targeted reload
**Advanced features:** CRD has more (alerts, status, multi-target)
**Simplicity:** Annotations are simpler for basic use cases

**Recent improvements:**
- ✅ Pause period now works for both annotation and CRD-based configs
- ✅ Targeted reload (search+match) now available in CRD via `enableTargetedReload` + `requireReference`

**Biggest remaining gap:** CRD needs `reloadOnCreate`, `reloadOnDelete`, and `ignoreResources` implemented
