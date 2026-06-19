# Event Log Package Grouping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse build events by package into collapsible group rows, reducing 48 events per package (16 targets × 3 event types) to a single row with a toggle to expand.

**Architecture:** Pure client-side change. A new `PackageEventGroup.vue` component renders one collapsible package group (header + expanded child rows). `EventLog.vue` gains a `groupedMode` ref, a `groupedEvents` computed that groups `filteredEvents` by `project/package`, and a `expandedGroups` Map that preserves expand state across SSE mutations but resets on window/version changes (array identity change).

**Tech Stack:** Vue 3 Composition API, TypeScript, inline CSS variables (no external UI library).

**User decisions (already made):**
- Approach A: dedicated `PackageEventGroup.vue` component (not inline in EventLog)
- Expand state lives in `EventLog.vue` as `Map<string, boolean>`, passed down as `:expanded` prop with `@toggle` emit — so SSE `unshift()` mutations (same array reference) preserve expand state, but fetch replacements (new array reference) reset it
- Groups are ordered by their most recent event's `at` (newest first); Today/Yesterday/Earlier bucketing uses the same `getBucket()` already in EventLog
- In grouped mode the header count shows "N packages in window"; in flat mode it remains "N in window"
- Filter interaction: a group is included if ≥1 of its events passes the current filters; the group header shows the most recent event overall (index 0), regardless of filter; the event count badge shows the total unfiltered event count for the group
- `EventRow.vue` is unchanged (not reused inside child rows; child rows are rendered inline in PackageEventGroup)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `frontend/src/components/PackageEventGroup.vue` | Create | Collapsed/expanded group row: header (arrow + glyph + package name + count badge + timestamp + subtitle + scope chip) and inline child rows (20×20 glyph + connector line + eventTitle + repo/arch + timestamp) |
| `frontend/src/components/EventLog.vue` | Modify | Add `groupedMode`, `expandedGroups`, `groupedEvents` computed, `groupedAndBucketed` computed, `toggleGroup()`, Group toggle button, conditional rendering in scrollable list, updated header count |

---

### Task 1: Create PackageEventGroup.vue

**Goal:** New component that renders one package group: a clickable header row showing the most recent event, and — when expanded — an indented list of child event rows.

**Files:**
- Create: `frontend/src/components/PackageEventGroup.vue`

**Acceptance Criteria:**
- [ ] Collapsed header shows: expand arrow (▶, rotates 90° to ▼ when expanded with 0.15s CSS transition), 24×24 glyph icon colour-coded by `events[0].type`, package name (bold, truncated), "N events" pill, right-aligned relative timestamp
- [ ] Second line of header shows `events[0].what` with `on repo/arch` suffix stripped (the subtitle)
- [ ] Third line shows scope chip (same style as EventRow) and project path in monospace muted
- [ ] Clicking anywhere on the header emits `toggle`
- [ ] When `expanded` is true, an indented list of child rows appears below the header
- [ ] Each child row is an `<a>` linking to `event.url`; shows 20×20 glyph + vertical connector line (omitted on last child) + eventTitle() + optional `repo/arch` code line + right-aligned timestamp
- [ ] Expanded group receives `background: var(--bg-card-2)`, `border: 1px solid var(--border)`, `border-radius: 9px` frame
- [ ] `cd frontend && npx vue-tsc --noEmit` exits 0

**Verify:** `cd frontend && npx vue-tsc --noEmit` → exits 0 with no output

**Steps:**

- [ ] **Step 1: Create the component file**

Create `frontend/src/components/PackageEventGroup.vue` with the following complete content:

