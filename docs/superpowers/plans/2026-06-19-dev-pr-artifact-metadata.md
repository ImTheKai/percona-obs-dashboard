# DEV/PR Artifact Metadata and Rebuild Status — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add backend-cached artifact metadata for DEV/PR projects so the Artifacts tab shows `built_at` timestamps and a "Rebuilding" pill when OBS is actively producing a replacement for a known artifact.

**Architecture:** New `POST /api/artifacts/metadata` endpoint groups request items by OBS project, fetches `ProjectBinaryList` once per project with a 5-minute in-process cache, and returns per-item metadata. The frontend composes a new `useArtifactMetadata` composable that eagerly batch-fetches metadata when visible rows change and merges it into `PackageRow`/`ContainerImage`. `isRebuilding` is frontend-derived: prior `builtAt` exists **and** current target/rollup state is `building`, `scheduled`, or `finished`. Release context skips the metadata fetch entirely.

**Tech stack:** Go (chi, standard library), Vue 3 + TypeScript, no SQLite changes.

**User decisions (already made):**
- Metadata fetch is eager (fires when visible rows are computed, not on row expand).
- New `useArtifactMetadata` composable (Option A) — not inlined in `ArtifactsPanel` or extended inside `useArtifacts`.
- `isRebuilding` is frontend-derived — no new persisted backend state.

---

## File Map

| File | Change |
|---|---|
| `backend/internal/api/artifact_metadata.go` | **Create** — cache type, request/response types, handler, `resolveMetadataItem` |
| `backend/internal/api/artifact_metadata_test.go` | **Create** — unit tests |
| `backend/internal/api/server.go` | **Modify** — register `POST /api/artifacts/metadata` |
| `frontend/src/composables/useArtifacts.ts` | **Modify** — add `mtime?`, `isRebuilding?` to both interfaces; add `project` to `ContainerImage` |
| `frontend/src/composables/useArtifactMetadata.ts` | **Create** — batch fetch + enrichment composable |
| `frontend/src/components/ArtifactsPanel.vue` | **Modify** — compose `useArtifactMetadata`, pass enriched rows |
| `frontend/src/components/PackagesSubTab.vue` | **Modify** — "Rebuilding" pill + CSS |
| `frontend/src/components/ContainersSubTab.vue` | **Modify** — "Rebuilding" pill + CSS |

---

## Task 1: Backend `POST /api/artifacts/metadata` endpoint

**Goal:** Implement the metadata endpoint with a per-project binary-list cache and register it in the router.

**Files:**
- Create: `backend/internal/api/artifact_metadata.go`
- Create: `backend/internal/api/artifact_metadata_test.go`
- Modify: `backend/internal/api/server.go`

**Acceptance Criteria:**
- [ ] `POST /api/artifacts/metadata` returns 400 for an empty `items` array.
- [ ] Package items return only distributable binaries; `built_at` reflects the max mtime of those binaries.
- [ ] Container items return `built_at` from the `.containerinfo` binary with the highest mtime; no `binaries` field.
- [ ] Two items from the same project result in a single `ProjectBinaryList` call (cache hit on second call).
- [ ] `go test ./backend/internal/api/... -run TestArtifactMetadata` passes.

**Verify:** `cd backend && go test ./internal/api/... -run TestArtifactMetadata -v` → all tests PASS

**Steps:**

- [ ] **Step 1: Write failing tests in `artifact_metadata_test.go`**

