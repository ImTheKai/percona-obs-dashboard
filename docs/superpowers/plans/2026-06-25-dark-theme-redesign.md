# Dark Theme Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the dark-mode color palette with a neutral cool "slate" base and cool-harmonized accents, removing the purple tint and lifting it out of the too-dark range.

**Architecture:** All dark-mode colors are CSS custom properties in the `[data-theme="dark"]` block of `frontend/src/assets/theme.css`, consumed everywhere via `var(--token)` and Tailwind. This is a pure value swap inside that one block — no markup, config, or component changes.

**Tech Stack:** Vue 3, Tailwind CSS, plain CSS custom properties.

**User decisions (already made):**
- Background mood: Slate / blue-gray (chosen over neutral-gray, warm-gray, deep-ink).
- Accent treatment: Cool harmonized — teal/cyan family (chosen over muted and refined-vivid).
- "Too dark" → base lifted from `#0B0912` to `#11161D`.
- Light theme: unchanged.

---

### Task 1: Swap dark-mode palette values

**Goal:** Replace all 24 variable values in the `[data-theme="dark"]` block of `theme.css` with the approved slate/cool palette, leaving the light `:root` block untouched.

**Files:**
- Modify: `frontend/src/assets/theme.css:36-62` (the `[data-theme="dark"]` block)

**Acceptance Criteria:**
- [ ] All 24 variables in the `[data-theme="dark"]` block match the approved values below.
- [ ] The `:root` (light) block (lines 6-33) is unchanged.
- [ ] No old purple/neon dark values remain (`#0B0912`, `#181426`, `#4A9EFF`, `#EDE8FD`, `#FF6166`, `#FF5E7A`).

**Verify:**
- `grep -nE '0B0912|181426|4A9EFF|EDE8FD|FF6166|FF5E7A' frontend/src/assets/theme.css` → no output (all old values gone).
- `grep -nE '#11161D|#3FAFCB|#E8EDF4' frontend/src/assets/theme.css` → matches (new values present).

**Steps:**

- [ ] **Step 1: Replace the dark block**

Replace the entire `[data-theme="dark"]` block (currently lines 36-62 of `frontend/src/assets/theme.css`) with:

```css
/* ── Dark theme ── */
[data-theme="dark"] {
  --bg-app: #11161D;
  --bg-card: #1A222E;
  --bg-card-2: #151B24;
  --bg-muted: #1E2733;
  --brand-purple: #3FAFCB;
  --brand-purple-tint: rgba(63, 175, 203, 0.16);
  --ok: #2DBE96;
  --ok-tint: rgba(45, 190, 150, 0.16);
  --fail: #E15F78;
  --fail-tint: rgba(225, 95, 120, 0.16);
  --warn: #DCB446;
  --warn-tint: rgba(220, 180, 70, 0.16);
  --broken: #C84A6A;
  --broken-tint: rgba(200, 74, 106, 0.16);
  --blocked: #8595A8;
  --blocked-tint: rgba(133, 149, 168, 0.16);
  --info: #5B93C9;
  --info-tint: rgba(91, 147, 201, 0.16);
  --text-primary: #E8EDF4;
  --text-secondary: #94A2B5;
  --text-muted: #6B7888;
  --border: rgba(255, 255, 255, 0.09);
  --border-strong: rgba(255, 255, 255, 0.15);
  --tech-postgres: #3FAFCB;
  --tint-postgres: rgba(63, 175, 203, 0.16);
}
```

- [ ] **Step 2: Verify old values are gone**

Run: `grep -nE '0B0912|181426|4A9EFF|EDE8FD|FF6166|FF5E7A' frontend/src/assets/theme.css`
Expected: no output.

- [ ] **Step 3: Verify new values are present**

Run: `grep -nE '#11161D|#3FAFCB|#E8EDF4' frontend/src/assets/theme.css`
Expected: three matches (`--bg-app`, `--brand-purple`/`--tech-postgres`, `--text-primary`).

- [ ] **Step 4: Visual check in the browser**

Start the dev server (`cd frontend && npm run dev`), open the app, toggle to Dark mode via the header button. Confirm:
- Backgrounds are neutral slate (no purple cast); UI feels lighter than before.
- Status glyphs/chips (succeeded/warning/failed/started) and tags render with the new cool accents.
- Text is readable cool off-white; light mode is visually unchanged when toggled back.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/assets/theme.css
git commit -s -m "feat(frontend): redesign dark theme with slate base and cool accents"
```

---

## Self-Review

- **Spec coverage:** The spec's sole deliverable — the 24-value swap in the dark block with light theme untouched — is fully covered by Task 1. All acceptance criteria from the spec are reflected.
- **Placeholder scan:** No TBD/TODO; the full CSS block is provided verbatim; verify commands are concrete.
- **Type consistency:** N/A (CSS values only); variable names match the existing file exactly.
