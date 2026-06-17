# Artifacts Tab Design

## Goal

Add a top-level "Artifacts" panel to the PPG Build Board that lets users browse every RPM/DEB package and Docker container image published to OBS for a given PPG major version, copy repository setup instructions, and get `docker pull` commands — all sourced from data the backend already collects.

## Architecture

A segmented pill switcher in the header toggles between the existing Build Board panel and the new Artifacts panel. The Artifacts panel is a pure read-only frontend view over the existing `/packages` API response — no new backend endpoints needed. The only backend change is extending `Package` to store the full container tag list (currently only `tags[0]` is kept).

## Tech Stack

Go backend (model, store, worker task), Vue 3 Composition API + TypeScript frontend, existing SQLite store with idempotent `ALTER TABLE` migrations.

## User decisions (already made)

- Reuse existing backend package data (Option A); add backend tasks only for missing data.
- Container image variants are separate OBS projects (e.g. `isv:percona:ppg:17:containers:ubi8`), so they are already separate `Package` rows.
- All container tags must be stored (not just `tags[0]`); `Version` keeps `tags[0]` for the existing version badge.
- Component structure: shallow tree + one composable (Approach B).

---

## Backend changes

### 1. Model (`backend/internal/model/types.go`)

Add one field to `Package`:

```go
ContainerTags []string `json:"container_tags,omitempty"`
```

`Version` is unchanged — it keeps `tags[0]` for the existing version badge. `ContainerTags` holds the full ordered list for the Artifacts cards.

### 2. Database (`backend/internal/store/db.go`)

Add column to the `packages` CREATE TABLE:

```sql
container_tags TEXT NOT NULL DEFAULT '[]',
```

Idempotent additive migration in `Open()`:

```go
db.Exec(`ALTER TABLE packages ADD COLUMN container_tags TEXT NOT NULL DEFAULT '[]'`)
```

### 3. Store (`backend/internal/store/packages.go`)

- `UpsertPackageState`: marshal `p.ContainerTags` to JSON and include in INSERT/UPDATE.
- `scanPackages`: scan the `container_tags` column, unmarshal JSON into `p.ContainerTags`.
- Add `container_tags` to `packageSelectCols`.

### 4. Worker task (`backend/internal/obs/tasks.go`)

Update `ContainerTagsTask.Run` to store all tags instead of only the first:

```go
// Before (stores only first tag as Version):
pkg.Version = tags[0]

// After (stores all tags; Version keeps first for badge):
pkg.Version = tags[0]
pkg.ContainerTags = tags
```

The OBS containerinfo JSON already returns the full list — the previous implementation discarded everything after index 0.

### 5. Frontend types (`frontend/src/types/api.ts`)

```ts
interface Package {
  // existing fields …
  container_tags?: string[]
}
```

---

## Frontend

### State additions (`frontend/src/App.vue`)

```ts
const mainTab = ref<'board' | 'artifacts'>('board')
```

No persistence — resets to `'board'` on reload. All other state (`version`, `theme`, `activeScopes`, etc.) is unaffected when switching tabs.

### Tab switcher (`frontend/src/components/AppHeader.vue`)

New props and emit:

```ts
defineProps<{ theme: 'light' | 'dark', mainTab: 'board' | 'artifacts' }>()
const emit = defineEmits<{ 'toggle-theme': [], 'update:main-tab': ['board' | 'artifacts'] }>()
```

A pill-group control rendered in the header's right-side flex group **before** the theme toggle:

```
[ Build Board ][ Artifacts ]    [ ● Light mode ]
```

Outer wrapper: `display:flex; gap:3px; background:var(--bg-muted); padding:3px; border-radius:11px`.

Active button: `background:var(--bg-card); color:var(--brand-purple); box-shadow:0 1px 2px rgba(0,0,0,0.12)`.

Inactive button: `background:transparent; color:var(--text-muted)`.

Both: `padding:6px 14px; border-radius:8px; font-size:13px; font-weight:700; border:none; cursor:pointer`.

### App.vue template