```go
package api

import (
	"context"
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/obs"
)

func TestArtifactMetadataPackageFiltersDistributableAndPicksMaxMtime(t *testing.T) {
	item := ArtifactMetadataItem{
		Project: "isv:percona:ppg:17",
		Name:    "etcd",
		Repo:    "openSUSE_Leap_15.6",
		Arch:    "x86_64",
		Kind:    "package",
	}
	binaries := []obs.BinaryArtifact{
		{
			Project: "isv:percona:ppg:17", Repo: "openSUSE_Leap_15.6", Arch: "x86_64",
			Package: "etcd", Filename: "etcd-3.5.30-1.x86_64.rpm",
			MTime: 1000, BuiltAt: time.Unix(1000, 0).UTC(),
		},
		{
			Project: "isv:percona:ppg:17", Repo: "openSUSE_Leap_15.6", Arch: "x86_64",
			Package: "etcd", Filename: "etcd-debugsource-3.5.30-1.x86_64.rpm",
			MTime: 2000, BuiltAt: time.Unix(2000, 0).UTC(),
		},
	}
	result := resolveMetadataItem(item, binaries)
	if len(result.Binaries) != 1 {
		t.Fatalf("expected 1 distributable binary, got %d", len(result.Binaries))
	}
	if result.Binaries[0].Filename != "etcd-3.5.30-1.x86_64.rpm" {
		t.Errorf("unexpected filename: %s", result.Binaries[0].Filename)
	}
	want := time.Unix(1000, 0).UTC().Format(time.RFC3339)
	if result.BuiltAt != want {
		t.Errorf("BuiltAt = %q, want %q", result.BuiltAt, want)
	}
}

func TestArtifactMetadataContainerUsesContainerinfoMaxMtime(t *testing.T) {
	item := ArtifactMetadataItem{
		Project: "isv:percona:ppg:17:containers:ubi9",
		Name:    "postgresql-17",
		Kind:    "container",
	}
	binaries := []obs.BinaryArtifact{
		{
			Project: "isv:percona:ppg:17:containers:ubi9", Repo: "standard", Arch: "x86_64",
			Package: "postgresql-17", Filename: "postgresql-17.containerinfo",
			MTime: 5000, BuiltAt: time.Unix(5000, 0).UTC(),
		},
		{
			Project: "isv:percona:ppg:17:containers:ubi9", Repo: "standard", Arch: "aarch64",
			Package: "postgresql-17", Filename: "postgresql-17.containerinfo",
			MTime: 6000, BuiltAt: time.Unix(6000, 0).UTC(),
		},
	}
	result := resolveMetadataItem(item, binaries)
	if result.MTime != 6000 {
		t.Errorf("MTime = %d, want 6000 (highest mtime)", result.MTime)
	}
	want := time.Unix(6000, 0).UTC().Format(time.RFC3339)
	if result.BuiltAt != want {
		t.Errorf("BuiltAt = %q, want %q", result.BuiltAt, want)
	}
	if result.Binaries != nil {
		t.Errorf("container result must not include Binaries")
	}
}

func TestArtifactMetadataContainerNoMatchReturnsEmptyBuiltAt(t *testing.T) {
	item := ArtifactMetadataItem{
		Project: "isv:percona:ppg:17:containers:ubi9",
		Name:    "postgresql-17",
		Kind:    "container",
	}
	result := resolveMetadataItem(item, nil)
	if result.BuiltAt != "" {
		t.Errorf("expected empty BuiltAt when no binaries, got %q", result.BuiltAt)
	}
}

func TestBinaryListCacheReturnsCachedResult(t *testing.T) {
	calls := 0
	cache := newBinaryListCache(5 * time.Minute)
	fetch := func(ctx context.Context) ([]obs.BinaryArtifact, error) {
		calls++
		return []obs.BinaryArtifact{}, nil
	}
	if _, err := cache.Get(context.Background(), "proj", fetch); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(context.Background(), "proj", fetch); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected 1 fetch call, got %d (cache miss on second call)", calls)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL (functions not yet defined)**

```bash
cd backend && go test ./internal/api/... -run TestArtifactMetadata -v
```

Expected output contains: `undefined: resolveMetadataItem` or `undefined: newBinaryListCache`

- [ ] **Step 3: Create `backend/internal/api/artifact_metadata.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/percona/obs-dashboard/internal/obs"
)

// ArtifactMetadataItem is a single item in a POST /api/artifacts/metadata request.
type ArtifactMetadataItem struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Repo    string `json:"repo"` // empty = any repo (used for containers)
	Arch    string `json:"arch"` // empty = any arch (used for containers)
	Kind    string `json:"kind"` // "package" | "container"
}

// ArtifactMetadataResult is the per-item metadata response.
type ArtifactMetadataResult struct {
	Project  string           `json:"project"`
	Name     string           `json:"name"`
	Repo     string           `json:"repo"`
	Arch     string           `json:"arch"`
	Kind     string           `json:"kind"`
	BuiltAt  string           `json:"built_at,omitempty"`
	MTime    int64            `json:"mtime,omitempty"`
	Binaries []ArtifactBinary `json:"binaries,omitempty"`
}

