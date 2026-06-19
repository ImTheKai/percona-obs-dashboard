# Artifacts Tab UX Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Three UX improvements to the Artifacts tab Packages view: replace RPM/DEB sidebar headers with distro-brand sections, show a loading spinner while packages and metadata fetch, and hide package rows that have no binaries.

**Architecture:** Pure frontend changes — no backend or API changes. A new exported `distroGroup()` utility in `useArtifacts.ts` drives sidebar grouping. Loading state is threaded from `useArtifactMetadata` (`isLoading: Ref<boolean>`) through `ArtifactsPanel` (`artifactsLoading || metadataLoading`) down to `PackagesSubTab` as a single `loading` prop. Row visibility is filtered in `ArtifactsPanel`'s `packageRows` computed before the rows reach `PackagesSubTab`.

**Tech Stack:** Vue 3 + TypeScript, no new dependencies.

**User decisions (already made):**
- Sidebar uses distro-brand sections: RHEL → openSUSE → Ubuntu → Debian → Other (fixed order).
- RHEL family includes RHEL, Rocky Linux, Oracle Linux, CentOS, UBI (substring-match on `repo.name`).
- Loading style: centred spinner with label — replaces the package list area while loading, sidebar stays visible.
- Spinner covers both the packages fetch AND the metadata fetch; disappears only when both resolve.
- Containers sub-tab: no spinner changes.
- Binary-less rows: hidden (not rendered). Binaries presence is the sole visibility gate; `state !== 'succeeded'` greys out and disables a visible row.

---

## File Map

| File | Change |
|---|---|
| `frontend/src/composables/useArtifacts.ts` | Add exported `distroGroup(repo: RepoInfo): string` after `deriveBaseOs` |
| `frontend/src/composables/useArtifactMetadata.ts` | Add `isLoading: Ref<boolean>` to return; wrap body in try/finally |
| `frontend/src/components/ArtifactsPanel.vue` | Destructure `isLoading: metadataLoading`, add combined `isLoading` computed, add `visiblePackageRows` filter, pass `:loading` to `PackagesSubTab` |
| `frontend/src/components/PackagesSubTab.vue` | Replace `rpmRepos`/`debRepos` with 5 distro group computeds; overhaul sidebar template; add `loading` prop; add spinner; update pkg-row button disabled logic |

---

## Task 1: `distroGroup` utility and distro-brand sidebar sections

**Goal:** Replace the RPM/DEB sidebar grouping with RHEL/openSUSE/Ubuntu/Debian/Other sections, sorted by name within each group.

**Files:**
- Modify: `frontend/src/composables/useArtifacts.ts`
- Modify: `frontend/src/components/PackagesSubTab.vue`

**Acceptance Criteria:**
- [ ] `distroGroup(repo)` is exported from `useArtifacts.ts` and maps repo names correctly
- [ ] Sidebar shows RHEL, openSUSE, Ubuntu, Debian sections (Other only if non-empty)
- [ ] Each section is sorted by `repo.name` with numeric collation
- [ ] Repos that don't match any distro pattern appear under "Other"
- [ ] `rpm-label` / `deb-label` CSS classes are gone; single `.group-label` style remains
- [ ] `cd frontend && npm run build` exits 0

**Verify:** `cd frontend && npm run build` → exits 0 with no errors

**Steps:**

- [ ] **Step 1: Add `distroGroup` to `useArtifacts.ts`**

  After the `deriveBaseOs` function (line 63), add:

  ```typescript
  export function distroGroup(repo: RepoInfo): string {
    const name = repo.name.toLowerCase()
    if (/rhel|centos|rocky|oracle|ubi/.test(name)) return 'RHEL'
    if (/opensuse|suse/.test(name)) return 'openSUSE'
    if (/ubuntu/.test(name)) return 'Ubuntu'
    if (/debian/.test(name)) return 'Debian'
    return 'Other'
  }
  ```

