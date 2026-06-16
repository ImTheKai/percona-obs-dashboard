# Package State Duration Design

**Goal:** Show how long a package has been in its current state inside the package card, for the three transient in-progress states: `scheduled`, `building`, and `finishing`.

**Architecture:** A new `state_changed_at` timestamp is persisted in the `packages` table and updated only when `rollup_state` changes. The frontend reads it and renders a right-aligned relative duration ("for 23m") in the card header row, visible only for the three target states.

**Tech Stack:** Go (SQLite, `database/sql`), Vue 3 Composition API, TypeScript.

**User decisions (already made):**
- Show duration only for `scheduled`, `building`, and `finishing` (not for succeeded, failed, blocked, etc.)
- Placement: right-aligned in row 1 of the card, before the OBS link

---

## Backend

### DB Migration

Add a nullable column to the `packages` table. Nullable so existing rows are unaffected until their state next changes.

```sql
ALTER TABLE packages ADD COLUMN state_changed_at DATETIME;
```

### Model

Add an optional pointer field to `model.Package` (`omitempty` so it is omitted from JSON when NULL):

```go
StateChangedAt *time.Time `json:"state_changed_at,omitempty"`
```

### Store — `UpsertPackageState`

Pass `now` as the candidate `state_changed_at` on every upsert. A `CASE` expression in the `ON CONFLICT DO UPDATE` clause applies it only when `rollup_state` actually changes:

```sql
state_changed_at = CASE
  WHEN excluded.rollup_state != rollup_state THEN excluded.state_changed_at
  ELSE state_changed_at
END
```

First-time inserts always set `state_changed_at = now`. Subsequent writes preserve the existing timestamp unless the state changed.

### Store — `scanPackages`

Read the new nullable column into `StateChangedAt *time.Time` using `sql.NullTime`.

### Scope

No changes to the worker, MQ consumer, API handler, poller, or any other package. `UpsertPackageState` is the single write path for package state, so the logic lives in one place.

---

## Frontend

### Type (`frontend/src/types/api.ts`)

```ts
state_changed_at?: string   // ISO timestamp; absent when NULL
```

### `PackageCard.vue`

Add a `stateAge` computed that returns a formatted string when the rollup state is one of the three target states and `state_changed_at` is present; `null` otherwise:

```ts
const IN_PROGRESS_STATES = new Set(['scheduled', 'building', 'finished'])

const stateAge = computed((): string | null => {
  if (!IN_PROGRESS_STATES.has(props.pkg.rollup_state)) return null
  if (!props.pkg.state_changed_at) return null
  const ms = Date.now() - new Date(props.pkg.state_changed_at).getTime()
  const m = Math.floor(ms / 60000)
  if (m < 1) return 'for <1m'
  if (m < 60) return `for ${m}m`
  return `for ${Math.floor(m / 60)}h ${m % 60}m`
})
```

Render it right-aligned in row 1, between the package name and the OBS link:

```
[ Building ]   postgresql17   for 23m   OBS ↗
```

Styling: `var(--text-muted)`, 10.5px, `var(--font-mono)`, `margin-left: auto` to push it right, `flex-shrink: 0`.

The OBS link loses its `margin-left: auto` (the duration takes that role).

---

## Files Changed

| File | Change |
|------|--------|
| `backend/internal/model/types.go` | Add `StateChangedAt *time.Time` to `Package` |
| `backend/internal/store/packages.go` | Migration, upsert `CASE` expression, scan nullable column |
| `frontend/src/types/api.ts` | Add `state_changed_at?: string` to `Package` |
| `frontend/src/components/PackageCard.vue` | Add `stateAge` computed, render in row 1 |

---

## Acceptance Criteria

- `state_changed_at` is set on first insert and updated only when `rollup_state` changes
- Existing rows with `NULL` `state_changed_at` display no duration (field absent from JSON)
- Duration shown for `scheduled`, `building`, `finishing` rollup states only
- Duration hidden for all other states (succeeded, failed, blocked, etc.)
- Format: `for <1m` / `for 23m` / `for 1h 23m`
- Placement: right-aligned in card header row, before OBS link
- `vue-tsc` type-checks cleanly with the new optional field