```vue
<script setup lang="ts">
import { computed } from 'vue'
import type { Event, EventType } from '../types/api'

const props = defineProps<{
  project: string
  package: string
  scope: string
  events: Event[]
  expanded: boolean
}>()

const emit = defineEmits<{ toggle: [] }>()

const GLYPH: Record<EventType, string> = {
  succeeded: '✓', failed: '✗', broken: '✗', unresolvable: '⚠',
  blocked: '⊘', published: '↑', triggered: '↻', started: '▶',
  created: '+', deleted: '−', build_started: '▶', build_finished: '■',
  version_change: '↕', updated: '◉',
}
const GLYPH_COLOR: Record<EventType, string> = {
  succeeded: 'var(--ok)', failed: 'var(--fail)', broken: 'var(--blocked)',
  unresolvable: 'var(--blocked)', blocked: 'var(--blocked)',
  published: 'var(--brand-purple)', triggered: 'var(--blocked)', started: 'var(--blocked)',
  created: 'var(--ok)', deleted: 'var(--fail)', build_started: 'var(--info)',
  build_finished: 'var(--blocked)', version_change: 'var(--blocked)', updated: 'var(--blocked)',
}
const GLYPH_BG: Record<EventType, string> = {
  succeeded: 'var(--ok-tint)', failed: 'var(--fail-tint)', broken: 'var(--blocked-tint)',
  unresolvable: 'var(--blocked-tint)', blocked: 'var(--blocked-tint)',
  published: 'var(--brand-purple-tint)', triggered: 'var(--blocked-tint)', started: 'var(--blocked-tint)',
  created: 'var(--ok-tint)', deleted: 'var(--fail-tint)', build_started: 'var(--info-tint)',
  build_finished: 'var(--blocked-tint)', version_change: 'var(--blocked-tint)', updated: 'var(--blocked-tint)',
}
const SCOPE_STYLE: Record<string, string> = {
  version:   'background: var(--brand-purple-tint); color: var(--brand-purple);',
  container: 'background: var(--info-tint); color: var(--info);',
  release:   'background: var(--ok-tint); color: var(--ok);',
  common:    'background: var(--blocked-tint); color: var(--blocked);',
  ppgcommon: 'background: var(--blocked-tint); color: var(--blocked);',
  pr:        'background: var(--warn-tint); color: var(--warn);',
}
const SCOPE_LABEL: Record<string, string> = {
  version: 'PPG', ppgcommon: 'PPG Common', common: 'Common',
  container: 'Container', release: 'Release', pr: 'PR',
}

const head = computed(() => props.events[0])

function eventTitle(event: Event): string {
  if (event.repo && event.arch) {
    return event.what.replace(` on ${event.repo}/${event.arch}`, '')
  }
  return event.what
}

function timeStr(iso: string): string {
  const d = new Date(iso)
  const diff = Date.now() - d.getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return d.toLocaleDateString()
}
</script>

<template>
  <div
    :style="{
      borderRadius: '9px',
      border: expanded ? '1px solid var(--border)' : '1px solid transparent',
      background: expanded ? 'var(--bg-card-2)' : 'transparent',
      marginBottom: expanded ? '4px' : '0',
    }"
  >
    <!-- Header row (always visible, click to toggle) -->
    <div
      @click="emit('toggle')"
      style="display: flex; gap: 9px; padding: 9px 14px; cursor: pointer; border-radius: 9px;"
    >
      <!-- Expand arrow -->
      <span
        style="width: 16px; flex-shrink: 0; font-size: 10px; color: var(--text-muted); display: flex; align-items: flex-start; padding-top: 6px; transition: transform 0.15s;"
        :style="{ transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)' }"
      >▶</span>

      <!-- Glyph -->
      <div style="flex-shrink: 0;">
        <span
          style="width: 24px; height: 24px; border-radius: 7px; display: flex; align-items: center; justify-content: center; font-size: 12px; font-weight: 800;"
          :style="{ color: GLYPH_COLOR[head.type], background: GLYPH_BG[head.type] }"
        >{{ GLYPH[head.type] }}</span>
      </div>

      <!-- Text -->
      <div style="display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1;">
        <!-- Row 1: package name + count badge + timestamp -->
        <div style="display: flex; align-items: center; gap: 8px;">
          <span style="font-size: 12.5px; font-weight: 700; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ props.package }}</span>
          <span style="font-size: 10.5px; font-weight: 600; color: var(--text-muted); background: var(--bg-muted, var(--blocked-tint)); border-radius: 5px; padding: 1px 6px; white-space: nowrap; flex-shrink: 0;">{{ events.length }} events</span>
          <span :title="head.at" style="margin-left: auto; font-size: 10.5px; color: var(--text-muted); font-family: var(--font-mono); white-space: nowrap; flex-shrink: 0;">{{ timeStr(head.at) }}</span>
        </div>
        <!-- Row 2: subtitle (most recent event's what, repo/arch stripped) -->
        <span style="font-size: 11.5px; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ eventTitle(head) }}</span>
        <!-- Row 3: scope chip + project path -->
        <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 1px;">
          <span :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${SCOPE_STYLE[scope] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`">{{ SCOPE_LABEL[scope] ?? scope }}</span>
          <code style="font-family: var(--font-mono); font-size: 10px; color: var(--text-muted);">{{ project }}</code>
        </div>
      </div>
    </div>

    <!-- Expanded child event rows -->
    <div v-if="expanded" style="padding: 0 14px 8px 14px;">
      <div v-for="(event, idx) in events" :key="event.id">
        <a
          :href="event.url"
          target="_blank"
          rel="noopener"
          style="display: flex; gap: 10px; padding: 5px 0; text-decoration: none;"
        >
          <!-- Glyph + connector -->
          <div style="display: flex; flex-direction: column; align-items: center; gap: 0; flex-shrink: 0; margin-left: 6px;">
            <span
              style="width: 20px; height: 20px; border-radius: 6px; display: flex; align-items: center; justify-content: center; font-size: 10px; font-weight: 800;"
              :style="{ color: GLYPH_COLOR[event.type], background: GLYPH_BG[event.type] }"
            >{{ GLYPH[event.type] }}</span>
            <span
              v-if="idx < events.length - 1"
              style="flex: 1; width: 2px; background: var(--border); margin-top: 2px; min-height: 8px; border-radius: 2px;"
            ></span>
          </div>
          <!-- Child text -->
          <div style="display: flex; flex-direction: column; gap: 2px; min-width: 0; padding-bottom: 4px; flex: 1;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <span style="font-size: 12px; font-weight: 600; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ eventTitle(event) }}</span>
              <span :title="event.at" style="margin-left: auto; font-size: 10.5px; color: var(--text-muted); font-family: var(--font-mono); white-space: nowrap; flex-shrink: 0;">{{ timeStr(event.at) }}</span>
            </div>
            <code v-if="event.repo" style="font-family: var(--font-mono); font-size: 11px; font-weight: 600; color: var(--text-secondary);">{{ event.repo }}/{{ event.arch }}</code>
          </div>
        </a>
      </div>
    </div>
  </div>
