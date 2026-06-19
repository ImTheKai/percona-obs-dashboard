# Target Box Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the target row section of `PackageCard.vue` to a collapsible per-target design that surfaces build reason and state-specific detail in a structured labelled body.

**Architecture:** Single-file change to `frontend/src/components/PackageCard.vue`. Helper functions replace the current flat inline `v-if` spans. The target row `<a>` element becomes a `<div>` with a clickable header and a `v-show` collapsible body. Per-target expand state is tracked in a `ref<Set<string>>`.

**Tech Stack:** Vue 3 Composition API, TypeScript, CSS variables from `frontend/src/assets/theme.css`, `vue-tsc` for type checking.

**User decisions (already made):**
- Option A: single chevron toggle per target (not always-visible or per-field collapse)
- Finished title: dot + label recolour to green/red based on `details` (Option A2, not icon approach)
- Build outcome always shown in body too (A2 variant) so succeeded vs unchanged is unambiguous
- `unchanged` outcome treated as successful (green), same as `succeeded`
- Chevron hidden when target has no detail to show
- `broken` and `unresolvable` details come from `target.details` (`_result?view=status`)
- `failed` state error detail is future work — body shows build reason only

---

## File Structure

Only one file changes:

- **Modify:** `frontend/src/components/PackageCard.vue` — add helper functions for per-target colour, expand state, and detail content; replace the `<a>` target row with a `<div>` + collapsible body

---

### Task 1: Refactor PackageCard.vue target rows

**Goal:** Replace the flat target row `<a>` elements with collapsible `<div>` rows that show build reason and state-specific detail in a structured body.

**Files:**
- Modify: `frontend/src/components/PackageCard.vue`

**Acceptance Criteria:**
- [ ] Target header row shows: dot, `repo/arch`, state label (right-aligned), `log ↗` link, chevron (▸/▾) — chevron absent when target has no detail
- [ ] Clicking the header toggles the body open/closed; clicking `log ↗` navigates without toggling
- [ ] Body shows `Build reason` section when `t.build_reason` is set; packages appended as `: pkg1, pkg2` if `t.build_reason_packages` is non-empty
- [ ] Body shows state-specific section: blocked → `Waiting for` + `t.blocked_by` in amber; finished (succeeded/unchanged) → `Build outcome` + word in green; finished (failed) → `Build outcome` + "failed" in red; unresolvable → `Unresolvable` + `t.details` in purple; broken → `Broken` + `t.details` in red/pink
- [ ] `finished` dot and state label recolour: green for succeeded/unchanged, red for failed, amber when `details` is empty
- [ ] `finished` background recolours to match: `--ok-tint` for succeeded/unchanged, `--fail-tint` for failed, `--warn-tint` when unknown
- [ ] `showAll` / "show N more" buttons still work as before
- [ ] `vue-tsc --noEmit` passes with no errors in the `frontend/` directory

**Verify:** `cd frontend && npm run build` → exits 0 with no TypeScript errors

**Steps:**

- [ ] **Step 1: Replace the script section**

Replace the entire `<script setup lang="ts">` block (lines 1–61 of the current file) with the following:

```vue
<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Package, Target } from '../types/api'

const props = defineProps<{ pkg: Package }>()

const SKIP_STATES = new Set(['disabled', 'excluded', 'locked'])

const STATE_COLOR: Record<string, string> = {
  succeeded: 'var(--ok)',
  failed: 'var(--fail)',
  unresolvable: 'var(--brand-purple)',
  broken: 'var(--broken)',
  blocked: 'var(--blocked)',
  building: 'var(--info)',
  finished: 'var(--warn)',
  scheduled: 'var(--info)',
}

const STATE_BG: Record<string, string> = {
  succeeded: 'var(--ok-tint)',
  failed: 'var(--fail-tint)',
  unresolvable: 'var(--brand-purple-tint)',
  broken: 'var(--broken-tint)',
  blocked: 'var(--blocked-tint)',
  building: 'var(--info-tint)',
  finished: 'var(--warn-tint)',
  scheduled: 'var(--info-tint)',
}

const STATE_LABEL: Record<string, string> = {
  succeeded: 'Succeeded', failed: 'Failed', unresolvable: 'Unresolvable',
  broken: 'Broken', blocked: 'Blocked', building: 'Building',
  finished: 'Finishing', scheduled: 'Scheduled',
}

const SCOPE_LABEL: Record<string, string> = {
  common: 'Common', ppgcommon: 'PPG Common', version: 'Version',
  container: 'Container', release: 'Release',
}

const INITIAL_VISIBLE = 3

const showAll = ref(false)
const expandedTargets = ref(new Set<string>())

function targetKey(t: Target): string {
  return `${t.repo}/${t.arch}`
}

function toggleTarget(t: Target) {
  if (!hasDetail(t)) return
  const key = targetKey(t)
  const next = new Set(expandedTargets.value)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  expandedTargets.value = next
}

function isExpanded(t: Target): boolean {
  return expandedTargets.value.has(targetKey(t))
}

function hasDetail(t: Target): boolean {
  if (t.build_reason) return true
  if (t.state === 'blocked' && t.blocked_by) return true
  if ((t.state === 'finished' || t.state === 'unresolvable' || t.state === 'broken') && t.details) return true
  return false
}

function finishedOutcome(t: Target): 'ok' | 'fail' | 'unknown' {
  if (t.state !== 'finished') return 'unknown'
  if (t.details === 'succeeded' || t.details === 'unchanged') return 'ok'
  if (t.details === 'failed') return 'fail'
  return 'unknown'
}

function targetDotColor(t: Target): string {
  if (t.state === 'finished') {
    const o = finishedOutcome(t)
    if (o === 'ok') return 'var(--ok)'
    if (o === 'fail') return 'var(--fail)'
  }
  return STATE_COLOR[t.state] ?? 'var(--blocked)'
}

function targetLabelColor(t: Target): string {
  return targetDotColor(t)
}

function targetBg(t: Target): string {
  if (t.state === 'finished') {
    const o = finishedOutcome(t)
    if (o === 'ok') return 'var(--ok-tint)'
    if (o === 'fail') return 'var(--fail-tint)'
  }
  return STATE_BG[t.state] ?? 'var(--blocked-tint)'
}

function buildReasonText(t: Target): string {
  if (!t.build_reason) return ''
  if (t.build_reason_packages?.length) return `${t.build_reason}: ${t.build_reason_packages.join(', ')}`
  return t.build_reason
}

function stateDetailLabel(t: Target): string {
  if (t.state === 'blocked') return 'Waiting for'
  if (t.state === 'finished') return 'Build outcome'
  if (t.state === 'unresolvable') return 'Unresolvable'
  if (t.state === 'broken') return 'Broken'
  return ''
}

function stateDetailValue(t: Target): string {
  if (t.state === 'blocked') return t.blocked_by ?? ''
  if (t.state === 'finished') return t.details ?? ''
  if (t.state === 'unresolvable') return t.details ?? ''
  if (t.state === 'broken') return t.details ?? ''
  return ''
}

function stateDetailColor(t: Target): string {
  if (t.state === 'blocked') return 'var(--warn)'
  if (t.state === 'finished') {
    const o = finishedOutcome(t)
    if (o === 'ok') return 'var(--ok)'
    if (o === 'fail') return 'var(--fail)'
  }
  if (t.state === 'unresolvable') return 'var(--brand-purple)'
  if (t.state === 'broken') return 'var(--broken)'
  return 'var(--text-muted)'
}

const failingTargets = computed(() =>
  props.pkg.targets.filter(t => !SKIP_STATES.has(t.state) && t.state !== 'succeeded')
)
const visibleFailing = computed(() =>
  showAll.value ? failingTargets.value : failingTargets.value.slice(0, INITIAL_VISIBLE)
)
const hiddenCount = computed(() => Math.max(0, failingTargets.value.length - INITIAL_VISIBLE))

const rollupColor = computed(() => STATE_COLOR[props.pkg.rollup_state] ?? 'var(--text-muted)')
const rollupBg = computed(() => STATE_BG[props.pkg.rollup_state] ?? 'var(--blocked-tint)')
const obsUrl = computed(() => `https://build.opensuse.org/package/show/${props.pkg.project}/${props.pkg.name}`)

