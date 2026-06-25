# Responsive Mobile Reflow (Stage 2) — Design

**Date:** 2026-06-25
**Status:** Approved (design)

## Context

This is **Stage 2** of the mobile-responsiveness work. Stage 1 converted all components from inline styles + scoped CSS to Tailwind utilities with no visual change (see `2026-06-25-tailwind-conversion-design.md`). Stage 2 adds the responsive reflow on top of that clean Tailwind base. The desktop layout (≥1024px) must remain visually identical to today; all changes are additive responsive behavior below desktop.

## Problem

The dashboard is desktop-only: fixed multi-column grids, a fixed 440px event-log sidebar, and a fixed 220px repo sidebar cause horizontal scrolling and unusable layouts on phones and tablets. There is no responsive handling anywhere.

## Goal

A **responsive reflow**: the same features and information on every screen, with no horizontal scrolling, readable text, and tappable controls. This is not a separate mobile redesign — it is the existing UI reflowing across three size tiers.

## Decisions (locked during brainstorming)

- **Goal:** responsive reflow, same features everywhere, no horizontal scroll.
- **Tiers:** phone (<640px), tablet (640–1024px), desktop (≥1024px), via Tailwind `sm` (640px) and `lg` (1024px). No custom breakpoints.
- **Board:** single-column stack below desktop; FailureBoard cards 1-col on phone, 2-col from `sm`.
- **Event Log:** collapsible section (collapsed by default) on phone/tablet; sticky side panel at `lg`.
- **Packages repo sidebar:** full-width grouped dropdown below `lg`; 220px sidebar at `lg`.
- **CVE security table:** stacked per-CVE cards on phone; `<table>` at `sm`+.
- **AppHeader (phone):** compact single row — subtitle hidden, title truncates, tab switcher kept, theme toggle becomes icon-only. Full header at `sm`+.
- **Shared infra:** a `useMediaQuery` composable for the event-log collapse behavior.

## Scope

**In scope:** Responsive Tailwind utilities on `App.vue`, `AppHeader.vue`, `ContextBar.vue`, `HealthHeader.vue`, `MainGrid.vue`, `FailureBoard.vue`, `EventLog.vue`, `ArtifactsVersionBar.vue`, `PackagesSubTab.vue`, `ContainersSubTab.vue`; a new `useMediaQuery.ts` composable; a phone CVE-card markup block; a phone repo `<select>`.

**Out of scope:** Any change to desktop (≥1024px) appearance; data/logic changes; new features; restyling colors or spacing beyond what the reflow requires; touch gestures or mobile-only navigation paradigms.

## Architecture

The reflow is overwhelmingly **pure CSS via Tailwind responsive prefixes** applied to existing elements — no markup duplication except where a structure genuinely cannot be one DOM (the CVE table vs. cards). Visibility toggles (`hidden`/`lg:hidden`/`hidden sm:table`) render both desktop and mobile variants and show the right one per breakpoint; this avoids JS for the repo dropdown (the `<select>` drives the same state via the existing emit).

The single piece of JS-driven behavior is the **event-log collapse**, because the component must behave differently (always-open side panel vs. collapsed-by-default section) depending on viewport — a state difference, not just styling. A small `useMediaQuery` composable provides a reactive `isDesktop` boolean for this.

### New unit: `useMediaQuery`

`frontend/src/composables/useMediaQuery.ts`

- **What it does:** Given a media-query string, returns a reactive `Ref<boolean>` that tracks whether the query matches.
- **How to use:** `const isDesktop = useMediaQuery('(min-width: 1024px)')`.
- **Depends on:** `window.matchMedia`, Vue `ref`/`onMounted`/`onUnmounted`. Initializes from `matchMedia(...).matches`, updates on the media query's `change` event, and removes the listener on unmount.

## Per-view design

### Root (`App.vue`)
- Horizontal padding: `px-7` → `px-4 sm:px-7` (less edge-hugging at 375px, unchanged at `sm`+). The `max-w-[1360px] mx-auto` wrapper is unchanged (max-width already collapses on small screens).

### Board view

**AppHeader** — below `sm`, one compact row:
- Subtitle: `hidden sm:block`.
- Title: `truncate` (ellipsis) so it never pushes the row wider.
- Tab switcher: kept; tighter padding on phone.
- Theme toggle: the text label (e.g. "Dark mode") is `hidden sm:inline`, leaving the colored dot / a glyph as an icon-only control on phone.
- At `sm`+: identical to today (`flex items-center justify-between`).

