# PPG Build Board — Backend Spec

A specification of the **PPG Build Board** dashboard mockup, written for backend
development. It describes what the dashboard shows, the data it needs, how that
data maps to the Open Build Service (OBS) API at `build.opensuse.org`, and the
exact JSON shapes the frontend consumes.

> The frontend is a static single-page app. It expects a backend that polls OBS,
> normalizes the result, and serves a small set of JSON endpoints. The dashboard
> auto-refreshes every 5 minutes.

---

## 1. Purpose & workflow

The dashboard answers one question every morning: **"Did everything build, and if
not, what broke and why?"** for a single product (e.g. Percona PostgreSQL / "PPG")
across **all of its OBS subprojects at once**.

OBS's own web UI is project-scoped — you can only see one project's packages at a
time. A Percona product is spread across many subprojects (version roots, shared
"common" projects, per-base-image container projects, and immutable release
projects). This dashboard rolls all of them into one view.

The dashboard is **failure-first**: the packages that need attention are surfaced
as cards at the top; everything green collapses into a compact "all clear" strip.
A right-hand **build-events log** shows recent state changes over a configurable
time window so you can see what OBS has been doing overnight.

---

## 2. Subproject model

Percona structures a product as a tree of OBS projects. The dashboard groups
packages by which **tier** of that tree their project belongs to. Naming pattern
(using PPG 17 as the example):

| Tier (scope id) | OBS project pattern | Example | Notes |
|---|---|---|---|
| `common` | `isv:percona:common` | `isv:percona:common` | Packages common to **all** products |
| `ppgcommon` | `isv:percona:<product>:common` | `isv:percona:ppg:common` | Packages common to all versions of the product |
| `version` | `isv:percona:<product>:<version>` | `isv:percona:ppg:17` | The product-version root (most packages live here) |
| `container` | `isv:percona:<product>:<version>:containers:<baseimage>:*` | `isv:percona:ppg:17:containers:oraclelinux-9` | Container images, one subproject per base image |
| `release` | `isv:percona:<product>:<version>:release` | `isv:percona:ppg:17:release` | **Immutable** — frozen, signed release snapshot |

**Discovery:** the set of subprojects under a product is dynamic. The backend
should enumerate them rather than hardcode. OBS exposes the subproject list:

```
GET /source/isv:percona:ppg:17        # lists packages in one project
GET /source?project=isv:percona       # + the project meta / _meta for hierarchy
```

The subprojects page the dashboard is modelled on:
`https://build.opensuse.org/project/subprojects/isv:percona`

The backend should classify each discovered project into one of the five scope
tiers above by matching the naming pattern, then assign every package its tier.

---

## 3. Build status model

### 3.1 Per-target states (from OBS)

Each package builds against a set of **build targets** = (repository × architecture).
OBS reports a code per target. The dashboard recognizes these states:

| State | Meaning | Group |
|---|---|---|
| `succeeded` | Built OK | **success** |
| `failed` | Build ran and failed | needs attention |
| `unresolvable` | Dependencies can't be satisfied | needs attention |
| `broken` | Source/service is broken (e.g. source service failed) | needs attention |
| `blocked` | Waiting on another package to build first | needs attention |
| `building` / `scheduled` / `dispatching` / `finished` | In progress | needs attention (transient) |
| `disabled` | Build switched off for this target | **ignored** |
| `excluded` | Target not applicable | **ignored** |

### 3.2 Grouping rules (IMPORTANT)

This is the core business rule the backend must implement consistently with the UI:

- **Success group** = `succeeded` only. (Per the product owner, `disabled` and
  `excluded` are treated as not-a-problem, but they are **ignored / excluded from
  all counts entirely** — they are neither "built" nor "needs attention".)
- **Needs-attention group** = everything else (`failed`, `unresolvable`, `broken`,
  `blocked`, and any in-progress state).
- The dashboard always shows the **exact** state (color-coded), but rolls up to
  these two groups for counts and sorting.

### 3.3 Package rollup

A package's rollup status = its **worst** target status, by this severity order
(most severe first):

```
broken > failed > unresolvable > blocked > building > succeeded
```

If all non-ignored targets are `succeeded`, the package is "built" (green).
Otherwise it "needs attention" and its rollup label/color is the worst state.

### 3.4 OBS API for build results

```
GET /build/<project>/_result?view=summary
GET /build/<project>/_result?package=<pkg>        # per-package, all repos/arches
GET /build/<project>/<repo>/<arch>/<pkg>/_status   # one target
```

`_result` returns `<status package="..." code="..."/>` per (repo, arch). Map `code`
to the states above. The set of repos/arches comes from the project's `_meta`
`<repository>` entries.