function logUrl(repo: string, arch: string): string {
  return `https://build.opensuse.org/package/live_build_log/${props.pkg.project}/${props.pkg.name}/${repo}/${arch}`
}
</script>
```

Note: The `import type { Package }` line becomes `import type { Package, Target }` — `Target` is now needed for the helper function signatures.

- [ ] **Step 2: Replace the template's target list section**

In the `<template>`, replace the `<!-- Row 3: failing targets -->` block (the entire `<div v-if="failingTargets.length > 0">` section, lines 92–143) with:

```vue
    <!-- Row 3: failing targets -->
    <div v-if="failingTargets.length > 0" style="display: flex; flex-direction: column; gap: 6px;">
      <span style="font-size: 10.5px; font-weight: 700; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em;">
        {{ failingTargets.length }} failing target{{ failingTargets.length !== 1 ? 's' : '' }}
      </span>
      <div style="display: flex; flex-direction: column; gap: 5px;">
        <div
          v-for="t in visibleFailing"
          :key="targetKey(t)"
          :style="{
            borderRadius: '7px',
            overflow: 'hidden',
            background: targetBg(t),
          }"
        >
          <!-- Target header row -->
          <div
            :style="{
              display: 'flex', alignItems: 'center', gap: '9px',
              padding: '5px 9px',
              cursor: hasDetail(t) ? 'pointer' : 'default',
              userSelect: 'none',
            }"
            @click="toggleTarget(t)"
          >
            <span :style="{ width: '8px', height: '8px', borderRadius: '2px', background: targetDotColor(t), flexShrink: '0' }"></span>
            <code style="font-family: var(--font-mono); font-size: 11.5px; color: var(--text-primary); flex-shrink: 0;">{{ t.repo }}/{{ t.arch }}</code>
            <span :style="{ fontSize: '11px', color: targetLabelColor(t), marginLeft: 'auto', fontWeight: '600', flexShrink: '0' }">{{ STATE_LABEL[t.state] ?? t.state }}</span>
            <a
              :href="logUrl(t.repo, t.arch)"
              target="_blank"
              rel="noopener"
              style="font-size: 10.5px; color: var(--brand-purple); font-weight: 700; flex-shrink: 0; text-decoration: none;"
              @click.stop
            >log ↗</a>
            <span
              v-if="hasDetail(t)"
              style="font-size: 10px; color: var(--text-muted); flex-shrink: 0; width: 12px; text-align: center;"
            >{{ isExpanded(t) ? '▾' : '▸' }}</span>
          </div>

          <!-- Target body (collapsible) -->
          <div
            v-show="isExpanded(t)"
            style="padding: 0 9px 8px calc(9px + 8px + 9px); display: flex; flex-direction: column; gap: 5px;"
          >
            <hr style="border: none; border-top: 1px solid var(--border); margin: 0 0 3px;" />
            <div v-if="buildReasonText(t)" style="display: flex; flex-direction: column; gap: 1px;">
              <span style="font-size: 9px; text-transform: uppercase; letter-spacing: 0.07em; color: var(--text-muted); font-weight: 700;">Build reason</span>
              <span style="font-family: var(--font-mono); font-size: 10.5px; color: var(--text-secondary); line-height: 1.4;">{{ buildReasonText(t) }}</span>
            </div>
            <div v-if="stateDetailValue(t)" style="display: flex; flex-direction: column; gap: 1px;">
              <span style="font-size: 9px; text-transform: uppercase; letter-spacing: 0.07em; color: var(--text-muted); font-weight: 700;">{{ stateDetailLabel(t) }}</span>
              <span :style="{ fontFamily: 'var(--font-mono)', fontSize: '10.5px', color: stateDetailColor(t), fontWeight: '600', lineHeight: '1.4' }">{{ stateDetailValue(t) }}</span>
            </div>
          </div>
        </div>

        <button
          v-if="!showAll && hiddenCount > 0"
          @click="showAll = true"
          style="font-size: 11px; color: var(--brand-purple); font-weight: 600; padding: 4px 9px; border: none; background: transparent; cursor: pointer; text-align: left; font-family: inherit;"
        >+ {{ hiddenCount }} more</button>
        <button
          v-if="showAll && hiddenCount > 0"
          @click="showAll = false"
          style="font-size: 11px; color: var(--text-muted); font-weight: 600; padding: 4px 9px; border: none; background: transparent; cursor: pointer; text-align: left; font-family: inherit;"
        >Show less</button>
      </div>
    </div>
```

- [ ] **Step 3: Type-check**

```bash
cd frontend && npx vue-tsc --noEmit
```

Expected: no output, exit 0. If there are errors, fix them before proceeding.

- [ ] **Step 4: Build check**

```bash
cd frontend && npm run build
```

Expected: `✓ built in ...` with no TypeScript errors. The build output goes to `frontend/dist/`.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/PackageCard.vue
git commit -s -m "feat: collapsible target rows with state-specific detail"
```

```json:metadata
{"files": ["frontend/src/components/PackageCard.vue"], "verifyCommand": "cd frontend && npm run build", "acceptanceCriteria": ["Target header shows dot, repo/arch, state label, log link, chevron", "Chevron absent when target has no detail", "Clicking header toggles body; log link navigates without toggling", "Body shows Build reason section when build_reason is set", "Body shows state-specific section per state table in spec", "finished dot/label/bg recolour based on details value", "showAll / show N more still works", "vue-tsc passes with no errors"], "modelTier": "mechanical"}
```
