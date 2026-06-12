# GreenStrip Project Grouping Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task.

**Goal:** Group succeeded packages in the GreenStrip by OBS project, with each group header linking to the OBS project page and each package pill linking to the OBS package page.

**Architecture:** Single-component change — `GreenStrip.vue` only. A `computed` property groups the flat `packages` prop by `pkg.project`; the template iterates over groups. No prop changes, no caller changes.

**Tech Stack:** Vue 3 Composition API, TypeScript, inline styles (existing pattern).

**User decisions (already made):**
- Only the succeeded packages (GreenStrip) are grouped — failing package cards are untouched.
- Group headers show the full OBS project path (`isv:percona:ppg:17`), not a trimmed label.
- Package pills link to the OBS package page.
- Approach A chosen: modify `GreenStrip.vue` in place, no new files.

---

## Scope

Only `frontend/src/components/GreenStrip.vue` changes. All other files — `FailureBoard.vue`, `PackageCard.vue`, `MainGrid.vue`, composables, backend — are untouched.

## Data Grouping

A `computed` property groups the incoming `packages: Package[]` by `pkg.project` using a `Map<string, Package[]>` to preserve insertion order. Groups are then sorted alphabetically by project path for a predictable, stable order.

```ts
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
```

## Rendered Structure

The existing summary header (`● All clear · N packages fully built`) is preserved at the top of the card, unchanged.

Below it, each project group renders as:

```
isv:percona:ppg:17 ↗          ← <a> linking to OBS project page
[● pg_repack] [● pgaudit] [● timescaledb]   ← pills, each linking to OBS package page
```

Groups are visually separated from each other by a top border or increased gap.

### Group Header

An `<a>` element with:
- Text: full project path (e.g. `isv:percona:ppg:17`)
- Suffix: ` ↗` (external link indicator)
- `href`: `https://build.opensuse.org/project/show/{project}`
- `target="_blank" rel="noopener"`
- Style: small monospace font (`var(--font-mono)`), muted color (`var(--text-muted)`), no underline by default, subtle hover underline.

### Package Pills

Each pill becomes an `<a>` element (was `<span>`) with:
- `href`: `https://build.opensuse.org/package/show/{project}/{pkg.name}`
- `target="_blank" rel="noopener"`
- Visual: unchanged — green dot (`var(--ok)`) + monospace package name (`var(--text-secondary)`) on `var(--ok-tint)` background, `border-radius: 7px`, `padding: 4px 10px`.
- Subtle hover: slightly darker background or underline on the name to indicate clickability.
- `text-decoration: none` to suppress default anchor underline.

## OBS URL Patterns

Both URL patterns are already established in `PackageCard.vue`:

| Target | URL |
|--------|-----|
| Project page | `https://build.opensuse.org/project/show/{project}` |
| Package page | `https://build.opensuse.org/package/show/{project}/{name}` |

## What Does Not Change

- `GreenStrip` props: still `packages: Package[]` — no interface changes.
- `FailureBoard.vue`: passes `okPackages` to `GreenStrip` exactly as before.
- Failing package cards (`PackageCard`, `FailureBoard` grid): untouched.
- Backend, composables, types: untouched.