</template>
```

- [ ] **Step 2: Type-check**

```bash
cd frontend && npx vue-tsc --noEmit
```

Expected: exits 0 with no output.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/PackageEventGroup.vue
git commit -s -m "feat(ui): add PackageEventGroup component for grouped build events"
```

---

### Task 2: Integrate grouped mode into EventLog.vue

**Goal:** Add the `⊞ Group` toggle button, `groupedMode` state, `groupedEvents` computed, `expandedGroups` map, and conditional rendering so the event log can switch between flat and grouped views.

**Files:**
- Modify: `frontend/src/components/EventLog.vue`

**Acceptance Criteria:**
- [ ] A `⊞ Group` button appears to the right of the Filter button; it is purple-tinted when active, neutral when inactive (same visual style as Filter button)
- [ ] When grouped mode is off, the event list renders exactly as before (flat `EventRow` rows, "N in window" count)
- [ ] When grouped mode is on, events are grouped by `project + "/" + package`; each group is one `PackageEventGroup` row ordered newest-first within Today/Yesterday/Earlier buckets
- [ ] The header count reads "N packages in window" when grouped mode is on, where N is the number of groups (after filter)
- [ ] Clicking a group header toggles it expanded/collapsed
- [ ] Expanding a group shows all events for that package as child rows (newest-first)
- [ ] Active filters (type, repo, arch, package name) still work in grouped mode: a group is shown if ≥1 event passes all filters; the group header always shows the most recent event overall (index 0); event count badge shows the total (unfiltered) count for the group
- [ ] Expand state survives SSE `unshift()` mutations (same array ref) but resets when the events array is replaced by a new fetch
- [ ] `cd frontend && npx vue-tsc --noEmit` exits 0

