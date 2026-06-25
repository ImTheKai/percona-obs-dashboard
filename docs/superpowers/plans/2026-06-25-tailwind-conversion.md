# Tailwind Conversion (Stage 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert every frontend component from inline `style="…"` and scoped-CSS layout to Tailwind utility classes with zero visual or behavioral change (desktop, light + dark).

**Architecture:** Tailwind is already configured; `tailwind.config.ts` maps the theme CSS variables to color utilities (`bg-bg-app`, `text-text-primary`, `border-border`, `text-ok`, …) and `theme.css` holds the values. Conversion reuses those tokens, so a converted class resolves to the same CSS variable the inline style used — dark mode keeps working untouched. Each component is scoped and independent, so conversions don't interfere and are individually committable/verifiable.

**Tech Stack:** Vue 3 (SFC), TypeScript, Tailwind CSS 3, Vite.

**User decisions (already made):**
- Two-stage approach: Stage 1 = Tailwind conversion with NO mobile/responsive changes; Stage 2 = responsive reflow later (separate plan).
- Preserve exact appearance: faithful conversion, no visual drift.
- Value fidelity: standard Tailwind scale where it matches; arbitrary values (`text-[13px]`, `rounded-[14px]`, `shadow-[…]`) where off-scale; theme color tokens for colors.
- Keep minimal scoped `<style>` only for what Tailwind can't express (scrollbars, `@keyframes`, pseudo-elements, complex selectors).
- Dynamic data-driven `:style` bindings (status glyph colors, progress-bar widths) may remain as bindings.

---

## Conversion Procedure (apply to every task below)

For each component file:

1. **Read the whole SFC** — note every `style="…"` attribute, every `:style` binding, and every rule in the `<style>` block.
2. **Convert static inline `style="…"`** to Tailwind classes on the same element:
   - Match Tailwind's default scale where the px value already maps exactly: `8px→2`, `12px→3`, `16px→4`, `20px→5`, `24px→6` (e.g. `gap: 16px`→`gap-4`, `padding: 24px`→`p-6`).
   - Use **arbitrary values** for anything off-scale, preserving the exact value: `font-size:13px`→`text-[13px]`, `border-radius:14px`→`rounded-[14px]`, `gap:18px`→`gap-[18px]`, `padding:10px 11px`→`py-[10px] px-[11px]`, `box-shadow:0 1px 2px rgba(0,0,0,.12)`→`shadow-[0_1px_2px_rgba(0,0,0,0.12)]`, fixed widths→`w-[440px]`/`w-[220px]`, `max-width:1360px`→`max-w-[1360px]`.
   - Colors → theme tokens: `var(--bg-card)`→`bg-bg-card`, `var(--text-muted)`→`text-text-muted`, `var(--border)`→`border-border`, `var(--ok)`→`text-ok`/`bg-ok` as appropriate. Never re-hardcode a hex that a token already represents.
   - `display:flex`→`flex`, `flex-direction:column`→`flex-col`, `align-items:center`→`items-center`, `justify-content:space-between`→`justify-between`, `flex-wrap:wrap`→`flex-wrap`, grids→`grid grid-cols-[…]` with arbitrary track lists preserved (e.g. `grid-template-columns:minmax(0,1fr) 440px`→`grid-cols-[minmax(0,1fr)_440px]`).
3. **Convert scoped `<style>` layout rules** the same way, moving them onto the elements as classes. Convert `:hover`/`:disabled`/`.active`/`.expanded` to Tailwind variants (`hover:`, `disabled:`) or, where a parent state drives the styling, the existing class can stay or use a data-/group- variant — but reproduce the identical visual states.
4. **Keep in a minimal scoped `<style>` block** ONLY: `::-webkit-scrollbar*` rules, `@keyframes` + the elements that animate, `::before`/`::after`, and genuinely complex descendant/combinator selectors. Delete everything else from `<style>` once moved.
5. **Leave dynamic `:style` bindings** that compute from data (e.g. `:style="{ color: GLYPH_COLOR[type] }"`, progress-bar `width`) as bindings — do not attempt to classify them.
6. **No markup/structure changes, no responsive (`sm:`/`md:`/`lg:`) utilities** — those are Stage 2.

