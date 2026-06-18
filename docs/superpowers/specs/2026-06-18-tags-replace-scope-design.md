# Tags Replace Scope — Design Spec

## Goal

Replace the `scope` field on both `Package` and `Event` throughout the full stack (DB, Go model, API, frontend) with a `tags []string` field. Add `published` as a terminal success state. Fix the "all versions" query regression in `QueryBuildPackages`. Derive a `container` tag in the backend from `is_container`.

## Architecture

Tags are a string slice stored as JSON in both the `packages` and `events` tables. The backend classifier assigns project-level tags; the store layer splices in `"container"` when `is_container` becomes true. Events inherit the emitting package's tags. The frontend renders a fixed known set of tag pills (`ppg`, `common`, `container`) with AND multi-select semantics.

## Tech Stack

Go 1.25, SQLite/modernc, Vue 3 + TypeScript, chi router.

## User decisions (already made)

- Tag filtering is AND: a package/event must carry ALL selected tags to pass.
- `container` tag is synthesised by the backend store layer (Approach C), not the frontend.
- Tag pills are a known fixed set (always shown), not dynamically derived from loaded data.
- Known set: `ppg`, `common`, `container` (in that order).
- `scope` column is dropped from both `packages` and `events` DB tables.
- `published` is the new terminal success state (better than `succeeded`).

---

## Section 1 — DB Migration

New migration file (next sequence number after existing migrations):

```sql
ALTER TABLE packages DROP COLUMN scope;
ALTER TABLE events   ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
ALTER TABLE events   DROP COLUMN scope;
```

The `scope` column is dropped from `packages` (no longer written or read). `tags` is added to `events`; the `scope` column is then dropped from `events` too.

## Section 2 — Backend Model (`internal/model/types.go`)

- Remove `Scope Scope` from `Package`. `Tags []string` and `IsRelease bool` are already present.
- Remove `Scope Scope` from `Event`. Add `Tags []string \`json:"tags,omitempty"\``.
- The `Scope` type and constants (`ScopeCommon`, etc.) are removed entirely since nothing references them after this change.
- Add `'published'` to the `BuildState` documentation note (it is a `RollupState` constant already added in the backend workflow redesign).

## Section 3 — Backend Store (`internal/store/packages.go`)

**`UpsertPackageState`:**
- Drop `scope` from the INSERT column list and the ON CONFLICT SET clause.
- Extend the existing single-row pre-read to also fetch `is_container`:
  ```go
  var prevTargetsJSON string
  var prevIsContainer sql.NullInt64
  db.QueryRow(`SELECT targets_json, is_container FROM packages WHERE project = ? AND name = ?`,
      p.Project, p.Name).Scan(&prevTargetsJSON, &prevIsContainer)
  ```
- Before marshalling `p.Tags`, splice in `"container"` when either `(p.IsContainer != nil && *p.IsContainer)` OR `(prevIsContainer.Valid && prevIsContainer.Int64 != 0)`. Deduplicate with a seen-set to avoid double entries.
- This ensures the `container` tag is never lost when the poller or MQ consumer upserts a package with `p.IsContainer = nil` (before or between `PackageTypeTask` runs).
- The resulting `tagsJSON` is always non-empty (at minimum `[]`), so the existing CASE expression `WHEN excluded.tags != '[]' THEN excluded.tags ELSE tags` correctly preserves previously-set tags on no-op updates.

**`scanPackages`:** Remove the `scope` scan destination (`&p.Scope`).

**`QueryBuildPackages` — "all versions" fix:**

```go
func QueryBuildPackages(db *sql.DB, root, product, version string) ([]*model.Package, error) {
    cp := root + ":" + product + ":common"
    gp := root + ":common"
    var rows *sql.Rows
    var err error
    if version == "_" || version == "" {
        pp := root + ":" + product
        rows, err = db.Query(`SELECT`+packageSelectCols+`
            FROM packages
            WHERE ((project = ? OR project LIKE ? || ':%') AND is_release = 0)
               OR  (project = ? OR project LIKE ? || ':%')
            ORDER BY project, name`,
            pp, pp, gp, gp,
        )
    } else {
        vp := root + ":" + product + ":" + version
        rows, err = db.Query(`SELECT`+packageSelectCols+`
            FROM packages
            WHERE (project = ? OR project LIKE ? || ':%')
               OR (project = ? OR project LIKE ? || ':%')
               OR (project = ? OR project LIKE ? || ':%')
            ORDER BY project, name`,
            vp, vp, cp, cp, gp, gp,
        )
    }
    ...
}
```

The `"_"` branch queries the whole product subtree (`is_release = 0` excludes releases) plus global common. PPG common is included under the product prefix, so no third arm is needed. The specific-version branch is unchanged.

## Section 4 — Backend Store (`internal/store/events.go`)

**`AppendEvent`:** Replace `scope` with `tags` (JSON-encode `e.Tags`):