type artifactMetadataResponse struct {
	Items []ArtifactMetadataResult `json:"items"`
}

// binaryListCache caches ProjectBinaryList results per project for a configurable TTL.
// It deduplicates concurrent requests for the same project using an inflight map,
// matching the pattern used by releaseArtifactsCache.
type binaryListCache struct {
	mu       sync.Mutex
	ttl      time.Duration
	entries  map[string]binaryListCacheEntry
	inflight map[string]chan struct{}
}

type binaryListCacheEntry struct {
	binaries []obs.BinaryArtifact
	expires  time.Time
	err      error
}

func newBinaryListCache(ttl time.Duration) *binaryListCache {
	return &binaryListCache{
		ttl:      ttl,
		entries:  map[string]binaryListCacheEntry{},
		inflight: map[string]chan struct{}{},
	}
}

func (c *binaryListCache) Get(ctx context.Context, key string, fetch func(context.Context) ([]obs.BinaryArtifact, error)) ([]obs.BinaryArtifact, error) {
	now := time.Now()
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && now.Before(entry.expires) {
		c.mu.Unlock()
		return entry.binaries, entry.err
	}
	if wait, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.mu.Lock()
		entry := c.entries[key]
		c.mu.Unlock()
		return entry.binaries, entry.err
	}
	wait := make(chan struct{})
	c.inflight[key] = wait
	c.mu.Unlock()

	binaries, err := fetch(ctx)
	c.mu.Lock()
	expires := time.Now()
	if err == nil {
		expires = expires.Add(c.ttl)
	}
	c.entries[key] = binaryListCacheEntry{binaries: binaries, expires: expires, err: err}
	delete(c.inflight, key)
	close(wait)
	c.mu.Unlock()
	return binaries, err
}