```html
<AppHeader :theme :main-tab="mainTab"
  @toggle-theme="toggleTheme"
  @update:main-tab="mainTab = $event" />

<template v-if="mainTab === 'board'">
  <ContextBar … />
  <HealthHeader … />
  <MainGrid … />
</template>

<ArtifactsPanel
  v-else
  :packages="allPackages"
  :version="version"
  :available-versions="availableVersions"
  @update:version="version = $event"
/>
```

### Component tree

```
ArtifactsPanel.vue          local state: artifactsTab, artRepo, artArch, copiedKey
├── ArtifactsVersionBar.vue  version pills + OBS root chip
├── sub-tab switcher pills   (inline in ArtifactsPanel)
├── PackagesSubTab.vue       [v-if artifactsTab==='packages']
└── ContainersSubTab.vue     [v-else]
```

### `frontend/src/composables/useArtifacts.ts`

Single composable that derives Artifacts-specific data structures from the raw `packages` array. Accepts reactive refs for `packages`, `version`, `artRepo`, `artArch`.

**Repo config (static, defined in composable):**

```ts
interface Repo {
  short: string   // UI key, e.g. 'el9'
  name:  string   // display, e.g. 'RHEL 9'
  obs:   string   // OBS repo name, e.g. 'RHEL_9'
  type:  'rpm' | 'deb'
}

const REPOS: Repo[] = [
  { short: 'el9',    name: 'RHEL 9',       obs: 'RHEL_9',        type: 'rpm' },
  { short: 'el8',    name: 'RHEL 8',       obs: 'RHEL_8',        type: 'rpm' },
  { short: 'deb12',  name: 'Debian 12',    obs: 'Debian_12',     type: 'deb' },
  { short: 'deb11',  name: 'Debian 11',    obs: 'Debian_11',     type: 'deb' },
  { short: 'ub2404', name: 'Ubuntu 24.04', obs: 'xUbuntu_24.04', type: 'deb' },
  { short: 'ub2204', name: 'Ubuntu 22.04', obs: 'xUbuntu_22.04', type: 'deb' },
]
```

**`packageRows` computed** — filters packages to those with a target for the selected repo × arch:

```ts
const packageRows = computed(() => {
  const repo = REPOS.find(r => r.short === artRepo.value)
  if (!repo) return []
  return packages.value
    .filter(pkg => ['common', 'ppgcommon', 'version'].includes(pkg.scope))
    .filter(pkg =>
      pkg.scope !== 'version' ||
      !version.value ||
      pkg.project.includes(`:ppg:${version.value}`)
    )
    .flatMap(pkg => {
      const target = pkg.targets.find(
        t => t.repo === repo.obs && t.arch === artArch.value
      )
      if (!target) return []
      return [{ pkg, target, repoType: repo.type }]
    })
})
```

**`containerImages` computed** — one entry per container-scoped package:

```ts
const containerImages = computed(() =>
  packages.value
    .filter(pkg => pkg.scope === 'container')
    .map(pkg => ({
      id:        `${pkg.project}/${pkg.name}`,
      name:      pkg.name,
      project:   pkg.project,
      baseOs:    deriveBaseOs(pkg.project),
      tags:      pkg.container_tags ?? [],
      pullCmd:   `docker pull percona/${pkg.name}:${(pkg.container_tags ?? [])[0] ?? ''}`,
      published: pkg.targets.some(t => t.published),
    }))
)
```

**`deriveBaseOs`** — maps OBS project suffix to a human-readable OS label:

```ts
const BASE_OS_LABELS: Record<string, string> = {
  ubi8: 'Red Hat UBI 8',
  ubi9: 'Red Hat UBI 9',
}

function deriveBaseOs(project: string): string {
  const suffix = project.split(':containers:')[1] ?? ''
  return BASE_OS_LABELS[suffix] ?? suffix
}
```

New OS variants are added to `BASE_OS_LABELS` without touching any other code.

Returns: `{ packageRows, containerImages, REPOS }`.

---

### `ArtifactsPanel.vue`

Props: `packages: Package[]`, `version: string`, `availableVersions: string[]`.
Emits: `update:version`.

Local state: `artifactsTab`, `artRepo` (default `'el9'`), `artArch` (default `'x86_64'`), `copiedKey`.

`copy(key, text)` helper — writes to clipboard, sets `copiedKey = key`, reverts after 2 s:

```ts
function copy(key: string, text: string) {
  navigator.clipboard.writeText(text)
  copiedKey.value = key
  setTimeout(() => { if (copiedKey.value === key) copiedKey.value = null }, 2000)
}
```

The `obsRoot` computed is `` `isv:percona:ppg:${version}` `` (shown in `ArtifactsVersionBar`).

Template layout: vertical stack inside the standard `max-width: 1360px` container. `ArtifactsVersionBar` at top, sub-tab pills below it, then `PackagesSubTab` or `ContainersSubTab` based on `artifactsTab`.

Sub-tab switcher pills use the same active/inactive style as the main tab switcher.

---

### `ArtifactsVersionBar.vue`

Props: `version: string`, `availableVersions: string[]`, `obsRoot: string`.
Emits: `update:version`.

A single horizontal card (`var(--bg-card)`, `border-radius: 12px`, `padding: 14px 18px`), flex row left-to-right:

1. **Tech badge** — `<span class="tech-badge tech-badge--postgresql">` using existing component class.
2. **"VERSION" label** — `font-size: 11px; font-weight: 700; color: var(--text-muted); letter-spacing: 0.06em`.
3. **Version pills** — one `<button>` per entry in `availableVersions`. Active (matches `version`): `background: var(--brand-purple); color: #fff; border: 1px solid var(--brand-purple)`. Inactive: `background: var(--bg-card); color: var(--text-secondary); border: 1px solid var(--border)`. Both: `padding: 4px 12px; border-radius: 6px; font-size: 13px; font-weight: 600; cursor: pointer`.
4. **OBS root chip** — `<code>` element: `font-size: 12px; background: var(--bg-muted); color: var(--text-muted); padding: 3px 8px; border-radius: 5px`.

---

### `PackagesSubTab.vue`

Props: `packageRows`, `repos: Repo[]`, `artRepo: string`, `artArch: string`, `version: string`, `copiedKey: string | null`.
Emits: `update:art-repo`, `update:art-arch`, `copy`.

Two-column flex layout. Left: 220 px sticky sidebar. Right: main content area.

**Distro sidebar** — a card with two labelled groups ("RPM", "DEB"). Each repo is a full-width button. Active: `background: var(--tint-purple); color: var(--brand-purple); font-weight: 700`. Inactive: `background: transparent; color: var(--text-secondary); font-weight: 500`. Clicking emits `update:art-repo`.

**Right column** stacks two cards:

**1. Repo + setup card** — a single card (`border-radius: 14px`) containing the repo header and the setup snippet together.

The **card header row** (flex, space-between): left side has the distro name (17 px, bold) + OBS repo name (monospace, muted); right side has the **arch selector**.

**Arch selector** — a borderless pill-group wrapper (`background: var(--bg-muted); padding: 3px; border-radius: 9px`) with two buttons: `x86_64` and `aarch64` (display text matches the arch ID exactly). This uses the **same borderless style as the main tab switcher**, NOT the bordered version-pill style. Active button: `background: var(--bg-card); color: var(--brand-purple); box-shadow: 0 1px 2px rgba(0,0,0,0.10); border: none; border-radius: 6px`. Inactive button: `background: transparent; color: var(--text-muted); border: none`. Clicking emits `update:art-arch`.

Below the header row: a "REPOSITORY SETUP" section label (uppercase, 10 px, 700 weight, `color: var(--text-muted)`), then a `<pre>` code block with `background: var(--bg-card-2)` and a copy button in the top-right corner. Snippet text is derived from `version`, current repo, and repo type:

RPM template (repos `el9`, `el8`):
```
[percona-ppg{version}]
name=Percona PPG {version} — {repoName}
baseurl=https://download.opensuse.org/repositories/isv:percona:ppg:{version}/{obsRepo}/
enabled=1
gpgcheck=0

# Save to /etc/yum.repos.d/percona-ppg{version}.repo, then:
dnf makecache
dnf install percona-postgresql{version}-server
```