**Standard Verify for every task:** `cd frontend && npm run build` → completes with no `vue-tsc` or Vite errors (ends with `✓ built in …`).

**Standard equivalence self-review for every task:** for each element changed, confirm every original declaration has an exact Tailwind equivalent (same values, same `:hover`/`:disabled`/`.active` states, same dark-mode token) — nothing dropped, nothing added.

---

### Task 1: Convert leaf components

**Goal:** Convert the five leaf components to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/EventRow.vue`
- Modify: `frontend/src/components/PackageCard.vue`
- Modify: `frontend/src/components/GreenStrip.vue`
- Modify: `frontend/src/components/TimeWindowPicker.vue`
- Modify: `frontend/src/components/PRBoard.vue`

**Acceptance Criteria:**
- [ ] Static inline styles and scoped layout CSS in all five files converted to Tailwind per the Conversion Procedure.
- [ ] Data-driven `:style` bindings (e.g. `PackageCard`/`EventRow` glyph colors from `useEventDisplay`) left as bindings.
- [ ] `npm run build` passes.
- [ ] Equivalence self-review passes for each file.

**Verify:** `cd frontend && npm run build` → `✓ built` with no errors.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure to each of the five files in turn. These are small; watch specifically for `:style` color/glyph bindings sourced from `useEventDisplay.ts` (`GLYPH_COLOR`, `GLYPH_BG`, `TAG_STYLE`) — keep those as bindings.
- [ ] **Step 2:** Run `cd frontend && npm run build`; expect `✓ built`.
- [ ] **Step 3:** Equivalence self-review for each file.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/EventRow.vue frontend/src/components/PackageCard.vue frontend/src/components/GreenStrip.vue frontend/src/components/TimeWindowPicker.vue frontend/src/components/PRBoard.vue
git commit -s -m "refactor(frontend): convert leaf components to Tailwind"
```

---

### Task 2: Convert AppHeader

**Goal:** Convert `AppHeader.vue` (title, theme toggle, Build/Artifacts tab switcher) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/AppHeader.vue`

**Acceptance Criteria:**
- [ ] Inline header styles and the `.tab-switcher`/`.tab-pill`/`.tab-pill.active` scoped rules converted to Tailwind, reproducing the selected-pill border/shadow states exactly (from the recent dark-mode work).
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. Note: `.tab-pill` has `border:1px solid transparent` base and `.active` adds `border-color:var(--border-strong)` + `box-shadow` — reproduce with `border border-transparent` base and active `border-border-strong shadow-[0_1px_2px_rgba(0,0,0,0.12)]`.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/AppHeader.vue
git commit -s -m "refactor(frontend): convert AppHeader to Tailwind"
```

---

### Task 3: Convert ContextBar

**Goal:** Convert `ContextBar.vue` (context/version/tags/refresh control bar) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/ContextBar.vue`

**Acceptance Criteria:**
- [ ] Inline styles and scoped layout converted; existing `flex-wrap` behavior preserved exactly.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/ContextBar.vue
git commit -s -m "refactor(frontend): convert ContextBar to Tailwind"
```

---

### Task 4: Convert HealthHeader

**Goal:** Convert `HealthHeader.vue` (summary counts, progress bar, issue pills) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/HealthHeader.vue`

**Acceptance Criteria:**
- [ ] Inline styles and scoped layout converted; the `min-width:300px`/`max-width:520px` constraints preserved exactly as arbitrary values (`min-w-[300px]`, `max-w-[520px]`) — they are NOT removed in Stage 1.
- [ ] The progress-bar fill `:style` width binding left as a binding.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. Preserve `min-w-[300px]` and `max-w-[520px]` (Stage 2 removes/relaxes them, not now).
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/HealthHeader.vue
git commit -s -m "refactor(frontend): convert HealthHeader to Tailwind"
```

---

### Task 5: Convert ArtifactsVersionBar