**Verify:** `cd frontend && npx vue-tsc --noEmit` → exits 0 with no output

**Steps:**

- [ ] **Step 1: Add import for PackageEventGroup**

In `frontend/src/components/EventLog.vue`, in the `<script setup>` block, add the import after the existing `EventRow` import:

```ts
import PackageEventGroup from './PackageEventGroup.vue'
```

So the imports block becomes:

```ts
import { computed, ref, watch } from 'vue'
import TimeWindowPicker from './TimeWindowPicker.vue'
import EventRow from './EventRow.vue'
import PackageEventGroup from './PackageEventGroup.vue'
import type { Event, EventType } from '../types/api'
```

- [ ] **Step 2: Add grouped mode state and expand management**

After the `grouped` computed (which ends around line 136), add:

```ts
// ── Grouped mode ──────────────────────────────────────────────
const groupedMode = ref(false)
const expandedGroups = ref<Map<string, boolean>>(new Map())

// Reset expand state when events array is replaced by a new fetch.
// SSE mutations (unshift on the same reference) do NOT trigger this.
watch(() => props.events, (_new, old) => {
  if (_new !== old) expandedGroups.value = new Map()
}, { flush: 'sync' })

function toggleGroup(key: string) {
  const m = new Map(expandedGroups.value)
  m.set(key, !m.get(key))
  expandedGroups.value = m
}

interface PackageGroup {
  key: string
  project: string
  pkg: string
  scope: string
  events: Event[]
}

const groupedEvents = computed<PackageGroup[]>(() => {
  // Build a map of all events per project/package (unfiltered within the window)
  const allMap = new Map<string, Event[]>()
  for (const e of props.events) {
    const key = `${e.project}/${e.package}`
    if (!allMap.has(key)) allMap.set(key, [])
    allMap.get(key)!.push(e)
  }

  // Determine which keys have at least one event passing active filters
  const filteredKeys = new Set(filteredEvents.value.map(e => `${e.project}/${e.package}`))

  const result: PackageGroup[] = []
  for (const [key, evts] of allMap) {
    if (!filteredKeys.has(key)) continue
    const sorted = [...evts].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime())
    result.push({ key, project: sorted[0].project, pkg: sorted[0].package, scope: sorted[0].scope, events: sorted })
  }

  result.sort((a, b) => new Date(b.events[0].at).getTime() - new Date(a.events[0].at).getTime())
  return result
})

const groupedAndBucketed = computed(() => {
  const buckets: { bucket: Bucket; groups: PackageGroup[] }[] = [
    { bucket: 'Today', groups: [] },
    { bucket: 'Yesterday', groups: [] },
    { bucket: 'Earlier', groups: [] },
  ]
  for (const g of groupedEvents.value) {
    const b = getBucket(g.events[0].at)
    buckets.find(b2 => b2.bucket === b)!.groups.push(g)
  }
  return buckets.filter(b => b.groups.length > 0)
})
```

- [ ] **Step 3: Update the header count span**

Find the `<span>` in the title row that shows the event count (around line 146–150):

```html
<span style="font-size: 11.5px; color: var(--text-muted); font-family: var(--font-mono);">
  <template v-if="activeFilterCount > 0">{{ filteredEvents.length }} of {{ events.length }}</template>
  <template v-else>{{ events.length }}</template>
  in window
</span>
```