- [ ] **Step 2: Replace `rpmRepos`/`debRepos` computeds in `PackagesSubTab.vue`**

  In `<script setup>`, add this import after the existing type import (line 3):

  ```typescript
  import { distroGroup } from '../composables/useArtifacts'
  ```

  Replace lines 20–21:
  ```typescript
  const rpmRepos = computed(() => props.repos.filter(r => r.type === 'rpm'))
  const debRepos = computed(() => props.repos.filter(r => r.type === 'deb'))
  ```

  With:
  ```typescript
  const rhelRepos = computed(() =>
    props.repos.filter(r => distroGroup(r) === 'RHEL')
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
  )
  const opensuseRepos = computed(() =>
    props.repos.filter(r => distroGroup(r) === 'openSUSE')
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
  )
  const ubuntuRepos = computed(() =>
    props.repos.filter(r => distroGroup(r) === 'Ubuntu')
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
  )
  const debianRepos = computed(() =>
    props.repos.filter(r => distroGroup(r) === 'Debian')
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
  )
  const otherRepos = computed(() =>
    props.repos.filter(r => distroGroup(r) === 'Other')
      .sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }))
  )
  ```

- [ ] **Step 3: Replace the sidebar template sections**

  In `<template>`, replace the two `<template v-if>` blocks for `rpmRepos` and `debRepos` (lines 143–163) with five group sections:

  ```html
  <template v-if="rhelRepos.length > 0">
    <div class="group-label">RHEL</div>
    <button
      v-for="repo in rhelRepos"
      :key="repo.obs"
      class="sidebar-row"
      :class="{ active: selectedRepo?.obs === repo.obs }"
      @click="emit('update:art-repo', repo.obs)"
    >{{ repo.name }}</button>
  </template>

  <template v-if="opensuseRepos.length > 0">
    <div class="group-label">openSUSE</div>
    <button
      v-for="repo in opensuseRepos"
      :key="repo.obs"
      class="sidebar-row"
      :class="{ active: selectedRepo?.obs === repo.obs }"
      @click="emit('update:art-repo', repo.obs)"
    >{{ repo.name }}</button>
  </template>

  <template v-if="ubuntuRepos.length > 0">
    <div class="group-label">Ubuntu</div>
    <button
      v-for="repo in ubuntuRepos"
      :key="repo.obs"
      class="sidebar-row"
      :class="{ active: selectedRepo?.obs === repo.obs }"
      @click="emit('update:art-repo', repo.obs)"
    >{{ repo.name }}</button>
  </template>

  <template v-if="debianRepos.length > 0">
    <div class="group-label">Debian</div>
    <button
      v-for="repo in debianRepos"
      :key="repo.obs"
      class="sidebar-row"
      :class="{ active: selectedRepo?.obs === repo.obs }"
      @click="emit('update:art-repo', repo.obs)"
    >{{ repo.name }}</button>
  </template>

  <template v-if="otherRepos.length > 0">
    <div class="group-label">Other</div>
    <button
      v-for="repo in otherRepos"
      :key="repo.obs"
      class="sidebar-row"
      :class="{ active: selectedRepo?.obs === repo.obs }"
      @click="emit('update:art-repo', repo.obs)"
    >{{ repo.name }}</button>
  </template>
  ```

- [ ] **Step 4: Update `.group-label` CSS — replace `.rpm-label` and `.deb-label`**

  In `<style scoped>`, replace the `.group-label`, `.rpm-label`, and `.deb-label` blocks (lines 304–320):

  ```css
  .group-label {
    padding: 8px 16px;
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-muted);
    border-top: 1px solid var(--border);
    border-bottom: 1px solid var(--border);
  }
  ```

  (Remove `.rpm-label { ... }` and `.deb-label { ... }` entirely.)

- [ ] **Step 5: Build and commit**

  ```bash
  cd frontend && npm run build
  ```
  Expected: exits 0 with no TypeScript or build errors.

  ```bash
  git add frontend/src/composables/useArtifacts.ts frontend/src/components/PackagesSubTab.vue
  git commit -s -m "feat(frontend): replace RPM/DEB sidebar with distro-brand sections"
  ```

