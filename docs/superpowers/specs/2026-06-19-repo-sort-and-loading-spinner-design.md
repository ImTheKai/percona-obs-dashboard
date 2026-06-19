# Repo Distro Sorting, Loading Spinner, and Binary-less Row Filtering — Design

**Goal:** Three UX improvements to the Artifacts tab Packages view: (1) replace the RPM/DEB sidebar section headers with distro-brand sections sorted by family and version, (2) show a centred spinner while packages and metadata are loading, and (3) hide package rows that have no binaries.

**Architecture:** Pure frontend changes — no backend or API changes. Distro detection lives as a utility function alongside the `RepoInfo` type. Loading state is threaded from the two existing fetch sites down to `PackagesSubTab` as a single `loading` prop.

**Tech stack:** Vue 3 + TypeScript.

**User decisions (already made):**
- Sidebar uses distro-brand sections (RHEL, openSUSE, Ubuntu, Debian) instead of RPM/DEB headers.
- RHEL family includes RHEL, Rocky Linux, Oracle Linux, CentOS, UBI.
- Loading style: centred spinner with label.
- Spinner covers both the packages fetch and the metadata fetch — disappears only when both resolve.
- Containers sub-tab: no spinner (fast enough, metadata not blocking).
- Package rows with no binaries are hidden; binaries presence is primary gate, state drives row behaviour once visible.

---

## Feature 1: Repo Distro Sorting

### `frontend/src/composables/useArtifacts.ts`

Export a pure utility function `distroGroup(repo: RepoInfo): string` that detects the distro family from `repo.name` (case-insensitive substring match):

| Pattern matches (any) | Group label |
|---|---|
| `rhel`, `centos`, `rocky`, `oracle`, `ubi` | `RHEL` |
| `opensuse`, `suse` | `openSUSE` |
| `ubuntu` | `Ubuntu` |
| `debian` | `Debian` |
| *(no match)* | `Other` |

The function is pure and easily unit-tested.

### `frontend/src/components/PackagesSubTab.vue`

Replace the `rpmRepos` / `debRepos` computed properties with four group computeds:

```
rhelRepos     = repos filtered by distroGroup === 'RHEL',   sorted by repo.name natural order
opensuseRepos = repos filtered by distroGroup === 'openSUSE', sorted by repo.name natural order
ubuntuRepos   = repos filtered by distroGroup === 'Ubuntu',  sorted by repo.name natural order
debianRepos   = repos filtered by distroGroup === 'Debian',  sorted by repo.name natural order
otherRepos    = repos not matched by any of the above,       sorted by repo.name natural order
```

The sidebar template renders the four sections in fixed order: RHEL → openSUSE → Ubuntu → Debian → Other (Other section only rendered if `otherRepos.length > 0`). Each section uses the same clickable-button pattern as the current RPM/DEB groups.

Within each group, repos are sorted by `repo.name` using `localeCompare` with `{ numeric: true }` so version numbers sort correctly (e.g., "RHEL 8" before "RHEL 9", "Ubuntu 20.04" before "Ubuntu 22.04").

The RPM/DEB filter chips at the top of the panel and the repo-auto-selection logic in `ArtifactsPanel` are unchanged.

---

## Feature 2: Packages Loading Spinner

### `frontend/src/composables/useArtifactMetadata.ts`

Add `isLoading: Ref<boolean>` to the return value. Set to `true` at the start of `fetchMetadata()` (after the abort/new-controller setup), `false` in a `finally` block so it always clears on success, error, or abort.

### `frontend/src/components/ArtifactsPanel.vue`

`artifactsLoading` already exists as an unused `ref<boolean>` (set to `true` before `fetchPackages()`, `false` after). Wire it up:

```typescript
const { enrichedPackageRows, enrichedContainerImages, isLoading: metadataLoading } =
  useArtifactMetadata(livePackageRows, liveContainerImages, computed(() => !isReleaseContext.value))

const isLoading = computed(() => artifactsLoading.value || metadataLoading.value)
```

Pass `isLoading` to `PackagesSubTab`:

```html
<PackagesSubTab :loading="isLoading" ... />
```

### `frontend/src/components/PackagesSubTab.vue`

Add `loading: boolean` prop (default `false`). In the template, wrap the package list area:

```html
<div v-if="loading" class="packages-loading">
  <div class="spinner"></div>
  <span class="loading-label">Fetching packages…</span>
</div>
<div v-else>
  <!-- existing package rows -->
</div>
```

The spinner replaces only the package list area — the repo sidebar remains visible throughout (repos and packages arrive together, so the sidebar populates at the same time the spinner would disappear anyway; showing the sidebar during load gives the user something to see).

CSS for the spinner and loading state is scoped to `PackagesSubTab`. The `spinner` animation (`border-top` rotation) is consistent with the style shown in the brainstorming mockup.

---

## Feature 3: Hide Package Rows with No Binaries

### `frontend/src/composables/useArtifacts.ts` or `frontend/src/components/ArtifactsPanel.vue`

After metadata enrichment, filter `packageRows` to exclude rows that have no binaries. A row is considered to **have binaries** if ANY of the following is true:

- `row.binaries && row.binaries.length > 0` — metadata fetch returned binary files
- `row.state === 'succeeded'` — OBS reports the build succeeded (binaries exist even if metadata hasn't loaded yet or failed silently)
- `row.published === true` — the package has been published (implies a prior successful build)

A row is **hidden** when none of the above applies — i.e., the build is in a non-terminal or failed state (`failed`, `building`, `scheduled`, `blocked`, `disabled`, `excluded`, `broken`, `unresolvable`) and the metadata fetch returned no binaries for it.

The filter is applied in `ArtifactsPanel.vue` after the `enrichedPackageRows` computed, so it is always evaluated against fully-enriched data:

```typescript
const visiblePackageRows = computed(() =>
  enrichedPackageRows.value.filter(row =>
    (row.binaries && row.binaries.length > 0) ||
    row.state === 'succeeded' ||
    row.published
  )
)
```

`visiblePackageRows` is passed to `PackagesSubTab` instead of `enrichedPackageRows`. The `PackagesSubTab` component itself needs no changes for this feature — it receives only the rows it should show.

This filter applies only in the live/DEV/PR context. Release-context package rows come from a pre-filtered API response and are unaffected.

---

## Scope

- No backend changes.
- No changes to the Containers sub-tab.
- No changes to the release-context artifact view (release artifacts are pre-computed and already filtered).
- The RPM/DEB filter chips remain — they filter the package list, not the sidebar grouping.
- Binary-less row filtering applies only in live/DEV/PR context.
