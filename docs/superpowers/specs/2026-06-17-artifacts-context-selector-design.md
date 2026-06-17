# Artifacts Tab Context Selector Design

**Goal:** Add an independent context selector to the artifacts tab that lets users browse PPG builds, release projects (`isv:percona:ppg:releases:<version>`), and PR build projects.

**Architecture:** `ArtifactsPanel` becomes self-fetching — it no longer receives packages from `App.vue`. It maintains its own context state, fetches packages when the context changes, and derives available versions from what it fetches. The `ArtifactsVersionBar` grows a context selector (replacing the static OBS badge) that is a plain badge when only one context exists and a styled `<select>` when multiple are available. Two new backend routes serve releases packages and repos; one existing pattern is extended to PR repos.

**Tech Stack:** Go (chi router), SQLite via existing store layer, Vue 3 + TypeScript, existing `Context` interface from `types/api.ts`.

**User decisions (already made):**
- The artifacts tab context is independent from the builds tab context — switching in one tab does not affect the other.
- Three project types: PPG builds, Releases (`isv:percona:ppg:releases:<version>`), and PR builds.
- Context selector replaces the static OBS badge in `ArtifactsVersionBar`.
- New per-route repos endpoints (not a single generic endpoint): `/api/releases/ppg/{version}/repos` and `/api/pr/{pr}/{subproject}/{version}/repos`.
- `ArtifactsPanel` self-fetches; `App.vue` passes only `prGroups` for PR context discovery.

---

## Context Model

The existing `Context` interface (already in `types/api.ts`) is reused as-is:

```typescript
interface Context {
  label: string    // "PPG" | "Releases" | "PR #92 · ppg"
  apiBase: string  // "/api/products/ppg" | "/api/releases/ppg" | "/api/pr/pr-92/ppg"
  prefix: string   // "isv:percona:ppg" | "isv:percona:ppg:releases" | "isv:percona:PR:pr-92:ppg"
}
```

Fixed contexts (always present):
```typescript
const PPG_CONTEXT: Context = {
  label: 'PPG',
  apiBase: '/api/products/ppg',
  prefix: 'isv:percona:ppg',
}

const RELEASES_CONTEXT: Context = {
  label: 'Releases',
  apiBase: '/api/releases/ppg',
  prefix: 'isv:percona:ppg:releases',
}
```

PR contexts are derived from `prGroups` using the same logic as the builds tab: for each `(prSegment, subproject)` pair found in `prGroups`, produce:
```typescript
{
  label: `PR #${prNum} · ${subproject}`,
  apiBase: `/api/pr/${prSegment}/${subproject}`,
  prefix: `isv:percona:PR:${prSegment}:${subproject}`,
}
```
Sorted by PR number descending (newest first). Full context list: `[PPG_CONTEXT, RELEASES_CONTEXT, ...prContexts]`.

---

## Version Derivation

Available versions are derived from the fetched packages by reading the segment at `prefix.split(':').length` — the index immediately after the prefix:

| Context | Prefix segments | Version index | Example |
|---------|----------------|---------------|---------|
| PPG | 3 (`isv:percona:ppg`) | 3 | `isv:percona:ppg:17` → `"17"` |
| Releases | 4 (`isv:percona:ppg:releases`) | 4 | `isv:percona:ppg:releases:17` → `"17"` |
| PR | 5 (`isv:percona:PR:pr-92:ppg`) | 5 | `isv:percona:PR:pr-92:ppg:17` → `"17"` |

A segment is a version if it matches `/^\d+$/`. Non-numeric segments (`common`, `containers`, etc.) are ignored. Versions are sorted descending (newest first). The `localVersion` resets to `availableVersions[0]` when the context changes.

---

## Backend Changes

### New route: releases packages

```
GET /api/releases/ppg/{version}/packages
```

Handler builds prefix `isv:percona:ppg:releases` and calls `store.QueryPackages(db, prefix)`. No common packages are appended (releases are standalone, not mixed with common packages). Response format identical to the existing packages handler.

### New route: releases repos

```
GET /api/releases/ppg/{version}/repos
```

Same OBS repo-listing logic as the existing `reposHandler`, but with prefix `isv:percona:ppg:releases:{version}`.

### New route: PR repos

```
GET /api/pr/{pr}/{subproject}/{version}/repos
```

Same OBS repo-listing logic, prefix `isv:percona:PR:{pr}:{subproject}:{version}`.

### Router registration

```go
// in server.go
r.Route("/api/releases/ppg/{version}", func(r chi.Router) {
    r.Get("/packages", releasesPackagesHandler(db))
    r.Get("/repos",    releasesReposHandler(obsClient))
})