---

## Task 2: Loading spinner for packages fetch

**Goal:** Show a centred spinner in the package list area while `artifactsLoading` or `metadataLoading` is true; disappears only when both resolves.

**Files:**
- Modify: `frontend/src/composables/useArtifactMetadata.ts`
- Modify: `frontend/src/components/ArtifactsPanel.vue`
- Modify: `frontend/src/components/PackagesSubTab.vue`

**Acceptance Criteria:**
- [ ] `useArtifactMetadata` returns `isLoading: Ref<boolean>` (true during fetch, false in `finally`)
- [ ] `ArtifactsPanel` combines `artifactsLoading || metadataLoading` into a single `isLoading` computed and passes it as `:loading` to `PackagesSubTab`
- [ ] `PackagesSubTab` accepts a `loading?: boolean` prop and shows the spinner when true
- [ ] Spinner replaces the package list area (`.pkg-list`); repo sidebar remains visible
- [ ] `cd frontend && npm run build` exits 0

**Verify:** `cd frontend && npm run build` → exits 0 with no errors

**Steps:**

- [ ] **Step 1: Add `isLoading` to `useArtifactMetadata.ts`**

  After line 38 (`const metadataMap = ref(new Map<string, ArtifactMetadataResult>())`), add:

  ```typescript
  const isLoading = ref(false)
  ```

  Replace the `fetchMetadata` function body so that `isLoading` is set at the start and cleared in a `finally`:

  ```typescript
  async function fetchMetadata() {
    controller?.abort()
    controller = new AbortController()
    const signal = controller.signal
    isLoading.value = true

    try {
      if (!isLiveContext.value) {
        metadataMap.value = new Map()
        return
      }

      const items: ArtifactMetadataItem[] = [
        ...packageRows.value.map(row => ({
          project: row.project,
          name: row.name,
          repo: row.repo.obs,
          arch: row.arch,
          kind: 'package' as const,
        })),
        ...containerImages.value.map(img => ({
          project: img.project,
          name: img.imageName,
          repo: '',
          arch: '',
          kind: 'container' as const,
        })),
      ]

      if (items.length === 0) {
        metadataMap.value = new Map()
        return
      }

      const res = await fetch('/api/artifacts/metadata', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ items }),
        signal,
      })
      if (!res.ok || signal.aborted) return
      const data = await res.json() as { items: ArtifactMetadataResult[] }
      const newMap = new Map<string, ArtifactMetadataResult>()
      for (const result of data.items) {
        newMap.set(metaKey(result.project, result.name, result.repo, result.arch, result.kind), result)
      }
      metadataMap.value = newMap
    } catch {
      // metadata is best-effort; silently ignore network/parse errors (including AbortError)
    } finally {
      isLoading.value = false
    }
  }
  ```

  Update the return type annotation and the return statement:

  ```typescript
  export function useArtifactMetadata(
    packageRows: Ref<PackageRow[]>,
    containerImages: Ref<ContainerImage[]>,
    isLiveContext: Ref<boolean>,
  ): {
    enrichedPackageRows: ComputedRef<PackageRow[]>
    enrichedContainerImages: ComputedRef<ContainerImage[]>
    isLoading: Ref<boolean>
  } {
  ```

  ```typescript
  return { enrichedPackageRows, enrichedContainerImages, isLoading }
  ```

- [ ] **Step 2: Wire `isLoading` in `ArtifactsPanel.vue`**

  Change line 240 — destructure `isLoading: metadataLoading` from `useArtifactMetadata`:

  ```typescript
  const { enrichedPackageRows, enrichedContainerImages, isLoading: metadataLoading } = useArtifactMetadata(
    livePackageRows,
    liveContainerImages,
    computed(() => !isReleaseContext.value),
  )
  ```

  After this line, add:

  ```typescript
  const isLoading = computed(() => artifactsLoading.value || metadataLoading.value)
  ```

  In the `<template>`, add `:loading="isLoading"` to `<PackagesSubTab>`:

  ```html
  <PackagesSubTab
    v-if="artifactsTab === 'packages'"
    :package-rows="packageRows"
    :repos="repos"
    :selected-repo="selectedRepo"
    :version="localVersion"
    :art-arch="artArch"
    :copied-key="copiedKey"
    :loading="isLoading"
    @update:art-repo="artRepoObs = $event"
    @update:art-arch="artArch = $event as 'x86_64' | 'aarch64'"
    @copy="onCopy"
  />
  ```