**Goal:** Convert `ArtifactsVersionBar.vue` (version segment selector) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/ArtifactsVersionBar.vue`

**Acceptance Criteria:**
- [ ] Inline styles and the `.segment`/`.seg-btn`/`.seg-btn.active` scoped rules converted; reproduce the active-segment border (`border-border-strong`) + shadow from the recent dark-mode work exactly.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure (mirror the AppHeader pill treatment for the active segment).
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/ArtifactsVersionBar.vue
git commit -s -m "refactor(frontend): convert ArtifactsVersionBar to Tailwind"
```

---

### Task 6: Convert ArtifactsPanel

**Goal:** Convert `ArtifactsPanel.vue` (sub-tab container for Packages/Containers) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/ArtifactsPanel.vue`

**Acceptance Criteria:**
- [ ] Inline styles and scoped layout converted.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/ArtifactsPanel.vue
git commit -s -m "refactor(frontend): convert ArtifactsPanel to Tailwind"
```

---

### Task 7: Convert PackageEventGroup

**Goal:** Convert `PackageEventGroup.vue` (grouped/expandable events for one package) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/PackageEventGroup.vue`

**Acceptance Criteria:**
- [ ] Inline styles and scoped layout converted; expand/collapse visual states reproduced exactly.
- [ ] Data-driven `:style` bindings (glyph colors) left as bindings.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/PackageEventGroup.vue
git commit -s -m "refactor(frontend): convert PackageEventGroup to Tailwind"
```

---

### Task 8: Convert FailureBoard

**Goal:** Convert `FailureBoard.vue` (2-column grid of failing PackageCards) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/FailureBoard.vue`

**Acceptance Criteria:**
- [ ] The `grid-template-columns: repeat(2, minmax(0,1fr))` grid converted to `grid grid-cols-[repeat(2,minmax(0,1fr))]` (or `grid-cols-2` only if the computed result is identical), gap preserved.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. Keep it 2-column (no responsive change).
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/FailureBoard.vue
git commit -s -m "refactor(frontend): convert FailureBoard to Tailwind"
```

---

### Task 9: Convert MainGrid

**Goal:** Convert `MainGrid.vue` (board left column + 440px EventLog right column) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/MainGrid.vue`

**Acceptance Criteria:**
- [ ] The `grid-template-columns: minmax(0,1fr) 440px` grid converted to `grid grid-cols-[minmax(0,1fr)_440px]` with `gap`/`align-items` preserved (`items-start`).
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. Keep the fixed 440px column (no responsive change).
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/MainGrid.vue
git commit -s -m "refactor(frontend): convert MainGrid to Tailwind"
```

---

### Task 10: Convert EventLog

**Goal:** Convert `EventLog.vue` (sticky event sidebar with filter dropdowns) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/EventLog.vue`

**Acceptance Criteria:**
- [ ] Inline styles and scoped layout converted; `position: sticky; top:16px`→`sticky top-4`, `max-height: calc(100vh - 40px)`→`max-h-[calc(100vh-40px)]`, dropdown `min-width:200px`→`min-w-[200px]`, all preserved.
- [ ] Any `::-webkit-scrollbar` styling kept in a minimal scoped `<style>`.
- [ ] Data-driven `:style` bindings (glyph colors) left as bindings.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. Keep scrollbar CSS scoped; preserve sticky + max-height + dropdown widths.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review (filters, dropdowns open state, sticky behavior, dark mode).
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/EventLog.vue
git commit -s -m "refactor(frontend): convert EventLog to Tailwind"
```

---

### Task 11: Convert PackagesSubTab