r.Route("/api/pr/{pr}/{subproject}/{version}", func(r chi.Router) {
    // existing:
    r.Get("/packages", prContextPackagesHandler(db))
    r.Get("/events",   prContextEventsHandler(db, h))
    // new:
    r.Get("/repos",    prReposHandler(obsClient))
})
```

---

## Frontend Changes

### `App.vue`

- Remove `:packages="rawPackages"` from `<ArtifactsPanel>`.
- Add `:pr-groups="prGroups"` to `<ArtifactsPanel>` (already available from `usePRPackages`).
- Remove `initialVersion` prop (ArtifactsPanel now self-manages).

### `ArtifactsPanel.vue`

Props change:
```typescript
defineProps<{
  prGroups: PRGroup[]   // replaces packages + availableVersions + initialVersion
}>()
```

New internal state:
```typescript
const artifactsContexts = computed<Context[]>(() => {
  const prContexts = derivePRContexts(props.prGroups)  // same logic as builds tab
  return [PPG_CONTEXT, RELEASES_CONTEXT, ...prContexts]
})

const selectedContext = ref<Context>(PPG_CONTEXT)
const artifactsPackages = ref<Package[]>([])
const localVersion = ref('17')
const artifactsLoading = ref(false)
```

`availableVersions` computed:
```typescript
const availableVersions = computed<string[]>(() => {
  const depth = selectedContext.value.prefix.split(':').length
  const found = new Set<string>()
  for (const pkg of artifactsPackages.value) {
    const seg = pkg.project.split(':')[depth]
    if (seg && /^\d+$/.test(seg)) found.add(seg)
  }
  return [...found].sort((a, b) => parseInt(b) - parseInt(a))
})
```

When `availableVersions` changes (after context switch), reset `localVersion` to `availableVersions[0]`.

`fetchPackages(context, version)`:
```typescript
async function fetchPackages(ctx: Context) {
  artifactsLoading.value = true
  try {
    const res = await fetch(`${ctx.apiBase}/${localVersion.value}/packages`)
    const data = await res.json() as { packages: Package[] }
    artifactsPackages.value = data.packages ?? []
  } catch {
    artifactsPackages.value = []
  } finally {
    artifactsLoading.value = false
  }
}
```

`fetchRepos` selects the right endpoint:
```typescript
async function fetchRepos(version: string) {
  const ctx = selectedContext.value
  let url: string
  if (ctx.apiBase.startsWith('/api/products/')) {
    url = `/api/products/ppg/${version}/repos`
  } else if (ctx.apiBase.startsWith('/api/releases/')) {
    url = `/api/releases/ppg/${version}/repos`
  } else {
    // /api/pr/{pr}/{subproject}
    url = `${ctx.apiBase}/${version}/repos`
  }
  // ... existing fetch + repos.value assignment
}
```

`onContextChange(ctx: Context)`:
1. Set `selectedContext.value = ctx`
2. Call `fetchPackages(ctx)` — version resets via `availableVersions` watcher
3. Call `fetchRepos(availableVersions.value[0] ?? localVersion.value)`

### `ArtifactsVersionBar.vue`

New props:
```typescript
contexts: Context[]
selectedContext: Context
```

New emit: `update:context`.

Template — replace the `<code class="obs-badge">` section:

```html
<!-- Single context: plain badge (no dropdown chrome) -->
<code v-if="contexts.length <= 1" class="obs-badge">
  {{ selectedContext.prefix }}:{{ version }}
</code>

<!-- Multiple contexts: styled select -->
<select
  v-else
  class="context-select"
  :value="selectedContext.apiBase"
  @change="emit('update:context', contexts.find(c => c.apiBase === ($event.target as HTMLSelectElement).value)!)"
>
  <option v-for="ctx in contexts" :key="ctx.apiBase" :value="ctx.apiBase">
    {{ ctx.label }}
  </option>
</select>
```

CSS for `.context-select`:
```css
.context-select {
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--text-secondary);
  background: var(--bg-muted);
  padding: 5px 10px;
  border-radius: 7px;
  border: none;
  cursor: pointer;
}
```

### `useArtifacts.ts`

New parameter: `contextPrefix: MaybeRef<string>` (replaces the hardcoded `isv:percona:ppg` assumption).

`packageRows` filter:
```typescript
const exactProject = `${toValue(contextPrefix)}:${ver}`
if (pkg.project !== exactProject) continue
```

`containerImages` filter:
```typescript
.filter(pkg =>
  pkg.scope === 'container' &&
  pkg.is_container !== false &&
  pkg.project.startsWith(`${toValue(contextPrefix)}:${ver}:`)
)
```

---

## Behaviour Details

- **Context label in bar**: the OBS badge shows `selectedContext.prefix + ':' + version` (full project path) when there is only one context; when there is a dropdown, the selected option label (`PPG`, `Releases`, `PR #92 · ppg`) is shown in the select, and the static badge is removed.
- **Loading state**: while `artifactsPackages` is being fetched, the package list shows a "Loading…" placeholder.
- **No releases yet**: if the releases project has no packages in the DB, `availableVersions` is empty when Releases is selected — the version selector is hidden and the package list is empty (same as current behaviour when no packages exist).
- **Repos missing**: if a context has no repos (e.g. a PR project that only has container packages), `repos` will be empty and the packages sub-tab shows the "Loading…" placeholder. This is acceptable for the initial implementation.
- **Version persistence**: switching context resets `localVersion` to the highest available version for that context. Switching back to PPG resets again (doesn't remember previous selection).