- [ ] **Step 3: Add `loading` prop and spinner to `PackagesSubTab.vue`**

  In `<script setup>`, add `loading?: boolean` to `defineProps`:

  ```typescript
  const props = defineProps<{
    packageRows: PackageRow[]
    repos: RepoInfo[]
    selectedRepo: RepoInfo | null
    version: string
    artArch: string
    copiedKey: string | null
    loading?: boolean
  }>()
  ```

  In `<template>`, inside the `<!-- Package list card -->` div, replace `<div class="pkg-list">` with a conditional block. The full replaced section (currently lines 208–271) becomes:

  ```html
  <!-- Package list card -->
  <div class="pkg-card" v-if="showPackageList">
    <div class="pkg-card-header">
      <span class="pkg-card-title">Packages</span>
      <span class="pkg-card-subtitle">
        {{ packageRows.length }} available
        <template v-if="selectedRepo"> · {{ selectedRepo.name }} / {{ artArch }}</template>
      </span>
    </div>
    <div v-if="loading" class="packages-loading">
      <div class="spinner"></div>
      <span class="loading-label">Fetching packages…</span>
    </div>
    <div v-else class="pkg-list">
      <div
        v-for="row in packageRows"
        :key="rowKey(row)"
        class="pkg-group"
      >
        <!-- Package header row (click to expand) -->
        <button
          class="pkg-row"
          :class="{ expanded: expanded[rowKey(row)] }"
          @click="row.binariesAvailable && row.state === 'succeeded' ? toggleRow(row) : undefined"
          :disabled="!row.binariesAvailable || row.state !== 'succeeded'"
          :title="!row.binariesAvailable ? 'No target binaries available' : (row.state !== 'succeeded' ? 'Not built' : 'Click to show binaries')"
        >
          <span class="expand-glyph">{{ row.binariesAvailable ? (expanded[rowKey(row)] ? '▼' : '▶') : '' }}</span>
          <code class="pkg-name">{{ row.name }}</code>
          <code v-if="row.version" class="pkg-version">{{ row.version }}</code>
          <span v-if="row.builtAt" class="pkg-built-at">{{ formatArtifactTime(row.builtAt) }}</span>
          <span v-if="row.isRebuilding" class="status-badge status-rebuilding">Rebuilding</span>
          <span class="status-badge" :class="row.published ? 'status-published' : stateClass(row.state)">
            {{ row.published ? 'Published' : stateLabel(row.state) }}
          </span>
        </button>

        <!-- Binary list (expanded) -->
        <div v-if="expanded[rowKey(row)]" class="binary-list">
          <div v-if="binaryCache[rowKey(row)] === 'loading'" class="binary-loading">
            Loading…
          </div>
          <div v-else-if="binaryCache[rowKey(row)] === 'error'" class="binary-error">
            Failed to load binaries.
          </div>
          <template v-else-if="Array.isArray(binaryCache[rowKey(row)])">
            <div
              v-for="binary in (binaryCache[rowKey(row)] as ArtifactBinary[])"
              :key="binary.filename"
              class="binary-row"
            >
              <div class="binary-details">
                <code class="binary-name">{{ binary.filename }}</code>
                <span v-if="binary.built_at" class="binary-built-at">{{ formatArtifactTime(binary.built_at) }}</span>
              </div>
              <a
                class="download-btn"
                :href="downloadUrl(row, binary.filename)"
                target="_blank"
              >&#x2193; Download</a>
            </div>
            <div v-if="(binaryCache[rowKey(row)] as ArtifactBinary[]).length === 0" class="binary-empty">
              No distributable binaries.
            </div>
          </template>
        </div>
      </div>
    </div>
  </div>
  ```

  In `<style scoped>`, add the spinner styles after `.pkg-list { ... }`:

  ```css
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .packages-loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 48px 0;
    gap: 12px;
  }

  .spinner {
    width: 28px;
    height: 28px;
    border: 3px solid var(--border);
    border-top-color: var(--brand-purple);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .loading-label {
    font-size: 13px;
    color: var(--text-muted);
  }
  ```