---

## 4. Build targets (repo × arch)

The mockup uses 6 repositories × 2 architectures = 12 targets per package. Real
values come from each project's `_meta`. Reference set used in the mockup:

**Repositories** (dashboard short → OBS repo name):
`deb12 → Debian_12`, `deb11 → Debian_11`, `ub2404 → xUbuntu_24.04`,
`ub2204 → xUbuntu_22.04`, `el9 → RHEL_9`, `el8 → RHEL_8`

**Architectures:** `x86_64`, `aarch64`

Repositories and architectures vary per subproject; the backend must read them
from `_meta` rather than assume this set.

---

## 5. "Triggered by" — the rebuild cause

For every package that needs attention, the dashboard shows **why it rebuilt**
(e.g. "openssl 3.2.1 → 3.2.2", "gcc default 12 → 13", "ubi9 base image digest
changed"). This is the single highest-value field and also the hardest to source.

OBS does **not** directly hand you a clean "caused by X" string. The backend will
need to infer it. Candidate sources, roughly in order of reliability:

1. **`/build/<project>/<repo>/<arch>/<pkg>/_history`** and the build's
   `reason` field (OBS records a rebuild *reason*: `source change`,
   `meta change`, `rebuild counter`, or a triggering package name).
2. **`/build/<project>/<repo>/<arch>/_builddepinfo`** — to find which dependency
   changed version between the last good build and the current one.
3. The **build log** tail (`/build/.../_log`) for the actual compile error, when
   the state is `failed`.
4. The package's source revision (`/source/<project>/<pkg>/_history`) for source
   service / commit triggers.

The backend should produce a `trigger` object (see §7) with a human string
(`what`), a category (`kind`), and a timestamp. If the cause can't be inferred,
return `kind: "unknown"` and a best-effort `what`.

`kind` values used by the mockup: `dependency bump`, `toolchain bump`,
`base image`, `service`, `unknown`.

---

## 6. Build-events log

The right-hand panel is a reverse-chronological feed of **build state-change
events** over a time window. Each event has a type, a "what happened" headline, a
"why" sentence, the scope tier, an optional target, and a timestamp. Events bucket
under **Today / Yesterday / Earlier**.

### 6.1 Event types

| Type | Meaning | Glyph in UI |
|---|---|---|
| `triggered` | A rebuild was scheduled (with cause) | ↻ |
| `started` | A build began | ▸ |
| `succeeded` | A target built OK | ✓ |
| `failed` | A target failed | ✕ |
| `unresolvable` | A target became unresolvable | ? |
| `broken` | Source/service broken | ✕ |
| `blocked` | Target blocked on a dependency | ‖ |
| `published` | Repository/registry publish completed | ↑ |

### 6.2 Time window

The user picks the window. Presets: **1h, 6h, 24h, 3d, 7d**, plus a **Custom**
date range (From / To date pickers, inclusive of whole days, capped at "now").
The backend should accept either a relative window or an explicit `from`/`to`
range and return events within it.

### 6.3 OBS API for events

OBS has no single "events" feed; assemble it from:

- **`/build/<project>/_result`** deltas between polls (state transitions →
  `succeeded`/`failed`/`unresolvable`/`broken`/`blocked` events).
- **Build `_history`** per target (`started`, durations, reason → `triggered`).
- **`/published/<project>`** / publish notifications → `published`.
- Optionally OBS's **event/notification** plugin or the message bus
  (`rabbitmq`) if available, which emits build-result events directly.

The backend persists a rolling event store (it must remember previous poll
results to compute transitions) and serves the windowed slice.

---

## 7. JSON shapes the frontend consumes

These mirror the objects the mockup builds internally. The backend should serve
equivalents. Times can be ISO 8601; the UI renders relative ("3h ago") and buckets.

### 7.1 `GET /api/products/<product>/<version>/packages`

```jsonc
{
  "product": "ppg",
  "version": "17",
  "root": "isv:percona:ppg:17",
  "updatedAt": "2026-06-11T07:42:00Z",
  "repositories": [
    { "short": "deb12", "name": "Debian 12", "obs": "Debian_12" }
    // ...
  ],
  "architectures": ["x86_64", "aarch64"],
  "subprojects": [
    { "scope": "common",    "project": "isv:percona:common" },
    { "scope": "version",   "project": "isv:percona:ppg:17" },
    { "scope": "container", "project": "isv:percona:ppg:17:containers:oraclelinux-9" },
    { "scope": "release",   "project": "isv:percona:ppg:17:release", "immutable": true }
    // ...
  ],
  "packages": [
    {
      "name": "pg_tde",
      "project": "isv:percona:ppg:17",
      "scope": "version",                 // common | ppgcommon | version | container | release
      "rollup": "unresolvable",           // worst non-ignored target state
      "ok": 10, "total": 12,              // succeeded targets / total non-ignored targets
      "showUrl": "https://build.opensuse.org/package/show/isv:percona:ppg:17/pg_tde",
      "trigger": {
        "what": "openssl 3.2.1 → 3.2.2",
        "kind": "dependency bump",        // dependency bump | toolchain bump | base image | service | unknown
        "at": "2026-06-11T04:40:00Z"
      },
      "targets": [
        {
          "repo": "el8", "arch": "aarch64",
          "state": "unresolvable",
          "cause": "openssl-devel ≥ 3.2 unavailable on el8 / aarch64",
          "logUrl": "https://build.opensuse.org/package/live_build_log/isv:percona:ppg:17/pg_tde/RHEL_8/aarch64"
        }
        // ... one per (repo × arch); omit `disabled`/`excluded` targets
      ]
    }
    // ...
  ]
}
```

Notes:
- Only include targets that are **not** `disabled`/`excluded` in `total`, `ok`,
  and the `targets` array.
- `cause` on a target is the per-target failure detail (compile error summary,
  missing dep). Optional; empty for succeeded targets.

### 7.2 `GET /api/products/<product>/<version>/events?window=24h`  (or `?from=…&to=…`)

```jsonc
{
  "now": "2026-06-11T07:42:00Z",
  "events": [
    {
      "id": "evt_001",
      "type": "started",                  // see §6.1
      "scope": "version",
      "project": "isv:percona:ppg:17",
      "package": "pgbackrest",
      "repo": "el9", "arch": "x86_64",    // null for project-level events (triggered/published)
      "what": "pgbackrest build started",
      "why": "Auto-rebuild triggered by libssh2 1.11.0 → 1.11.1 in the build root.",
      "at": "2026-06-11T07:36:00Z",
      "url": "https://build.opensuse.org/package/live_build_log/isv:percona:ppg:17/pgbackrest/RHEL_9/x86_64"
    }
    // ... reverse chronological
  ]
}
```

Notes:
- For events with a target, `url` → live build log; without one (e.g.
  `triggered`, `published`), `url` → package `show` page.
- The UI buckets by Today/Yesterday/Earlier using `now` and each `at`.

---

## 8. Derived numbers (computed in the UI, but backend may precompute)

Given the package list filtered to the active **scope selection** (the user can
multi-select any of the five tiers; empty selection = all):

- `totalPackages` = packages in scope
- `attentionPackages` = packages whose `rollup` ≠ `succeeded`
- `builtPackages` = `totalPackages − attentionPackages`
- `totalTargets` / `builtTargets` = sum of non-ignored targets / succeeded targets
- `subprojects` = distinct `project` values in scope
- **failBreakdown** = count of attention packages by their rollup state, ordered
  `broken, failed, unresolvable, blocked, building`

---

## 9. URL conventions (OBS deep links)

- Package overview: `https://build.opensuse.org/package/show/<project>/<pkg>`
- Live build log:   `https://build.opensuse.org/package/live_build_log/<project>/<pkg>/<repo>/<arch>`
- Subprojects:      `https://build.opensuse.org/project/subprojects/<project>`

`<repo>` is the OBS repository name (e.g. `RHEL_9`), `<arch>` the architecture
(e.g. `x86_64`).

---

## 10. Open questions for the backend

1. **Trigger inference fidelity.** How precise can the "why" be? `_history`
   `reason` gives a category and sometimes a triggering package, but the clean
   "openssl 3.2.1 → 3.2.2" string requires diffing `_builddepinfo` across builds.
   Decide how much inference to do vs. showing the raw reason.
2. **Event source.** Is the OBS message bus / notification plugin available, or
   must events be derived purely from polling `_result` deltas? The latter needs a
   persistent store of prior poll snapshots.
3. **Custom range bounds.** Confirm whether the event store retention covers the
   longest range users will pick (the UI offers up to 7d presets + arbitrary
   custom ranges).
4. **Multi-version.** The UI has a version switcher (17 / 18 / 16). Each maps to a
   separate `isv:percona:ppg:<version>` tree; the API is per-version.
5. **Auth.** `isv:` projects may require an authenticated OBS account; the backend
   holds the credential and the dashboard never talks to OBS directly.

---

*This document describes a design mockup. All package names, versions, failure
causes, and timestamps in the mockup are illustrative sample data.*
