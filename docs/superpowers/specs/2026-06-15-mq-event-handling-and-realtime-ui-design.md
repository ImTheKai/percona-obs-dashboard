# MQ Event Handling and Real-Time UI Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task.

**Goal:** Handle additional OBS MQ events on the backend and propagate them to the frontend in real-time so that new PR projects appear immediately in the context dropdown and new major versions appear immediately in the version tabs.

**Architecture:** Four new event types are wired into the MQ consumer's switch statement. A new `EventUpdated` type is added to the model. The frontend's `useRealtimeStream` composable is extended to detect new PR projects via `package_update` SSE messages and call `refreshPR()` when a previously unseen PR number arrives — updating the context dropdown without polling. Version tabs are already reactive via a Vue computed property and require no extra code once the backend `package.create` fix lands.

**Tech Stack:** Go backend (no new dependencies), Vue 3 / TypeScript frontend (no new dependencies).

**User decisions (already made):**
- `package.create` → append `EventCreated` + `ws.Signal(stub)` for immediate working-set dispatch
- `project.update` and `project.update_project_conf` → append `EventUpdated` only, no working-set interaction
- `package.commit` → append `EventUpdated` only; `package.update` and `package.upload` are ignored (too noisy, build events cover them)
- PR dropdown refresh is gated: call `refreshPR()` only when the incoming PR number is not yet in `prGroups` (new PR), update in-place otherwise
- Version tabs need no additional frontend code — already reactive via Vue computed from `rawPackages`

---

## Scope

| File | Change |
|------|--------|
| `backend/internal/model/types.go` | Add `EventUpdated EventType = "updated"` |
| `backend/internal/mq/consumer.go` | Handle `package.create`, `project.update`, `project.update_project_conf`, `package.commit` |
| `frontend/src/types/api.ts` | Add `'updated'` to `EventType` union |
| `frontend/src/composables/useRealtimeStream.ts` | Accept `prGroups` + `refreshPR`; handle PR `package_update` |
| `frontend/src/App.vue` | Pass `prGroups` and `refreshPR` to `useRealtimeStream` |

No new files. No DB changes. No new API endpoints.

---

## Backend Changes

### `internal/model/types.go`

Add one constant after `EventVersionChange`:

```go
EventUpdated EventType = "updated"
```

### `internal/mq/consumer.go`

Four new cases in the `switch` block inside `handle()`. They slot in after the existing `project.delete` and `package.version_change` cases.

#### `package.create`

```go
case key == "opensuse.obs.package.create":
    evt := &model.Event{
        ID:      "evt_" + ulid.Make().String(),
        Type:    model.EventCreated,
        Scope:   inferScopeFromProject(m.Project),
        Project: m.Project,
        Package: m.Package,
        What:    fmt.Sprintf("package %s created", m.Package),
        Why:     m.Sender,
        URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", m.Project, m.Package),
        At:      time.Now().UTC(),
    }
    c.appendEvent(evt)
    stub := &model.Package{
        Project: m.Project,
        Name:    m.Package,
        Scope:   inferScopeFromProject(m.Project),
    }
    c.ws.Signal(stub)
```

The stub has no targets and zero `RollupState`. `BuildStateTask` will populate it from OBS on first worker run. If OBS has not yet scheduled any targets, `BuildStateTask` writes an empty package to the DB (harmless); subsequent build events will update it. The package enters the working set immediately, so the first time OBS returns real targets the worker upserts and notifies SSE.

#### `project.update`

```go
case key == "opensuse.obs.project.update":
    evt := &model.Event{
        ID:      "evt_" + ulid.Make().String(),
        Type:    model.EventUpdated,
        Scope:   inferScopeFromProject(m.Project),
        Project: m.Project,
        What:    fmt.Sprintf("project %s updated", m.Project),
        Why:     m.Sender,
        URL:     fmt.Sprintf("https://build.opensuse.org/project/show/%s", m.Project),
        At:      time.Now().UTC(),
    }
    c.appendEvent(evt)
```

#### `project.update_project_conf`

```go
case key == "opensuse.obs.project.update_project_conf":
    evt := &model.Event{
        ID:      "evt_" + ulid.Make().String(),
        Type:    model.EventUpdated,
        Scope:   inferScopeFromProject(m.Project),
        Project: m.Project,
        What:    fmt.Sprintf("project %s configuration updated", m.Project),
        Why:     m.Sender,
        URL:     fmt.Sprintf("https://build.opensuse.org/project/show/%s", m.Project),
        At:      time.Now().UTC(),
    }
    c.appendEvent(evt)
```

#### `package.commit`

```go
case key == "opensuse.obs.package.commit":
    evt := &model.Event{
        ID:      "evt_" + ulid.Make().String(),
        Type:    model.EventUpdated,
        Scope:   inferScopeFromProject(m.Project),
        Project: m.Project,
        Package: m.Package,
        What:    fmt.Sprintf("%s committed (rev %s)", m.Package, m.Rev),
        Why:     m.Comment,
        URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", m.Project, m.Package),
        At:      time.Now().UTC(),
    }
    c.appendEvent(evt)
```