- [ ] **Step 4: Build and commit**

  ```bash
  cd frontend && npm run build
  ```
  Expected: exits 0 with no TypeScript or build errors.

  ```bash
  git add frontend/src/composables/useArtifactMetadata.ts frontend/src/components/ArtifactsPanel.vue frontend/src/components/PackagesSubTab.vue
  git commit -s -m "feat(frontend): show loading spinner while packages and metadata are fetching"
  ```

---

## Task 3: Hide binary-less rows; grey out non-succeeded rows

**Goal:** Package rows with no binaries are hidden; rows with binaries but `state !== 'succeeded'` are greyed out and non-interactive.

**Files:**
- Modify: `frontend/src/components/ArtifactsPanel.vue`
- Modify: `frontend/src/components/PackagesSubTab.vue`

**Acceptance Criteria:**
- [ ] Non-release `packageRows` in `ArtifactsPanel` only contains rows where `row.binaries && row.binaries.length > 0`
- [ ] Release-context rows are unaffected (no filter applied there)
- [ ] `PackagesSubTab` pkg-row button is disabled when `row.state !== 'succeeded'`
- [ ] Expand glyph is empty string when `row.state !== 'succeeded'`
- [ ] Title tooltip says "Package is not in succeeded state" when disabled
- [ ] `cd frontend && npm run build` exits 0

**Verify:** `cd frontend && npm run build` → exits 0 with no errors

**Steps:**

- [ ] **Step 1: Filter binary-less rows in `ArtifactsPanel.vue`**

  In the `packageRows` computed (currently line 247), change the non-release branch from:

  ```typescript
  if (!isReleaseContext.value) return enrichedPackageRows.value
  ```

  To:

  ```typescript
  if (!isReleaseContext.value) return enrichedPackageRows.value.filter(row => row.binaries && row.binaries.length > 0)
  ```

- [ ] **Step 2: Update disabled logic in `PackagesSubTab.vue`**

  In the `<template>`, find the pkg-row `<button>` element and replace the three attributes:

  Old:
  ```html
  @click="row.binariesAvailable && row.state === 'succeeded' ? toggleRow(row) : undefined"
  :disabled="!row.binariesAvailable || row.state !== 'succeeded'"
  :title="!row.binariesAvailable ? 'No target binaries available' : (row.state !== 'succeeded' ? 'Not built' : 'Click to show binaries')"
  ```

  New:
  ```html
  @click="row.state === 'succeeded' ? toggleRow(row) : undefined"
  :disabled="row.state !== 'succeeded'"
  :title="row.state !== 'succeeded' ? 'Package is not in succeeded state' : 'Click to show binaries'"
  ```

- [ ] **Step 3: Update expand glyph**

  Old:
  ```html
  <span class="expand-glyph">{{ row.binariesAvailable ? (expanded[rowKey(row)] ? '▼' : '▶') : '' }}</span>
  ```

  New:
  ```html
  <span class="expand-glyph">{{ row.state === 'succeeded' ? (expanded[rowKey(row)] ? '▼' : '▶') : '' }}</span>
  ```

- [ ] **Step 4: Build and commit**

  ```bash
  cd frontend && npm run build
  ```
  Expected: exits 0 with no TypeScript or build errors.

  ```bash
  git add frontend/src/components/ArtifactsPanel.vue frontend/src/components/PackagesSubTab.vue
  git commit -s -m "feat(frontend): hide binary-less rows; grey out non-succeeded package rows"
  ```
