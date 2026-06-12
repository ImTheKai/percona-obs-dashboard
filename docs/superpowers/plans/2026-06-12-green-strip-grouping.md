# GreenStrip Project Grouping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Group succeeded packages in the GreenStrip by OBS project, with each group header linking to the OBS project page and each package pill linking to the OBS package page.

**Architecture:** Single-file change — `GreenStrip.vue` only. A `computed` groups the flat `packages` prop by `pkg.project`, sorts groups alphabetically, then the template iterates over groups rendering a project-link header and pill row per group.

**Tech Stack:** Vue 3 Composition API, TypeScript, inline styles + scoped CSS for hover.

**User decisions (already made):**
- Only the GreenStrip (succeeded packages) is grouped — failing package cards and their grid are untouched.
- Group headers show the full OBS project path (e.g. `isv:percona:ppg:17`), not a trimmed label.
- Package pills link to the OBS package page.
- Approach A: modify `GreenStrip.vue` in place — no new files, no caller changes.

---

## File Structure

| File | Change |
|------|--------|
| `frontend/src/components/GreenStrip.vue` | Modify — add grouping computed, replace flat pill list with grouped rendering, add scoped hover style |

No other files change.

---

### Task 1: Rewrite GreenStrip.vue with project grouping

**Goal:** Replace the flat pill list in `GreenStrip.vue` with a grouped layout: one section per OBS project, each with a linked header and linked package pills.

**Files:**
- Modify: `frontend/src/components/GreenStrip.vue`

**Acceptance Criteria:**
- [ ] `vue-tsc --noEmit` exits 0 with no errors on this file
- [ ] Packages are grouped by `pkg.project`; groups are sorted alphabetically by project path
- [ ] Each group header is an `<a>` linking to `https://build.opensuse.org/project/show/{project}` with ` ↗` suffix
- [ ] Each package pill is an `<a>` linking to `https://build.opensuse.org/package/show/{project}/{pkg.name}`
- [ ] All anchors have `target="_blank" rel="noopener"`
- [ ] The summary header ("All clear · N packages fully built") is unchanged
- [ ] Pills have `opacity: 0.85` on hover
- [ ] `FailureBoard.vue` still type-checks with no changes (prop interface is unchanged)

**Verify:** `cd frontend && ./node_modules/.bin/vue-tsc --noEmit` → exits 0, no output

**Steps:**

- [ ] **Step 1: Replace the entire contents of `GreenStrip.vue`**

  The current file has a flat `<span v-for="pkg in packages">` loop. Replace the whole file with this:

  ```vue
  <script setup lang="ts">
  import { computed } from 'vue'
  import type { Package } from '../types/api'

  const props = defineProps<{ packages: Package[] }>()

  const groups = computed(() => {
    const map = new Map<string, Package[]>()
    for (const pkg of props.packages) {
      const list = map.get(pkg.project) ?? []
      list.push(pkg)
      map.set(pkg.project, list)
    }
    return [...map.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([project, pkgs]) => ({ project, pkgs }))
  })

  function projectUrl(project: string): string {
    return `https://build.opensuse.org/project/show/${project}`
  }

  function packageUrl(project: string, name: string): string {
    return `https://build.opensuse.org/package/show/${project}/${name}`
  }
  </script>

  <template>
    <div style="background: var(--bg-card); border: 1px solid var(--border); border-radius: 12px; padding: 15px; display: flex; flex-direction: column; gap: 14px;">
      <!-- Summary header -->
      <div style="display: flex; align-items: center; gap: 9px;">
        <span style="width: 10px; height: 10px; border-radius: 3px; background: var(--ok);"></span>
        <span style="font-size: 13px; font-weight: 700; color: var(--text-primary);">All clear · {{ packages.length }} package{{ packages.length !== 1 ? 's' : '' }} fully built</span>
      </div>
      <!-- Per-project groups -->
      <div
        v-for="group in groups"
        :key="group.project"
        style="display: flex; flex-direction: column; gap: 7px; border-top: 1px solid var(--border); padding-top: 10px;"
      >
        <!-- Group header: full OBS project path linking to project page -->
        <a
          :href="projectUrl(group.project)"
          target="_blank"
          rel="noopener"
          class="project-link"
          style="font-family: var(--font-mono); font-size: 11px; color: var(--text-muted); text-decoration: none; display: inline-flex; align-items: center; gap: 3px;"
        >{{ group.project }} ↗</a>
        <!-- Package pills linking to individual OBS package pages -->
        <div style="display: flex; gap: 7px; flex-wrap: wrap;">
          <a
            v-for="pkg in group.pkgs"
            :key="pkg.name"
            :href="packageUrl(group.project, pkg.name)"
            target="_blank"
            rel="noopener"
            class="pkg-pill"
            style="display: inline-flex; align-items: center; gap: 6px; padding: 4px 10px; border-radius: 7px; background: var(--ok-tint); text-decoration: none;"
          >
            <span style="width: 6px; height: 6px; border-radius: 99px; background: var(--ok); flex-shrink: 0;"></span>
            <code style="font-family: var(--font-mono); font-size: 11px; color: var(--text-secondary);">{{ pkg.name }}</code>
          </a>
        </div>
      </div>
    </div>
  </template>

  <style scoped>
  .pkg-pill:hover {
    opacity: 0.75;
  }
  .project-link:hover {
    color: var(--text-secondary);
  }
  </style>
  ```

- [ ] **Step 2: Run the type-check**

  ```bash
  cd frontend && ./node_modules/.bin/vue-tsc --noEmit
  ```

  Expected: exits 0, no output. If there are errors, fix them before continuing.

- [ ] **Step 3: Verify visually with the dev server**

  ```bash
  cd frontend && npm run dev
  ```

  Open the app in a browser. In the main dashboard, scroll to the bottom of the left panel. Confirm:
  - The "All clear · N packages" header is still present
  - Packages are grouped under their project path (e.g. `isv:percona:ppg:17 ↗`, `isv:percona:ppg:common ↗`)
  - Groups are in alphabetical order by project path
  - Clicking a project header opens `https://build.opensuse.org/project/show/{project}` in a new tab
  - Clicking a pill opens `https://build.opensuse.org/package/show/{project}/{name}` in a new tab
  - Hovering a pill reduces its opacity
  - The failing package cards above are unaffected

  Stop the dev server (`Ctrl-C`) when done.

- [ ] **Step 4: Commit**

  ```bash
  git add frontend/src/components/GreenStrip.vue
  git commit -s -m "feat(frontend): group succeeded packages by OBS project in GreenStrip"
  ```

```json:metadata
{"files": ["frontend/src/components/GreenStrip.vue"], "verifyCommand": "cd frontend && ./node_modules/.bin/vue-tsc --noEmit", "acceptanceCriteria": ["vue-tsc exits 0", "packages grouped by pkg.project sorted alphabetically", "group header is <a> linking to OBS project page with full path and ↗ suffix", "each pill is <a> linking to OBS package page", "all anchors have target=_blank rel=noopener", "summary header unchanged", "pills have opacity 0.75 on hover", "FailureBoard type-checks unchanged"], "modelTier": "mechanical"}
```