func artifactMetadataHandler(obsClient *obs.Client, cache *binaryListCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if obsClient == nil {
			http.Error(w, "OBS client not configured", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			Items []ArtifactMetadataItem `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Items) == 0 {
			http.Error(w, "items must be a non-empty array", http.StatusBadRequest)
			return
		}

		// Deduplicate projects so we call ProjectBinaryList at most once per project.
		projects := map[string]struct{}{}
		for _, item := range req.Items {
			projects[item.Project] = struct{}{}
		}
		projectBinaries := make(map[string][]obs.BinaryArtifact, len(projects))
		for project := range projects {
			bins, err := cache.Get(r.Context(), project, func(ctx context.Context) ([]obs.BinaryArtifact, error) {
				return obsClient.ProjectBinaryList(ctx, project)
			})
			if err != nil {
				slog.Warn("artifact_metadata: binarylist fetch failed", "project", project, "err", err)
			}
			projectBinaries[project] = bins
		}

		results := make([]ArtifactMetadataResult, len(req.Items))
		for i, item := range req.Items {
			results[i] = resolveMetadataItem(item, projectBinaries[item.Project])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(artifactMetadataResponse{Items: results})
	}
}

// resolveMetadataItem matches a single request item against a project's binary list.
// For packages: collects distributable binaries matching name/repo/arch; built_at is
// max mtime among those. For containers: finds .containerinfo with highest mtime.
func resolveMetadataItem(item ArtifactMetadataItem, binaries []obs.BinaryArtifact) ArtifactMetadataResult {
	result := ArtifactMetadataResult{
		Project: item.Project,
		Name:    item.Name,
		Repo:    item.Repo,
		Arch:    item.Arch,
		Kind:    item.Kind,
	}

	if item.Kind == "container" {
		for _, b := range binaries {
			if b.Package != item.Name || !strings.HasSuffix(b.Filename, ".containerinfo") {
				continue
			}
			if b.MTime > result.MTime {
				result.MTime = b.MTime
				result.BuiltAt = b.BuiltAt.Format(time.RFC3339)
			}
		}
		return result
	}

	// kind == "package"
	for _, b := range binaries {
		if b.Package != item.Name {
			continue
		}
		if item.Repo != "" && b.Repo != item.Repo {
			continue
		}
		if item.Arch != "" && b.Arch != item.Arch {
			continue
		}
		if !obs.IsDistributableBinary(b.Filename) {
			continue
		}
		result.Binaries = append(result.Binaries, releaseBinary(b))
		if b.MTime > result.MTime {
			result.MTime = b.MTime
			result.BuiltAt = b.BuiltAt.Format(time.RFC3339)
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
cd backend && go test ./internal/api/... -run TestArtifactMetadata -v
```

Expected: `PASS` for all 4 tests.

- [ ] **Step 5: Register the route in `backend/internal/api/server.go`**

Find the line `releaseArtifacts := newReleaseArtifactsCache(10 * time.Minute)` and add the metadata cache below it:

```go
releaseArtifacts := newReleaseArtifactsCache(10 * time.Minute)
metadataCache := newBinaryListCache(5 * time.Minute)
```

Add the route after the existing `r.Get("/api/binaries", ...)` line:

```go
r.Get("/api/binaries", binariesHandler(obsClient))
r.Post("/api/artifacts/metadata", artifactMetadataHandler(obsClient, metadataCache))
```

- [ ] **Step 6: Build check**

```bash
cd backend && go build ./...
```

Expected: exits 0, no output.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/api/artifact_metadata.go \
        backend/internal/api/artifact_metadata_test.go \
        backend/internal/api/server.go
git commit -s -m "feat(api): add POST /api/artifacts/metadata endpoint with per-project cache"
```

---

## Task 2: Frontend types and `useArtifactMetadata` composable

**Goal:** Extend `PackageRow` and `ContainerImage` with metadata fields and create the composable that batch-fetches and merges metadata.

**Files:**
- Modify: `frontend/src/composables/useArtifacts.ts`
- Create: `frontend/src/composables/useArtifactMetadata.ts`

**Acceptance Criteria:**
- [ ] `PackageRow` has optional fields `mtime?: number` and `isRebuilding?: boolean`.
- [ ] `ContainerImage` has new fields `project: string`, `mtime?: number` (already existed — verify), and `isRebuilding?: boolean`.
- [ ] `useArtifacts` populates `ContainerImage.project` from `pkg.project`.
- [ ] `useArtifactMetadata` skips the fetch when `isLiveContext.value` is false.
- [ ] `enrichedPackageRows` returns rows with `builtAt`, `mtime`, `binaries`, and `isRebuilding` merged from metadata when available.
- [ ] `npm run build` exits 0.

**Verify:** `cd frontend && npm run build` → exits 0 with no TypeScript errors

**Steps:**

- [ ] **Step 1: Update types in `frontend/src/composables/useArtifacts.ts`**

Add `mtime?` and `isRebuilding?` to `PackageRow` (after the existing `builtAt?` field):

```typescript
export interface PackageRow {
  project: string
  name: string
  version: string
  tags: string[]
  state: string
  published: boolean
  repo: RepoInfo
  arch: string
  binariesAvailable: boolean
  binaries?: ArtifactBinary[]
  builtAt?: string
  mtime?: number
  isRebuilding?: boolean
}
```

Add `project`, `isRebuilding?` to `ContainerImage` (note: `mtime?` already exists):

```typescript
export interface ContainerImage {
  id: string
  project: string        // ← new: OBS project path
  imageName: string
  baseOs: string
  registry: string
  tags: string[]
  pullCmd: string
  rollupState: string
  published: boolean
  mtime?: number
  builtAt?: string
  isRebuilding?: boolean // ← new
}
```

In the `containerImages` computed inside `useArtifacts`, add `project: pkg.project` to the returned object:

```typescript
return {
  id: pkg.project + '/' + pkg.name,
  project: pkg.project,           // ← add this line
  imageName: pkg.name,
  baseOs,
  registry,
  tags,
  pullCmd,
  rollupState: pkg.rollup_state ?? '',
  published,
}
```

- [ ] **Step 2: Create `frontend/src/composables/useArtifactMetadata.ts`**

```typescript
import { ref, watch, computed } from 'vue'
import type { Ref, ComputedRef } from 'vue'
import type { PackageRow, ContainerImage, ArtifactBinary } from './useArtifacts'

interface ArtifactMetadataItem {
  project: string
  name: string
  repo: string
  arch: string
  kind: 'package' | 'container'
}

interface ArtifactMetadataResult {
  project: string
  name: string
  repo: string
  arch: string
  kind: string
  built_at?: string
  mtime?: number
  binaries?: ArtifactBinary[]
}

const REBUILDING_STATES = new Set(['building', 'scheduled', 'finished'])

function metaKey(project: string, name: string, repo: string, arch: string, kind: string): string {
  return `${project}/${name}/${repo}/${arch}/${kind}`
}

export function useArtifactMetadata(
  packageRows: Ref<PackageRow[]>,
  containerImages: Ref<ContainerImage[]>,
  isLiveContext: Ref<boolean>,
): {
  enrichedPackageRows: ComputedRef<PackageRow[]>
  enrichedContainerImages: ComputedRef<ContainerImage[]>
} {
  const metadataMap = ref(new Map<string, ArtifactMetadataResult>())

  async function fetchMetadata() {
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

    try {
      const res = await fetch('/api/artifacts/metadata', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ items }),
      })
      if (!res.ok) return
      const data = await res.json() as { items: ArtifactMetadataResult[] }
      const newMap = new Map<string, ArtifactMetadataResult>()
      for (const result of data.items) {
        newMap.set(metaKey(result.project, result.name, result.repo, result.arch, result.kind), result)
      }
      metadataMap.value = newMap
    } catch {
      // metadata is best-effort; silently ignore network/parse errors
    }
  }

  watch([packageRows, containerImages, isLiveContext], fetchMetadata, { immediate: true })

  const enrichedPackageRows = computed<PackageRow[]>(() =>
    packageRows.value.map(row => {
      const meta = metadataMap.value.get(
        metaKey(row.project, row.name, row.repo.obs, row.arch, 'package'),
      )
      if (!meta?.built_at) return row
      return {
        ...row,
        builtAt: meta.built_at,
        mtime: meta.mtime,
        binaries: meta.binaries ?? row.binaries,
        isRebuilding: REBUILDING_STATES.has(row.state),
      }
    })
  )

  const enrichedContainerImages = computed<ContainerImage[]>(() =>
    containerImages.value.map(img => {
      const meta = metadataMap.value.get(
        metaKey(img.project, img.imageName, '', '', 'container'),
      )
      if (!meta?.built_at) return img
      return {
        ...img,
        builtAt: meta.built_at,
        mtime: meta.mtime,
        isRebuilding: REBUILDING_STATES.has(img.rollupState),
      }
    })
  )

  return { enrichedPackageRows, enrichedContainerImages }
}
```

- [ ] **Step 3: Build check**

```bash
cd frontend && npm run build
```

Expected: exits 0. If TypeScript errors appear, they will be about the new `project` field missing on `ContainerImage` object literals in `ArtifactsPanel.vue` — those are fixed in Task 3.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/composables/useArtifacts.ts \
        frontend/src/composables/useArtifactMetadata.ts
git commit -s -m "feat(frontend): add artifact metadata types and useArtifactMetadata composable"
```