Replace it with:

```html
<span style="font-size: 11.5px; color: var(--text-muted); font-family: var(--font-mono);">
  <template v-if="groupedMode">{{ groupedEvents.length }} packages</template>
  <template v-else-if="activeFilterCount > 0">{{ filteredEvents.length }} of {{ events.length }}</template>
  <template v-else>{{ events.length }}</template>
  in window
</span>
```

- [ ] **Step 4: Add the Group toggle button**

Find the `<button>` for the Filter toggle (around line 166–178):

```html
<button
  @click="filterOpen = !filterOpen"
  ...
>Filter{{ activeFilterCount > 0 ? ` · ${activeFilterCount}` : '' }}</button>
```

After that button (inside the same `<div style="display: flex; ...">` row that also contains `<TimeWindowPicker>`), add:

```html
<button
  @click="groupedMode = !groupedMode"
  :style="{
    flexShrink: '0',
    fontSize: '11.5px', fontWeight: '700', padding: '4px 11px',
    borderRadius: '7px', border: '1px solid',
    cursor: 'pointer', fontFamily: 'inherit',
    display: 'inline-flex', alignItems: 'center', gap: '6px',
    background: groupedMode ? 'var(--brand-purple-tint)' : 'var(--bg-card)',
    color: groupedMode ? 'var(--brand-purple)' : 'var(--text-secondary)',
    borderColor: groupedMode ? 'var(--brand-purple)' : 'var(--border)',
  }"
>⊞ Group</button>
```

- [ ] **Step 5: Replace the scrollable event list template**

Find the scrollable list section (around line 301–309):

```html
<!-- Scrollable event list -->
<div style="overflow-y: auto; padding: 6px 4px 10px;">
  <div v-for="group in grouped" :key="group.bucket">
    <div style="padding: 11px 14px 5px; font-size: 10.5px; font-weight: 700; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em;">{{ group.bucket }}</div>
    <EventRow v-for="event in group.events" :key="event.id" :event="event" />
  </div>
  <div v-if="grouped.length === 0" style="padding: 30px 16px; text-align: center; color: var(--text-muted); font-size: 13px;">
    No events in this time window
  </div>
</div>
```

Replace with:

```html
<!-- Scrollable event list -->
<div style="overflow-y: auto; padding: 6px 4px 10px;">
  <!-- Grouped mode -->
  <template v-if="groupedMode">
    <div v-for="bucket in groupedAndBucketed" :key="bucket.bucket">
      <div style="padding: 11px 14px 5px; font-size: 10.5px; font-weight: 700; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em;">{{ bucket.bucket }}</div>
      <PackageEventGroup
        v-for="group in bucket.groups"
        :key="group.key"
        :project="group.project"
        :package="group.pkg"
        :scope="group.scope"
        :events="group.events"
        :expanded="expandedGroups.get(group.key) ?? false"
        @toggle="toggleGroup(group.key)"
      />
    </div>
    <div v-if="groupedAndBucketed.length === 0" style="padding: 30px 16px; text-align: center; color: var(--text-muted); font-size: 13px;">
      No events in this time window
    </div>
  </template>
  <!-- Flat mode -->
  <template v-else>
    <div v-for="group in grouped" :key="group.bucket">
      <div style="padding: 11px 14px 5px; font-size: 10.5px; font-weight: 700; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em;">{{ group.bucket }}</div>
      <EventRow v-for="event in group.events" :key="event.id" :event="event" />
    </div>
    <div v-if="grouped.length === 0" style="padding: 30px 16px; text-align: center; color: var(--text-muted); font-size: 13px;">
      No events in this time window
    </div>
  </template>
</div>
```

- [ ] **Step 6: Type-check**

```bash
cd frontend && npx vue-tsc --noEmit
```

Expected: exits 0 with no output.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/EventLog.vue
git commit -s -m "feat(ui): add grouped mode to event log — collapse per-package build events"
```