**Goal:** Convert `PackagesSubTab.vue` (220px repo sidebar + package list + arch pills) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/PackagesSubTab.vue`

**Acceptance Criteria:**
- [ ] Inline styles and scoped layout converted; `.sidebar` `width:220px; flex-shrink:0`→`w-[220px] shrink-0`, the `.arch-selector`/`.arch-pill`/`.arch-pill.active` treatment reproduced (active border + shadow), and the status-badge classes (`.status-built`, `.status-building`, `.status-failed`, `.status-other`, `.status-stale-warning`) preserved with identical colors/states.
- [ ] `.spinner` `@keyframes` (if present) and any `::-webkit-scrollbar` kept in a minimal scoped `<style>`.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. This is the largest file — keep the spinner keyframes and scrollbar rules scoped; the badge color classes can stay as scoped classes if cleaner than long arbitrary-value soups, but their values must be unchanged.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review (sidebar, arch pills active state, badges, rebuilding warning, expanded rows, dark mode).
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/PackagesSubTab.vue
git commit -s -m "refactor(frontend): convert PackagesSubTab to Tailwind"
```

---

### Task 12: Convert ContainersSubTab

**Goal:** Convert `ContainersSubTab.vue` (image card grid + CVE security table) to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/components/ContainersSubTab.vue`

**Acceptance Criteria:**
- [ ] `.images-grid` `grid-template-columns: repeat(auto-fill, minmax(340px,1fr))`→`grid-cols-[repeat(auto-fill,minmax(340px,1fr))]`; CVE table `white-space:nowrap`→`whitespace-nowrap` and `overflow-x:auto`→`overflow-x-auto` preserved (table layout unchanged in Stage 1).
- [ ] Any `::-webkit-scrollbar` kept scoped.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure. Keep the CVE table as a table (Stage 2 reflows it to cards).
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review (image cards, CVE table, dark mode).
- [ ] **Step 4: Commit**
```bash
git add frontend/src/components/ContainersSubTab.vue
git commit -s -m "refactor(frontend): convert ContainersSubTab to Tailwind"
```

---

### Task 13: Convert App.vue root

**Goal:** Convert the `App.vue` root container styling to Tailwind with no visual change.

**Files:**
- Modify: `frontend/src/App.vue`

**Acceptance Criteria:**
- [ ] Root inline styles converted: `padding:24px 28px 60px`→`pt-6 px-7 pb-[60px]` (24=6, 28=`px-7`, 60 arbitrary), inner `max-width:1360px; margin:0 auto`→`max-w-[1360px] mx-auto`, `display:flex; flex-direction:column; gap:16px`→`flex flex-col gap-4`. The existing `min-h-screen bg-bg-app` Tailwind classes stay.
- [ ] No change to the theme/`prefers-color-scheme` script logic.
- [ ] `npm run build` passes; equivalence self-review passes.

**Verify:** `cd frontend && npm run build` → `✓ built`.

**Steps:**

- [ ] **Step 1:** Apply the Conversion Procedure to the two root container divs only. Do not touch the `<script setup>` logic.
- [ ] **Step 2:** `cd frontend && npm run build` → `✓ built`.
- [ ] **Step 3:** Equivalence self-review.
- [ ] **Step 4: Commit**
```bash
git add frontend/src/App.vue
git commit -s -m "refactor(frontend): convert App root container to Tailwind"
```

---

## Self-Review

- **Spec coverage:** Every component listed in the spec's in-scope list has a task (leaf batch = Task 1; mid = Tasks 2–7; complex = Tasks 8–12; root = Task 13). The spec's conventions are captured in the shared Conversion Procedure; verification (build + equivalence self-review) is in every task; human spot-check happens at execution review checkpoints. Out-of-scope items (responsiveness, token-value changes, markup changes) are explicitly excluded in each task.
- **Placeholder scan:** No TBD/TODO; each task names exact files, exact conversions for the notable values, exact verify command and commit.
- **Type/naming consistency:** Class/value mappings (`w-[220px]`, `grid-cols-[minmax(0,1fr)_440px]`, active-pill `border-border-strong`) are consistent across tasks and match the current source values from the spec/exploration.

## Notes for the executor

- Tasks are independent (disjoint files, scoped styles) and MAY be run in parallel; do a human light/dark spot-check after each batch against pre-conversion `main`.
- This is Stage 1 only. Do NOT add any `sm:`/`md:`/`lg:` responsive utilities or change any layout values — that is Stage 2 (see the spec appendix).