---

## Task 3: Wire `ArtifactsPanel` and add "Rebuilding" pills to sub-tabs

**Goal:** Connect `useArtifactMetadata` to `ArtifactsPanel` and render the "Rebuilding" pill in both sub-tabs.

**Files:**
- Modify: `frontend/src/components/ArtifactsPanel.vue`
- Modify: `frontend/src/components/PackagesSubTab.vue`
- Modify: `frontend/src/components/ContainersSubTab.vue`

**Acceptance Criteria:**
- [ ] Release context rows pass through unchanged (no metadata fetch, no Rebuilding pill).
- [ ] DEV/PR package rows show `builtAt` timestamp and "Rebuilding" pill when `isRebuilding` is true.
- [ ] DEV/PR container images show BUILT timestamp and "Rebuilding" pill when `isRebuilding` is true.
- [ ] Expanding a DEV/PR package row with metadata binaries does not trigger `GET /api/binaries`.
- [ ] `npm run build` exits 0.

**Verify:** `cd frontend && npm run build` → exits 0 with no TypeScript errors

**Steps:**

- [ ] **Step 1: Update `frontend/src/components/ArtifactsPanel.vue`**

Add the import for `useArtifactMetadata` near the other composable imports:

```typescript
import { useArtifactMetadata } from '../composables/useArtifactMetadata'
```