---

## Frontend Changes

### `types/api.ts`

Add `'updated'` to the `EventType` union:

```ts
export type EventType = 'triggered' | 'started' | 'succeeded' | 'failed' | 'unresolvable' |
  'broken' | 'blocked' | 'published' | 'created' | 'deleted' | 'build_started' |
  'build_finished' | 'version_change' | 'updated'
```

### `useRealtimeStream.ts`

Extended signature:

```ts
export function useRealtimeStream(
  packages: Ref<Package[]>,
  events: Ref<Event[]>,
  prGroups: Ref<PRGroup[]>,
  refresh: () => void,
  refreshPR: () => void,
): void
```

Helper to extract the PR number from a project string (mirrors the backend `obs.PRNumber` function):

```ts
function prNumberFromProject(project: string): string {
  const parts = project.split(':')
  const idx = parts.findIndex(p => p.toLowerCase() === 'pr')
  if (idx >= 0 && idx + 1 < parts.length) {
    return parts[idx + 1].toLowerCase().replace(/^pr-/, '')
  }
  return ''
}
```

Updated `package_update` handling:

```ts
if (msg.type === 'package_update') {
  const pkg = msg.data as Package
  const prNum = prNumberFromProject(pkg.project)

  if (prNum) {
    // PR package: check if this PR is already known
    const group = prGroups.value.find(g => g.pr === prNum)
    if (!group) {
      // New PR — fetch full PR data so the context dropdown updates
      refreshPR()
    } else {
      // Known PR — update the package in the group in-place
      const pkgIdx = group.packages.findIndex(
        p => p.project === pkg.project && p.name === pkg.name,
      )
      if (pkgIdx >= 0) {
        group.packages[pkgIdx] = pkg
      } else {
        group.packages.push(pkg)
      }
      // Recompute group rollup_state as worst state among packages
      const SEVERITY: Record<string, number> = {
        broken: 5, unresolvable: 4, failed: 3, blocked: 2,
        building: 1, finished: 1, scheduled: 1, succeeded: 0,
      }
      const worst = group.packages.reduce((acc, p) => {
        return (SEVERITY[p.rollup_state] ?? 0) > (SEVERITY[acc] ?? 0)
          ? p.rollup_state
          : acc
      }, 'succeeded' as string)
      group.rollup_state = worst as PRGroup['rollup_state']
    }
  } else {
    // Non-PR package: existing in-place update logic
    const idx = packages.value.findIndex(
      p => p.project === pkg.project && p.name === pkg.name,
    )
    if (idx >= 0) {
      packages.value[idx] = pkg
    } else {
      packages.value.push(pkg)
    }
  }
}
```

### `App.vue`

Update the `useRealtimeStream` call to pass the two new arguments:

```ts
useRealtimeStream(rawPackages, events, prGroups, refresh, refreshPR)
```

---

## Version Tabs: No Extra Code Needed

`availableVersions` is a Vue `computed` derived from `rawPackages`. When a `package_update` SSE arrives for a new version's package (e.g., `isv:percona:ppg:18:postgres`), it is pushed into `rawPackages` via the non-PR branch above, and `availableVersions` recomputes automatically — the "18" tab appears without any additional code.

The reason version tabs do not update today is that `package.create` events are unhandled, so new version packages never enter the DB and never emit `package_update` SSE. The `package.create` backend fix above resolves this as a side effect.

---

## Data Flow After Changes

```
OBS MQ
  opensuse.obs.package.create
    → consumer: appendEvent(EventCreated) + ws.Signal(stub)
    → worker: BuildStateTask fetches OBS state → upserts DB → hub.Notify(package_update SSE)
    → frontend: package_update received
        if new PR → refreshPR() → prGroups updates → context dropdown shows new PR
        if new version → rawPackages updated → availableVersions recomputes → version tab appears

  opensuse.obs.project.update / project.update_project_conf
    → consumer: appendEvent(EventUpdated)
    → frontend: new_event received → appears in event log

  opensuse.obs.package.commit
    → consumer: appendEvent(EventUpdated)
    → frontend: new_event received → appears in event log
```

---

## Error Handling

- **OBS not yet ready on `package.create`**: `BuildStateTask` may return empty targets if OBS has not scheduled builds yet. The worker upserts an empty package (harmless). Subsequent `repo.build_started` / `package.build_*` events will update it.
- **`refreshPR()` failure**: `usePRPackages` already swallows fetch errors into `error.value`. A failed refresh leaves `prGroups` unchanged; the next `package_update` for the same PR will retry `refreshPR()` since the PR is still absent from `prGroups`.
- **`package.commit` with empty `Rev`**: `fmt.Sprintf("%s committed (rev %s)", m.Package, m.Rev)` produces `"foo committed (rev )"` which is acceptable — `Rev` is populated in practice.