```go
_, err := db.Exec(`
    INSERT INTO events (id, type, tags, project, package, repo, arch, what, why, url, at, version)
    VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
    e.ID, string(e.Type), tagsJSON,
    e.Project, e.Package, nullStr(e.Repo), nullStr(e.Arch),
    e.What, e.Why, e.URL, e.At, e.Version,
)
```

**`QueryEvents`:** SELECT `tags` instead of `scope`; unmarshal into `e.Tags`.

## Section 5 — Backend Worker (`internal/worker/worker.go`)

`emitBuildEvents`: replace `Scope: pkg.Scope` with `Tags: pkg.Tags` on all four event structs (`EventBuildStarted`, `EventFailed`, `EventSucceeded`, `EventPublished`).

## Section 6 — Backend MQ Consumer (`internal/mq/consumer.go`)

- Remove `scope := kind.EventScope()` and all `Scope: scope` event field assignments.
- For events created from MQ messages, use `Tags: obs.ProjectTags(c.root, m.Project)`.
- `mergePackageTarget`: drop the `scope model.Scope` parameter; set `Tags: obs.ProjectTags(c.root, m.Project)` on the returned `model.Package`. Remove `Scope: scope` from the package literal.
- All callers of `mergePackageTarget` updated accordingly.

## Section 7 — Frontend Types (`frontend/src/types/api.ts`)

- Remove `PackageScope` type.
- `Package`: remove `scope: PackageScope`; add `tags?: string[]`, `is_release?: boolean`.
- `Event`: remove `scope: string`; add `tags?: string[]`.
- `BuildState`: add `'published'`.

## Section 8 — Frontend `usePackages.ts`

- `SEVERITY`: add `published: -1` (sorts last — least urgent).
- `sorted` filter: replace `pkg.scope !== 'release'` and the `:releases:` string check with `!pkg.is_release`.
- Remove `filterByScope`; add `filterByTags(tags: string[])`:

```ts
function filterByTags(tags: string[]): Package[] {
  if (tags.length === 0) return sorted.value
  return sorted.value.filter(p =>
    tags.every(t => (p.tags ?? []).includes(t))
  )
}
```

## Section 9 — Frontend `useEvents.ts`

`filterEvents` parameter `scopes: string[]` → `tags: string[]`. Filter logic:

```ts
if (tags.length > 0 && !tags.every(t => (e.tags ?? []).includes(t))) return false
```

## Section 10 — Frontend `ContextBar.vue`

- Remove `SCOPES` array and `activeScopes` prop; add `activeTags: string[]` prop.
- Rename emit `toggle-scope` → `toggle-tag`.
- Replace scope pill section with a known-set of three pills in fixed order: `ppg` ("PPG"), `common` ("Common"), `container` ("Container").
- A pill is greyed out (inactive border style) if `activeTags` does not include it; active (filled) if it does.
- No "All" pill — deselecting all tags returns to show-everything, same as before. The label reads "Tags" instead of "Scope".

## Section 11 — Frontend `App.vue`

- `activeScopes` → `activeTags`, `toggleScope` → `toggleTag`.
- `filterByScope(activeScopes.value)` → `filterByTags(activeTags.value)`.
- `filterEvents(activeScopes.value, ...)` → `filterEvents(activeTags.value, ...)`.
- ContextBar: `active-scopes` → `active-tags`, `@toggle-scope` → `@toggle-tag`.

## Section 12 — Frontend `useEventDisplay.ts`

Replace `SCOPE_STYLE` / `SCOPE_LABEL` with `TAG_STYLE` / `TAG_LABEL`:

```ts
export const TAG_STYLE: Record<string, string> = {
  ppg:       'background: var(--brand-purple-tint); color: var(--brand-purple);',
  common:    'background: var(--blocked-tint); color: var(--blocked);',
  container: 'background: var(--info-tint); color: var(--info);',
  pr:        'background: var(--warn-tint); color: var(--warn);',
  release:   'background: var(--ok-tint); color: var(--ok);',
}

export const TAG_LABEL: Record<string, string> = {
  ppg: 'PPG', common: 'Common', container: 'Container', pr: 'PR', release: 'Release',
}
```

## Section 13 — Frontend `EventRow.vue`

- Replace the single scope badge with one pill per tag in `event.tags`:

```html
<span
  v-for="tag in (props.event.tags ?? [])"
  :key="tag"
  :style="`font-size: 9px; font-weight: 700; ... ${TAG_STYLE[tag] ?? ''}`"
>{{ TAG_LABEL[tag] ?? tag }}</span>
```

- `displayVersion` container-detection: `event.scope === 'container'` → `(event.tags ?? []).includes('container')`.

## Section 14 — Frontend `PackageCard.vue`

- Add `published` to `STATE_COLOR` / `STATE_BG` / `STATE_LABEL`:
  - `STATE_COLOR.published = 'var(--ok)'`
  - `STATE_BG.published = 'var(--ok-tint)'`
  - `STATE_LABEL.published = 'Published'`
- Remove `SCOPE_LABEL` map; replace the scope badge with tag pills using `TAG_LABEL` / `TAG_STYLE` from `useEventDisplay.ts`. Render one pill per tag in `pkg.tags`.

## Section 15 — Frontend `HealthHeader.vue`

Treat `published` as a success state everywhere `succeeded` appears:

- `okCount`: `p.rollup_state === 'succeeded' || p.rollup_state === 'published'`
- `buildingCount` / other counts: unchanged (published is not in those buckets).
- `allGreen`: unchanged (uses `okCount === total`).

## Section 16 — Frontend `FailureBoard.vue`

- `failingPackages`: add `&& p.rollup_state !== 'published'`
- `okPackages`: add `|| p.rollup_state === 'published'`

## Section 17 — Cleanup

- Delete `frontend/src/components/ScopeChip.vue` (unused).
- `classifier.go`: `EventScope()` method can be removed once all callers in `mq/consumer.go` are replaced with `ProjectTags`.
