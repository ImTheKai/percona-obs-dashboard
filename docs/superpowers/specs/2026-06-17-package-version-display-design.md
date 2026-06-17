# Package Version Display Design

## Goal

Show the version of each package in the PackageCard and in relevant event log entries (succeeded and published events), fetched from OBS and stored on the package model.

## User decisions

- The package model gains an `IsContainer bool` field; detection runs as a dedicated worker task.
- RPMs/DEBs: fetch `versrel` via the build result API, display the version part only (`17.5-1` → `17.5`), grey badge.
- Containers: fetch image tags via the binary artifact + containerinfo API, display the most specific tag (`18.4-1-1.7`), purple badge.
- Version badge placement in PackageCard: meta row, between scope tag and project path.
- Version in event log: only `succeeded` and `published` events; other event types leave version empty.
- Fetching strategy: worker pipeline tasks (Approach A). Container support is split into two separate tasks: one for type detection, one for tag fetching.

## OBS APIs

### 1. Container type detection

`GET /source/{project}/{pkg}?view=info&repository=images`

Returns XML containing `<filename>` elements listing the package's source files. If any filename is `Dockerfile` or ends in `.kiwi`, the package is a container image. Otherwise it is an RPM/DEB package.

### 2. RPM/DEB version

`GET /build/{project}/_result?view=versrel&package={pkg}`

Returns XML structured like:

```xml
<resultlist state="...">
  <result project="isv:percona:ppg:17" repository="UBI_9" arch="x86_64" code="published" state="published">
    <status package="percona-pg_tde" code="succeeded" versrel="17.5-1"/>
  </result>
  ...
</resultlist>
```

The `versrel` attribute is on `<status>`. We take the first non-empty value across all targets (version is the same for all). Returns empty string if package not yet built.

### 3. Container image tags — binary artifact list

`GET /build/{project}/{repo}/{arch}/{pkg}`

Returns an XML directory listing of binary artifacts produced by the build. We scan the list for a file ending in `.containerinfo`.

### 4. Container image tags — containerinfo file

`GET /build/{project}/{repo}/{arch}/{pkg}/{containerinfo-filename}`

Returns a JSON document like:

```json
{
  "version": "18.4-1",
  "tags": [
    "percona-distribution-postgresql:18.4-1-1.7",
    "percona-distribution-postgresql:18.4-1",
    "percona-distribution-postgresql:18.4",
    "percona-distribution-postgresql:18"
  ],
  ...
}
```

We store `tags[0]` (the most specific tag, e.g. `18.4-1-1.7`) as the package version. If no `.containerinfo` file exists in the artifact list (build not yet complete), the task is a no-op.

## Backend

### Model (`backend/internal/model/types.go`)

Add two fields to the `Package` struct:

```go
IsContainer bool   // true if the package produces a container image
Version     string // versrel for RPMs; most specific image tag for containers
```

### Database (`backend/internal/store/db.go`)

Add two columns to the `packages` table, each applied idempotently on startup (silently ignored if already present):

```sql
ALTER TABLE packages ADD COLUMN is_container INTEGER NOT NULL DEFAULT 0;
ALTER TABLE packages ADD COLUMN version TEXT NOT NULL DEFAULT '';
```

### OBS client (`backend/internal/obs/client.go`)

Four new methods:

```go
// Returns true if any source file is a Dockerfile or *.kiwi.
func (c *Client) PackageIsContainer(project, pkg string) (bool, error)

// Returns versrel for the first built target, or "" if not yet built.
func (c *Client) PackageVersionResult(project, pkg string) (string, error)

// Returns the filename of the .containerinfo artifact, or "" if not present.
func (c *Client) PackageContainerInfoFilename(project, repo, arch, pkg string) (string, error)

// Fetches and parses the containerinfo JSON; returns tags[0] or "" if empty.
func (c *Client) PackageContainerTags(project, repo, arch, pkg, filename string) (string, error)
```

### Worker tasks (`backend/internal/obs/tasks.go`)

Three new task structs, all added to the regular pipeline:

**`PackageTypeTask`** — runs first; detects container vs RPM/DEB:

```go
type PackageTypeTask struct{}

func (t PackageTypeTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
    isContainer, err := client.PackageIsContainer(pkg.Project, pkg.Name)
    if err != nil || isContainer == pkg.IsContainer { return err }
    pkg.IsContainer = isContainer
    // persist to DB
    return nil
}
```

**`VersionTask`** — runs after `PackageTypeTask`; skipped for containers:

```go
type VersionTask struct{}

func (t VersionTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
    if pkg.IsContainer { return nil }
    versrel, err := client.PackageVersionResult(pkg.Project, pkg.Name)
    if err != nil || versrel == "" || versrel == pkg.Version { return err }
    pkg.Version = versrel
    // persist to DB
    return nil
}
```

**`ContainerTagsTask`** — runs after `PackageTypeTask`; skipped for non-containers. Uses the first available target (repo/arch) from `pkg.Targets`:

```go
type ContainerTagsTask struct{}

func (t ContainerTagsTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
    if !pkg.IsContainer || len(pkg.Targets) == 0 { return nil }
    target := pkg.Targets[0]
    filename, err := client.PackageContainerInfoFilename(pkg.Project, target.Repo, target.Arch, pkg.Name)
    if err != nil || filename == "" { return err }
    tag, err := client.PackageContainerTags(pkg.Project, target.Repo, target.Arch, pkg.Name, filename)
    if err != nil || tag == "" || tag == pkg.Version { return err }
    pkg.Version = tag
    // persist to DB
    return nil
}
```

### Pipeline order

```
PackageTypeTask → BuildStateTask → PublishStateTask → VersionTask → ContainerTagsTask → BlockedReasonTask → BuildReasonTask
```

`PackageTypeTask` runs first so all subsequent tasks can branch on `pkg.IsContainer`.

### Event emission

When the worker emits a `succeeded` or `published` event, populate the event's `version` field from `pkg.Version` at the time of emission. All other event types leave `version` empty.

### SSE broadcast

No changes needed. The existing package-state broadcast sends the full `Package` struct; the new fields propagate automatically.

## Frontend

### API types (`frontend/src/types/api.ts`)

```ts
interface Package {
  // existing fields ...
  isContainer?: boolean
  version?: string
}

interface Event {
  // existing fields ...
  version?: string
}
```

### Display helper (`frontend/src/composables/useEventDisplay.ts`)

```ts
export function displayVersion(version: string | undefined, isContainer: boolean): string | null {
  if (!version) return null
  if (isContainer) return 'Tag: ' + version          // e.g. "Tag: 18.4-1-1.7"
  return version.replace(/-[^-]+$/, '')               // "17.5-1" → "17.5"
}
```

Note: `EventRow` and `PackageEventGroup` receive `event.version` but not `event.isContainer` directly. Since container events always have the scope `container`, use `scope === 'container'` as the `isContainer` proxy in those components:

```ts
displayVersion(event.version, event.scope === 'container')
```

### PackageCard (`frontend/src/components/PackageCard.vue`)

In the meta row (scope tag + project path), insert the version badge between the two when `displayVersion` returns non-null:

```html
<span class="scope-tag">…</span>
<span
  v-if="displayVersion(pkg.version, pkg.isContainer ?? false)"
  class="ver-badge"
  :class="{ tag: pkg.isContainer }"
>{{ displayVersion(pkg.version, pkg.isContainer ?? false) }}</span>
<code class="project-path">…</code>
```

Badge styles: grey (`background: var(--bg-muted); color: var(--text-secondary)`) for RPMs; purple (`background: var(--brand-purple-tint); color: var(--brand-purple)`) for containers.

### EventRow (`frontend/src/components/EventRow.vue`)

In the meta row at the bottom of each event row:

```html
<span class="scope-tag">…</span>
<span
  v-if="displayVersion(event.version, event.scope === 'container')"
  class="ver-badge"
  :class="{ tag: event.scope === 'container' }"
>{{ displayVersion(event.version, event.scope === 'container') }}</span>
<code class="project-path">…</code>
```

### PackageEventGroup (`frontend/src/components/PackageEventGroup.vue`)

Same treatment as EventRow: version badge in the group header's meta row (driven by `head.version` and `head.scope`), and in each expanded child row.
