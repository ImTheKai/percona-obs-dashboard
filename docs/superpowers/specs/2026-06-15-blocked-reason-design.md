# Blocked Package Reason Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task.

**Goal:** For packages in the `blocked` state, show in the package card the reason for blocking (which packages they are waiting on), fetched from the OBS `_builddepinfo` API.

**Architecture:** A `blocked_by` string is added to each `Target` in the data model. The OBS client gains a focused `PackageBlockedReason` method. Both the MQ consumer (for near-instant updates when a blocked event arrives via RabbitMQ) and the poller (to keep the reason current on every tick while the package remains blocked) enrich blocked targets before writing to the store. Enriched data flows through the existing `UpsertPackageState` → SSE hub → frontend pipeline unchanged.

**Tech Stack:** Go standard library, Vue 3 Composition API. No new dependencies, no new endpoints, no new DB tables.

**User decisions (already made):**
- Blocking reason fetched and stored by the backend, not on-demand by the frontend.
- Per-target granularity: each blocked target row shows its own blocking reason.
- Both the MQ consumer (real-time) and the poller (every tick while blocked) enrich blocked targets.
- The OBS `_builddepinfo` endpoint already returns an `error` XML element with the blocking reason string — no cross-referencing of dep graphs needed.

---

## Scope

| File | Change |
|------|--------|
| `backend/internal/model/package.go` | Add `BlockedBy string` to `Target` |
| `backend/internal/obs/client.go` | Add `PackageBlockedReason` method; extend `DepInfo` with `Error` field |
| `backend/internal/obs/poller.go` | Enrich blocked targets with `PackageBlockedReason` in `tick()` |
| `backend/internal/mq/consumer.go` | Enrich blocked targets with `PackageBlockedReason` before `upsertPackage` |
| `frontend/src/types/api.ts` | Add `blocked_by?: string` to `Target` |
| `frontend/src/components/PackageCard.vue` | Show `blocked_by` as a second line in blocked target rows |

No other files change. Existing REST endpoints and SSE stream are untouched.

---

## Backend

### Data model (`model/package.go`)

```go
type Target struct {
    Repo      string `json:"repo"`
    Arch      string `json:"arch"`
    State     string `json:"state"`
    BlockedBy string `json:"blocked_by,omitempty"`
}
```

`BlockedBy` is `omitempty` so non-blocked targets and targets where the API returned no reason add no overhead to the JSON payload.

### OBS client (`obs/client.go`)

Extend `DepInfo` to capture the `error` XML element:

```go
type DepInfo struct {
    Package string   `xml:"package,attr"`
    Deps    []string `xml:"pkgdep"`
    Error   string   `xml:"error"`
}
```

Add a focused method for fetching a single package's blocking reason:

```go
// PackageBlockedReason returns the blocking reason string for a specific package
// in a given (project, repo, arch) tuple, as reported by OBS _builddepinfo.
// Returns ("", nil) when no error element is present (package is not blocked or
// the API returned no reason).
func (c *Client) PackageBlockedReason(ctx context.Context, project, repo, arch, pkg string) (string, error) {
    path := fmt.Sprintf("/build/%s/%s/%s/_builddepinfo?package=%s", project, repo, arch, pkg)
    resp, err := c.get(ctx, path)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        Packages []DepInfo `xml:"package"`
    }
    if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", fmt.Errorf("parse _builddepinfo for %s/%s/%s/%s: %w", project, repo, arch, pkg, err)
    }
    for _, d := range result.Packages {
        if d.Package == pkg {
            return d.Error, nil
        }
    }
    return "", nil
}
```

### Poller (`obs/poller.go`)

In `tick()`, after `buildPackage()` produces a package, enrich blocked targets before the upsert:

```go
// After buildPackage returns pkg:
enrichBlockedTargets(ctx, p.client, pkg)

// New helper (unexported):
func enrichBlockedTargets(ctx context.Context, client *Client, pkg *model.Package) {
    for i, t := range pkg.Targets {
        if t.State != "blocked" {
            continue
        }
        reason, err := client.PackageBlockedReason(ctx, pkg.Project, t.Repo, t.Arch, pkg.Name)
        if err != nil {
            slog.Warn("poller: blocked reason", "pkg", pkg.Name, "repo", t.Repo, "arch", t.Arch, "err", err)
            continue
        }
        pkg.Targets[i].BlockedBy = reason
    }
}
```

`enrichBlockedTargets` is called unconditionally for packages with any blocked target, on every tick. Packages with no blocked targets have no blocked targets in their list, so the loop is a no-op at zero cost.

### MQ consumer (`mq/consumer.go`)

The consumer's `upsertPackage` helper currently:
1. Calls `store.UpsertPackageState`
2. Calls `hub.Notify`

Before step 1, enrich blocked targets using the same `enrichBlockedTargets` helper (shared between poller and consumer by moving it to a suitable location — either `obs/poller.go` as an exported helper, or a small shared internal function).

```go
func (c *Consumer) upsertPackage(ctx context.Context, pkg *model.Package) error {
    enrichBlockedTargets(ctx, c.obsClient, pkg)
    if err := store.UpsertPackageState(c.db, pkg); err != nil {
        return err
    }
    c.hub.Notify(hubpkg.PackageUpdate(pkg))
    return nil
}
```

`Consumer` gains an `obsClient *obs.Client` field; `NewConsumer` is updated to accept it.

**Sharing `enrichBlockedTargets`:** Export it from `obs` package as `obs.EnrichBlockedTargets(ctx, client, pkg)` so the consumer can call it without a circular import.

---

## Frontend

### Types (`types/api.ts`)

```ts
export interface Target {
  repo: string
  arch: string
  state: string
  blocked_by?: string
}
```

### PackageCard (`components/PackageCard.vue`)

The failing target row (currently a single-line flex row inside an `<a>`) gains a conditional second line for blocked targets with a reason. The `<a>` flex direction changes to `column` when `blocked_by` is present to accommodate the extra line.

Current target row (simplified):
```
[■] standard/x86_64          blocked  log ↗
```

After change for a blocked target with reason:
```
[■] standard/x86_64          blocked  log ↗
    waiting for perl to be built
```

Implementation: inside the `v-for` loop over `visibleFailing`, add a `<span>` below the existing content when `t.state === 'blocked' && t.blocked_by`:

```html
<a v-for="t in visibleFailing" :key="`${t.repo}-${t.arch}`"
   :style="{ display: 'flex', flexDirection: 'column', ... }"
>
  <!-- existing single-line row content -->
  <div style="display: flex; align-items: center; gap: 9px;">
    <span ...></span>
    <code>{{ t.repo }}/{{ t.arch }}</code>
    <span ...>{{ t.state }}</span>
    <span ...>log ↗</span>
  </div>
  <!-- blocking reason, shown only for blocked targets -->
  <span
    v-if="t.state === 'blocked' && t.blocked_by"
    style="font-family: var(--font-mono); font-size: 10.5px; color: var(--text-muted); padding-left: 17px;"
  >{{ t.blocked_by }}</span>
</a>
```

---

## Error handling

- **`PackageBlockedReason` API failure:** Log a warning and leave `BlockedBy` empty. The card renders the target row without a reason — the blocked state is still visible.
- **No `error` element in response:** `PackageBlockedReason` returns `("", nil)`. `BlockedBy` stays empty.
- **Consumer `obsClient` not yet configured:** `NewConsumer` will require the OBS client; `main.go` already constructs an `*obs.Client` before the consumer.

## What does not change

- All existing REST endpoints are untouched.
- `usePackages.ts`, `useRealtimeStream.ts`, and the SSE hub are untouched — `blocked_by` flows through as part of the serialized `Target` JSON.
- Non-blocked target rows in `PackageCard.vue` are visually unchanged.
- Packages that are blocked but where OBS returns no `error` string render without a reason (graceful degradation).
