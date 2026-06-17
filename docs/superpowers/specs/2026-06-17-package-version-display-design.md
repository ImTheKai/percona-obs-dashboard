# Package Version Display Design

## Goal

Show the version of each package in the PackageCard and in relevant event log entries (succeeded and published events), fetched from OBS and stored on the package model.

## User decisions

- Backend stores full `versrel` (e.g. `17.5-1`); frontend derives display format from scope.
- RPMs: show version only — strip the release suffix (`17.5-1` → `17.5`), grey badge.
- Containers: show full versrel as image tag (`Tag: 8.0.38-10.1`), purple badge.
- Version badge placement in PackageCard: meta row, between scope tag and project path (Option B).
- Version in event log: only `succeeded` and `published` events; other event types leave version empty.
- Fetching strategy: `VersionTask` in the regular worker pipeline (Approach A).

## OBS API

Endpoint: `GET /build/{project}/_result?view=versrel&package={pkg}`

Returns an XML document with one `<result>` element per build target. Each contains a `<status>` element with a `versrel` attribute (e.g. `versrel="17.5-1"`). We take the first non-empty `versrel` across all targets, since the version is the same for all targets of a given package.

If OBS returns no populated `versrel` (package not yet built), the task is a no-op.

## Backend

### Model (`backend/internal/model/types.go`)

Add `Version string` to the `Package` struct.

### Database (`backend/internal/store/db.go`)

Add `version TEXT NOT NULL DEFAULT ''` to the `packages` table. Applied with:

```sql
ALTER TABLE packages ADD COLUMN version TEXT NOT NULL DEFAULT '';
```

Run on startup, wrapped to silently ignore the error if the column already exists.

### OBS client (`backend/internal/obs/client.go`)

Add method:

```go
func (c *Client) PackageVersionResult(project, pkg string) (string, error)
```

Calls `/build/{project}/_result?view=versrel&package={pkg}`, parses the XML response, and returns the `versrel` attribute from the first target entry that has a non-empty value. Returns `""` if none found.

### Worker task (`backend/internal/obs/tasks.go`)

New `VersionTask` struct implementing the task interface:

```go
type VersionTask struct{}

func (t VersionTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
    versrel, err := client.PackageVersionResult(pkg.Project, pkg.Name)
    if err != nil || versrel == "" || versrel == pkg.Version {
        return err
    }
    pkg.Version = versrel
    // persist to DB
    return nil
}
```

Runs after `BuildStateTask` in the pipeline so target state is already up to date.

### Event emission

When the worker emits a `succeeded` or `published` event, populate the event's `version` field from `pkg.Version` at the time of emission. All other event types leave `version` empty.

### SSE broadcast

No changes needed. The existing package-state broadcast sends the full `Package` struct; adding `Version` to the struct propagates it automatically.

## Frontend

### API types (`frontend/src/types/api.ts`)

```ts
interface Package {
  // existing fields ...
  version?: string
}

interface Event {
  // existing fields ...
  version?: string
}
```

### Display helper (`frontend/src/composables/useEventDisplay.ts`)

```ts
export function displayVersion(version: string | undefined, scope: string): string | null {
  if (!version) return null
  if (scope === 'container') return 'Tag: ' + version
  return version.replace(/-[^-]+$/, '')  // strip release suffix: "17.5-1" → "17.5"
}
```

### PackageCard (`frontend/src/components/PackageCard.vue`)

In the meta row (scope tag + project path), insert the version badge between the two when `displayVersion` returns non-null:

```html
<span class="scope-tag">…</span>
<span
  v-if="displayVersion(pkg.version, pkg.scope)"
  class="ver-badge"
  :class="{ tag: pkg.scope === 'container' }"
>{{ displayVersion(pkg.version, pkg.scope) }}</span>
<code class="project-path">…</code>
```

Badge styles: grey (`background: var(--bg-muted); color: var(--text-secondary)`) for RPMs; purple (`background: var(--brand-purple-tint); color: var(--brand-purple)`) for containers.

### EventRow (`frontend/src/components/EventRow.vue`)

In the meta row at the bottom of each event row, insert the same badge when `event.version` is set:

```html
<span class="scope-tag">…</span>
<span
  v-if="displayVersion(event.version, event.scope)"
  class="ver-badge"
  :class="{ tag: event.scope === 'container' }"
>{{ displayVersion(event.version, event.scope) }}</span>
<code class="project-path">…</code>
```

### PackageEventGroup (`frontend/src/components/PackageEventGroup.vue`)

Same treatment as EventRow: version badge in the group header's meta row (driven by `head.version`), and in each expanded child row (driven by `event.version`).