After the existing `useArtifacts` call (currently near line 231):

```typescript
const { packageRows: livePackageRows, containerImages: liveContainerImages } = useArtifacts(
  artifactsPackages,
  localVersion,
  selectedRepo,
  artArch,
  contextPrefix,
)
```

Add immediately after it:

```typescript
const { enrichedPackageRows, enrichedContainerImages } = useArtifactMetadata(
  livePackageRows,
  liveContainerImages,
  computed(() => !isReleaseContext.value),
)
```

In the `packageRows` computed (currently returns `livePackageRows.value` for non-release), replace:

```typescript
const packageRows = computed<PackageRow[]>(() => {
  if (!isReleaseContext.value) return livePackageRows.value
```

with:

```typescript
const packageRows = computed<PackageRow[]>(() => {
  if (!isReleaseContext.value) return enrichedPackageRows.value
```

In the `containerImages` computed (currently returns `liveContainerImages.value` for non-release), replace:

```typescript
const containerImages = computed<ContainerImage[]>(() => {
  if (!isReleaseContext.value) return liveContainerImages.value
```

with:

```typescript
const containerImages = computed<ContainerImage[]>(() => {
  if (!isReleaseContext.value) return enrichedContainerImages.value
```

Also fix the release `ContainerImage` literal to include `project` (required by the updated interface). Find the `containerImages` computed release branch:

```typescript
return releaseArtifacts.value.container_images.map(img => ({
  id: `${img.project}/${img.image_name}`,
  imageName: img.image_name,
```

and add `project`:

```typescript
return releaseArtifacts.value.container_images.map(img => ({
  id: `${img.project}/${img.image_name}`,
  project: img.project,
  imageName: img.image_name,
```

- [ ] **Step 2: Add "Rebuilding" pill to `frontend/src/components/PackagesSubTab.vue`**

Find the package row template (around line 233) — it currently reads:

```html
<span v-if="row.builtAt" class="pkg-built-at">{{ formatArtifactTime(row.builtAt) }}</span>
<span class="status-badge" :class="row.published ? 'status-published' : stateClass(row.state)">
  {{ row.published ? 'Published' : stateLabel(row.state) }}
</span>
```

Replace with:

```html
<span v-if="row.builtAt" class="pkg-built-at">{{ formatArtifactTime(row.builtAt) }}</span>
<span v-if="row.isRebuilding" class="status-badge status-rebuilding">Rebuilding</span>
<span class="status-badge" :class="row.published ? 'status-published' : stateClass(row.state)">
  {{ row.published ? 'Published' : stateLabel(row.state) }}
</span>
```

Add the CSS class after the existing `.status-other` rule (around line 599):

```css
.status-rebuilding {
  background: #fef9c3;
  color: #a16207;
}
```

- [ ] **Step 3: Add "Rebuilding" pill to `frontend/src/components/ContainersSubTab.vue`**

Find the BUILT section (around line 97) — it currently reads:

```html
<div v-if="image.builtAt" class="built-section">
  <div class="section-label">BUILT</div>
  <span class="built-time">{{ formatArtifactTime(image.builtAt) }}</span>
</div>
```

Replace with:

```html
<div v-if="image.builtAt" class="built-section">
  <div class="section-label">BUILT</div>
  <span class="built-time">{{ formatArtifactTime(image.builtAt) }}</span>
  <span v-if="image.isRebuilding" class="status-badge building">Rebuilding</span>
</div>
```

(Uses the existing `.status-badge.building` CSS class — yellow badge — already defined in ContainersSubTab.)

- [ ] **Step 4: Build check**

```bash
cd frontend && npm run build
```

Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/ArtifactsPanel.vue \
        frontend/src/components/PackagesSubTab.vue \
        frontend/src/components/ContainersSubTab.vue
git commit -s -m "feat(frontend): wire artifact metadata enrichment and show Rebuilding pill"
```