**MainGrid** — the two-column grid is desktop-only:
- `grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_440px] gap-[18px] items-start`.
- Below `lg`: single column — FailureBoard stacks above the EventLog. The fixed 440px column appears only at `lg`.

**FailureBoard** — `grid-cols-1 sm:grid-cols-[repeat(2,minmax(0,1fr))]` (one card per row on phone, two from `sm`).

**HealthHeader** — relax blocking constraints:
- Left summary column: `min-w-0 sm:min-w-[300px]`.
- Container gap: `gap-4 sm:gap-[30px]`.
- Already `flex-wrap`; summary / progress bar / issue pills stack on narrow screens with no other change.

**ContextBar** — already `flex-wrap`; only tighten gaps on phone (`gap-2 sm:gap-4`). No structural change.

**EventLog** — uses `const isDesktop = useMediaQuery('(min-width: 1024px)')`:
- **Desktop:** unchanged — `sticky top-4`, 440px column (from MainGrid), `max-h-[calc(100vh-40px)]`, always visible.
- **Phone/tablet:** a collapsible section. An `expanded` ref defaults to `false`. A full-width tappable header row shows "Event Log" and the event count; tapping toggles the body. Not sticky, not height-capped (flows in the page). When `isDesktop` is true the panel renders always-open with no toggle header.

### Artifacts view

**ArtifactsVersionBar** — already `flex-wrap`; tighten gaps on phone only. No structural change.

**PackagesSubTab** — sidebar → dropdown below `lg`:
- Container: `flex flex-col lg:flex-row gap-4 ...`.
- The `w-[220px]` repo sidebar: `hidden lg:flex`.
- A full-width grouped `<select>` rendered `lg:hidden`, with `<optgroup>` per distro (RHEL / openSUSE / Ubuntu / Debian / Other) mirroring the sidebar grouping, bound to `selectedRepo` and emitting `update:art-repo` on change. Both controls always exist; CSS shows the right one. No JS media query needed here.
- Arch pills and package list already flow full-width.

**ContainersSubTab:**
- **Image grid:** `grid-cols-1 sm:grid-cols-[repeat(auto-fill,minmax(340px,1fr))]` (single column on phone to avoid the 340px min overflowing ~375px once padding is counted; auto-fill from `sm`).
- **CVE security table → cards on phone.** This is the one structural duplication: a card list `<div>` (`sm:hidden`) renders each CVE as a stacked card — severity + CVE ID prominent, then Package, Installed→Fixed, and Title as labeled lines; the existing `<table>` becomes `hidden sm:table`. Both are driven by the same CVE data.

## Error handling

Not applicable beyond the composable: `useMediaQuery` guards listener setup/teardown in `onMounted`/`onUnmounted` so there is no leak and no SSR access to `window` at module scope.

## Testing / Verification

No automated UI test harness exists; verification is:
1. **Build green:** `cd frontend && npm run build` (vue-tsc + vite) passes.
2. **`useMediaQuery` sanity:** confirm the event log collapses/expands as the viewport crosses 1024px (manually, or a small mocked-`matchMedia` test if a runner is added).
3. **Manual responsive check** in browser devtools at **375px**, **768px**, **1280px** for each view:
   - No horizontal scrolling at any width.
   - Board: compact header on phone; failures stack (1-col phone, 2-col `sm`+); event log collapsed-by-default below `lg`, sticky side panel at `lg`.
   - Artifacts: repo dropdown below `lg` / sidebar at `lg`; CVE cards on phone / table at `sm`+; image cards single-column on phone.
   - Desktop (≥1024px) visually identical to today.

## Acceptance Criteria

- [ ] `frontend/src/composables/useMediaQuery.ts` exists and returns a reactive boolean tracking a media query.
- [ ] No horizontal scrolling at 375px / 768px / 1280px on either tab.
- [ ] Board stacks to one column below `lg`; FailureBoard is 1-col on phone, 2-col at `sm`+.
- [ ] Event log is collapsible (collapsed by default) below `lg` and a sticky side panel at `lg`.
- [ ] Packages repo selector is a grouped dropdown below `lg` and the 220px sidebar at `lg`, both driving the same selection.
- [ ] CVE list renders as stacked cards below `sm` and as the table at `sm`+.
- [ ] AppHeader is a compact single row on phone (subtitle hidden, title truncated, icon-only toggle) and unchanged at `sm`+.
- [ ] Desktop (≥1024px) layout is visually unchanged from before Stage 2.
- [ ] `npm run build` passes.