DEB template (repos `deb12`, `deb11`, `ub2404`, `ub2204`):
```
# 1. Add repository
echo "deb https://download.opensuse.org/repositories/isv:percona:ppg:{version}/{obsRepo}/ ./" \
  | sudo tee /etc/apt/sources.list.d/percona-ppg{version}.list

# 2. Import GPG key
wget -qO- https://download.opensuse.org/repositories/isv:percona:ppg:{version}/{obsRepo}/Release.key \
  | sudo apt-key add -

# 3. Update and install
sudo apt-get update
sudo apt-get install percona-postgresql-{version}
```

Copy button: `copiedKey === 'repo-config'` → shows "✓ Copied" (green). Clicking emits `copy('repo-config', snippetText)`.

**2. Package list card** — header "Packages" + subtitle `{count} available · {repoName} / {arch}`. Each row:

| Element | Details |
|---|---|
| Package name | Monospace 13.5 px bold |
| Scope badge | Pill. `common` → "Common" (grey); `ppgcommon` → "PPG·Common" (grey); `version` → `PPG {version}` (purple tint) |
| Install command | Monospace 11 px muted. `dnf install {name}` for RPM; `apt-get install {name}` for DEB |
| Build status | Right-aligned. `succeeded` → "Built" green badge; others → raw state string in matching colour |
| Download button | Links to `https://build.opensuse.org/package/binaries/{project}/{name}/{obsRepo}?arch={arch}`, opens new tab. Disabled (`opacity: 0.4; pointer-events: none`) when state ≠ `succeeded` |

---

### `ContainersSubTab.vue`

Props: `images` (array from `useArtifacts`), `copiedKey: string | null`.
Emits: `copy`.

Responsive grid: `display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 16px`.

Each card is a `var(--bg-card)` rounded card (`border-radius: 12px; border: 1px solid var(--border)`) with four sections separated by `1px solid var(--border)` dividers:

**Section 1 — Header** (`padding: 16px`): The icon is a specific box/shelf SVG in a `var(--info-tint)` rounded square:
```html
<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor"
     stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
  <rect x="2" y="7" width="20" height="14" rx="3"/>
  <path d="M7 7V5a2 2 0 012-2h6a2 2 0 012 2v2"/>
  <path d="M2 13h20"/>
</svg>
```
Image name (bold 14 px) + base OS (muted 11.5 px). Right-aligned: "Published" badge (green) or "Build failing" badge (red) based on `image.published`.

**Section 2 — Registry** (`padding: 10px 18px; background: var(--bg-card-2)`): "REGISTRY" label (10 px uppercase muted, `font-weight: 700; letter-spacing: 0.07em`) + `docker.io/percona/{name}` in monospace (`word-break: break-all`).

**Section 3 — Available tags** (`padding: 12px 18px`): "AVAILABLE TAGS" label + wrapping flex of tag chips. `tags[0]`: `background: var(--tint-purple); color: var(--brand-purple); font-weight: 700`. All others: `background: var(--bg-muted); color: var(--text-secondary)`. All chips: monospace 11 px, `padding: 2px 7px; border-radius: 4px`.

**Section 4 — Docker pull** (`padding: 12px 18px`): "DOCKER PULL" label + `<code>` block with `background: var(--bg-card-2)` showing `image.pullCmd` + copy button. `copiedKey === image.id` → "✓ Copied" (green). Clicking emits `copy(image.id, image.pullCmd)`.

---

## File inventory

| File | Change |
|---|---|
| `backend/internal/model/types.go` | Add `ContainerTags []string` |
| `backend/internal/store/db.go` | Add `container_tags` column + migration |
| `backend/internal/store/packages.go` | Upsert + scan `container_tags` |
| `backend/internal/obs/tasks.go` | `ContainerTagsTask` stores all tags |
| `frontend/src/types/api.ts` | Add `container_tags?: string[]` |
| `frontend/src/App.vue` | Add `mainTab`, wire ArtifactsPanel |
| `frontend/src/components/AppHeader.vue` | Add tab switcher |
| `frontend/src/components/ArtifactsPanel.vue` | New |
| `frontend/src/components/ArtifactsVersionBar.vue` | New |
| `frontend/src/components/PackagesSubTab.vue` | New |
| `frontend/src/components/ContainersSubTab.vue` | New |
| `frontend/src/composables/useArtifacts.ts` | New |
