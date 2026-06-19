# Tags Replace Scope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `scope` with `tags []string` throughout the full stack, add `published` as a terminal success state, fix the "all versions" query, and derive the `container` tag from `is_container` in the store layer.

**Architecture:** Tags are a JSON-encoded string slice stored in both `packages` and `events` DB tables. The classifier assigns project-level tags; the store layer splices in `"container"` when `is_container` is true. Worker events inherit full package tags; MQ events use `ProjectTags` (project-level only). The frontend renders three fixed tag pills (`ppg`, `common`, `container`) with AND multi-select semantics.

**Tech Stack:** Go 1.25, modernc/sqlite, Vue 3 + TypeScript, chi router.

**User decisions (already made):**
- Tag filtering is AND: a package/event must carry ALL selected tags to pass.
- `container` tag synthesised by the store layer, not the frontend.
- Known tag pill set: `ppg`, `common`, `container` (fixed order, always shown).
- `scope` column dropped from both `packages` and `events` DB tables.
- `published` is the new terminal success state (better than `succeeded`).

---

### Task 1: DB migration — schema const, migration order, event tags backfill

**Goal:** Update `db.go` so fresh DBs never create a `scope` column, existing DBs migrate event tags from scope before scope is dropped, and the migration order is safe for all DB ages.

**Files:**
- Modify: `backend/internal/store/db.go`
- Modify: `backend/internal/store/db_test.go`

**Acceptance Criteria:**
- [ ] `CREATE TABLE packages` in `schema` const has no `scope` column.
- [ ] `CREATE TABLE events` in `schema` const has `tags TEXT NOT NULL DEFAULT '[]'` instead of `scope TEXT NOT NULL`.
- [ ] `migrateIsContainerNullable` internal `CREATE TABLE packages_new` and `INSERT` have no `scope` column.
- [ ] `columnExists` helper function present.
- [ ] `migrateTagsAndIsRelease` has `if !columnExists(db, "packages", "scope") { return nil }` guard at top.
- [ ] New `migrateEventTags` function with guard and scope→tags mapping.
- [ ] Migration order in `Open()`: events ADD COLUMN tags → `migrateTagsAndIsRelease` → `migrateIsContainerNullable` → `migrateEventTags` → DROP COLUMN scope for both tables.
- [ ] `TestOpen` asserts `packages` has no `scope` column and `events` has `tags` column.
- [ ] `go test ./internal/store/... -count=1` passes.

**Verify:** `cd backend && go test ./internal/store/... -count=1 -v -run TestOpen` → PASS

**Steps:**

- [ ] **Step 1: Update the `schema` const**

In `backend/internal/store/db.go`, replace the `schema` const with:

```go
const schema = `
CREATE TABLE IF NOT EXISTS packages (
    project        TEXT NOT NULL,
    name           TEXT NOT NULL,
    rollup_state   TEXT NOT NULL,
    ok_targets     INTEGER NOT NULL DEFAULT 0,
    total_targets  INTEGER NOT NULL DEFAULT 0,
    trigger_what   TEXT,
    trigger_kind   TEXT,
    trigger_at     DATETIME,
    targets_json    TEXT NOT NULL DEFAULT '[]',
    updated_at      DATETIME NOT NULL,
    state_changed_at DATETIME,
    is_container   INTEGER,
    version        TEXT NOT NULL DEFAULT '',
    container_tags TEXT NOT NULL DEFAULT '[]',
    tags           TEXT NOT NULL DEFAULT '[]',
    is_release     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (project, name)
);

CREATE TABLE IF NOT EXISTS events (
    id       TEXT PRIMARY KEY,
    type     TEXT NOT NULL,
    tags     TEXT NOT NULL DEFAULT '[]',
    project  TEXT NOT NULL,
    package  TEXT NOT NULL,
    repo     TEXT,
    arch     TEXT,
    what     TEXT NOT NULL,
    why      TEXT NOT NULL,
    url      TEXT NOT NULL,
    at       DATETIME NOT NULL,
    version  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS events_at ON events(at);

CREATE INDEX IF NOT EXISTS idx_packages_rollup_state ON packages(rollup_state);

CREATE TABLE IF NOT EXISTS target_state_durations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project     TEXT     NOT NULL,
    package     TEXT     NOT NULL,
    repo        TEXT     NOT NULL,
    arch        TEXT     NOT NULL,
    state       TEXT     NOT NULL,
    entered_at  DATETIME NOT NULL,
    exited_at   DATETIME,
    duration_ms INTEGER
);

CREATE INDEX IF NOT EXISTS idx_tsd_pkg ON target_state_durations (project, package);
`
```

- [ ] **Step 2: Add `columnExists` helper**

Add this function before `Open`:

```go
func columnExists(db *sql.DB, table, col string) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, col).Scan(&n)
	return n > 0
}
```

- [ ] **Step 3: Add guard to `migrateTagsAndIsRelease`**

At the very top of `migrateTagsAndIsRelease`, before the first `db.Exec`:

```go
func migrateTagsAndIsRelease(db *sql.DB) error {
	if !columnExists(db, "packages", "scope") {
		return nil
	}
	// ... existing body unchanged
```

- [ ] **Step 4: Add `migrateEventTags` function**

Add this new function after `migrateTagsAndIsRelease`:

```go
func migrateEventTags(db *sql.DB) error {
	if !columnExists(db, "events", "scope") {
		return nil
	}
	_, err := db.Exec(`
		UPDATE events SET tags = CASE
			WHEN scope = 'version'                              THEN '["ppg"]'
			WHEN scope = 'pr'                                   THEN '["ppg","pr"]'
			WHEN scope = 'ppgcommon'                            THEN '["ppg","common"]'
			WHEN scope = 'common'                               THEN '["common"]'
			WHEN scope = 'release'                              THEN '["ppg","release"]'
			WHEN scope = 'container' AND project LIKE '%:PR:%' THEN '["ppg","pr"]'
			WHEN scope = 'container'                            THEN '["ppg"]'
			ELSE '[]'
		END
		WHERE tags = '[]'
	`)
	return err
}
```

- [ ] **Step 5: Update `migrateIsContainerNullable` to remove scope**

In the `stmts` slice inside `migrateIsContainerNullable`, replace the `CREATE TABLE packages_new` statement:

```go
`CREATE TABLE packages_new (
    project          TEXT NOT NULL,
    name             TEXT NOT NULL,
    rollup_state     TEXT NOT NULL,
    ok_targets       INTEGER NOT NULL DEFAULT 0,
    total_targets    INTEGER NOT NULL DEFAULT 0,
    trigger_what     TEXT,
    trigger_kind     TEXT,
    trigger_at       DATETIME,
    targets_json     TEXT NOT NULL DEFAULT '[]',
    updated_at       DATETIME NOT NULL,
    state_changed_at DATETIME,
    is_container     INTEGER,
    version          TEXT NOT NULL DEFAULT '',
    container_tags   TEXT NOT NULL DEFAULT '[]',
    tags             TEXT NOT NULL DEFAULT '[]',
    is_release       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (project, name)
)`,
```

And replace the `INSERT INTO packages_new SELECT …` statement (remove `scope` from both the column list and the SELECT):

```go
`INSERT INTO packages_new
    SELECT project, name, rollup_state, ok_targets, total_targets,
           trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
           state_changed_at,
           CASE WHEN is_container = 1 THEN 1 ELSE NULL END,
           version,
           container_tags,
           tags,
           is_release
    FROM packages`,
```

- [ ] **Step 6: Reorder migration calls in `Open()`**

Replace the migration section in `Open()` with the correct order:

```go
// Additive migrations (fail silently if column already exists).
db.Exec(`ALTER TABLE packages ADD COLUMN state_changed_at DATETIME`)
db.Exec(`ALTER TABLE packages ADD COLUMN is_container INTEGER`)
db.Exec(`ALTER TABLE packages ADD COLUMN version TEXT NOT NULL DEFAULT ''`)
db.Exec(`ALTER TABLE events ADD COLUMN version TEXT NOT NULL DEFAULT ''`)
db.Exec(`ALTER TABLE packages ADD COLUMN container_tags TEXT NOT NULL DEFAULT '[]'`)
db.Exec(`ALTER TABLE packages ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`)
db.Exec(`ALTER TABLE packages ADD COLUMN is_release INTEGER NOT NULL DEFAULT 0`)
// events.tags: ensure column exists before backfill, before scope drop.
db.Exec(`ALTER TABLE events ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`)

// Data migration: backfill package tags/is_release from scope BEFORE structural rebuild.
if err := migrateTagsAndIsRelease(db); err != nil {
    db.Close()
    return nil, fmt.Errorf("migrate tags and is_release: %w", err)
}

// Structural migration: make is_container nullable.
var isContainerNotNull int
if err := db.QueryRow(
    `SELECT "notnull" FROM pragma_table_info('packages') WHERE name = 'is_container'`,
).Scan(&isContainerNotNull); err == nil && isContainerNotNull == 1 {
    if err := migrateIsContainerNullable(db); err != nil {
        db.Close()
        return nil, fmt.Errorf("migrate is_container nullable: %w", err)
    }
}

// Data migration: backfill event tags from scope BEFORE scope column is dropped.
if err := migrateEventTags(db); err != nil {
    db.Close()
    return nil, fmt.Errorf("migrate event tags: %w", err)
}

// Drop legacy scope columns (idempotent: fails silently if already gone).
db.Exec(`ALTER TABLE packages DROP COLUMN scope`)
db.Exec(`ALTER TABLE events DROP COLUMN scope`)

// Data migration: promote fully-published succeeded packages.
if err := migrateSucceededToPublished(db); err != nil {
    db.Close()
    return nil, fmt.Errorf("migrate succeeded to published: %w", err)
}
```

- [ ] **Step 7: Update `TestOpen` in `db_test.go`**

Add assertions that verify the new schema — no `scope` on packages, `tags` on events:

```go
func TestOpen(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"packages", "events"} {
		var name string
		if err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name); err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	var idx string
	if err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='events_at'",
	).Scan(&idx); err != nil {
		t.Errorf("events_at index not found: %v", err)
	}

	// packages must not have a scope column
	if columnExists(db, "packages", "scope") {
		t.Error("packages.scope should not exist on fresh DB")
	}
	// events must have tags, not scope
	if !columnExists(db, "events", "tags") {
		t.Error("events.tags missing on fresh DB")
	}
	if columnExists(db, "events", "scope") {
		t.Error("events.scope should not exist on fresh DB")
	}
}
```

- [ ] **Step 8: Run tests and commit**

```bash
cd backend && go test ./internal/store/... -count=1 -v
```

Expected: all store tests pass.

```bash
git add backend/internal/store/db.go backend/internal/store/db_test.go
git commit -s -m "feat(store): migrate events.tags from scope, reorder migration sequence"
```

---

### Task 2: Backend model + store layer — remove Scope, add container tag splice, fix QueryBuildPackages

**Goal:** Remove `Scope` from `model.Package` and `model.Event`, update store read/write paths to use `Tags`, splice `container` tag from `is_container`, and fix `QueryBuildPackages` for all-versions mode.

**Files:**
- Modify: `backend/internal/model/types.go`
- Modify: `backend/internal/store/packages.go`
- Modify: `backend/internal/store/events.go`
- Modify: `backend/internal/store/packages_test.go`
- Modify: `backend/internal/store/events_test.go`

**Acceptance Criteria:**
- [ ] `model.Scope` type and all `Scope*` constants removed from `types.go`.
- [ ] `Package.Scope` and `Event.Scope` fields removed.
- [ ] `UpsertPackageState` fetches existing `is_container` alongside `prevTargetsJSON`, splices `"container"` into tags when either field indicates container.
- [ ] `UpsertPackageState` INSERT/UPDATE column list has no `scope`.
- [ ] `scanPackages` has no `scope` scan destination.
- [ ] `QueryBuildPackages` uses `is_release = 0` at the top level wrapping all arms; handles `"_"` sentinel for all-versions mode.
- [ ] `AppendEvent` writes `tags` JSON, not `scope`.
- [ ] `QueryEvents` scans `tags` JSON into `e.Tags`.
- [ ] `go test ./internal/model/... ./internal/store/... -count=1` passes.

**Verify:** `cd backend && go test ./internal/store/... -count=1` → all PASS

**Steps:**

- [ ] **Step 1: Remove Scope from `model/types.go`**

Replace the `Scope` type block and remove `Scope` from both structs:

```go
// Remove entirely:
type Scope string

const (
    ScopeCommon    Scope = "common"
    ScopePPGCommon Scope = "ppgcommon"
    ScopeVersion   Scope = "version"
    ScopeContainer Scope = "container"
    ScopeRelease   Scope = "release"
    ScopePR        Scope = "pr"
)
```

In `Package` struct, remove the line:
```go
Scope          Scope       `json:"scope"`
```

In `Event` struct, remove the line:
```go
Scope   Scope     `json:"scope"`
```
And add `Tags []string` to `Event`:
```go
type Event struct {
    ID      string    `json:"id"`
    Type    EventType `json:"type"`
    Tags    []string  `json:"tags,omitempty"`
    Project string    `json:"project"`
    // ... rest unchanged
}
```

- [ ] **Step 2: Update `UpsertPackageState` in `store/packages.go`**

Replace the pre-read at the top of the function:

```go
var prevTargetsJSON string
var prevIsContainer sql.NullInt64
var prevTags string
if err := db.QueryRow(
    `SELECT targets_json, is_container, tags FROM packages WHERE project = ? AND name = ?`,
    p.Project, p.Name,
).Scan(&prevTargetsJSON, &prevIsContainer, &prevTags); err == nil {
    _ = json.Unmarshal([]byte(prevTargetsJSON), &prevTargets)
}
```

Before marshalling `p.Tags`, splice in `"container"` when the package is a container (current or previously known):

```go
// Merge container tag when is_container is true now or was true previously.
isContainer := (p.IsContainer != nil && *p.IsContainer) ||
    (prevIsContainer.Valid && prevIsContainer.Int64 != 0)
mergedTags := p.Tags
if isContainer {
    seen := make(map[string]bool, len(p.Tags)+1)
    for _, t := range p.Tags {
        seen[t] = true
    }
    if !seen["container"] {
        mergedTags = append(append([]string(nil), p.Tags...), "container")
    }
}
tagsJSON, err := json.Marshal(mergedTags)
if err != nil {
    return err
}
if tagsJSON == nil || string(tagsJSON) == "null" {
    tagsJSON = []byte("[]")
}
```

Remove `scope` from the INSERT column list and ON CONFLICT clause. The full INSERT becomes:

```go
_, err = db.Exec(`
    INSERT INTO packages
        (project, name, rollup_state, ok_targets, total_targets,
         trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
         state_changed_at, is_container, version, container_tags, tags, is_release)
    VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
    ON CONFLICT(project, name) DO UPDATE SET
        rollup_state=excluded.rollup_state,
        ok_targets=excluded.ok_targets,
        total_targets=excluded.total_targets,
        trigger_what=excluded.trigger_what,
        trigger_kind=excluded.trigger_kind,
        trigger_at=excluded.trigger_at,
        targets_json=excluded.targets_json,
        updated_at=excluded.updated_at,
        is_container=CASE WHEN excluded.is_container IS NOT NULL
                           THEN excluded.is_container ELSE is_container END,
        version=excluded.version,
        container_tags=excluded.container_tags,
        tags=CASE WHEN excluded.tags != '[]' THEN excluded.tags ELSE tags END,
        is_release=CASE WHEN excluded.is_release != 0 THEN 1 ELSE is_release END,
        state_changed_at = CASE
            WHEN excluded.rollup_state != rollup_state THEN excluded.state_changed_at
            WHEN state_changed_at IS NULL              THEN excluded.state_changed_at
            ELSE state_changed_at
        END`,
    p.Project, p.Name, string(p.RollupState),
    p.OKTargets, p.TotalTargets,
    trigWhat, trigKind, trigAt,
    string(targetsJSON), p.UpdatedAt, now,
    isContainerVal, p.Version, string(containerTagsJSON),
    string(tagsJSON), isReleaseVal,
)
```

- [ ] **Step 3: Remove `scope` from `scanPackages`**

In `scanPackages`, remove `&p.Scope` from the `rows.Scan(...)` call and remove `scope` from `packageSelectCols`:

```go
const packageSelectCols = ` project, name, rollup_state, ok_targets, total_targets,
    trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
    state_changed_at, is_container, version, container_tags, tags, is_release`
```

In the `rows.Scan(...)` call, remove `&p.Scope`:

```go
if err := rows.Scan(
    &p.Project, &p.Name, &p.RollupState,
    &p.OKTargets, &p.TotalTargets,
    &trigWhat, &trigKind, &trigAt,
    &targetsJSON, &p.UpdatedAt,
    &stateChangedAt, &isContainerNull, &p.Version,
    &containerTagsJSON, &tagsJSON, &isRelease,
); err != nil {
    return nil, err
}
```

- [ ] **Step 4: Fix `QueryBuildPackages`**

Replace the entire function body with the new version that handles `"_"` all-versions sentinel and wraps `is_release = 0` at the top level:

```go
func QueryBuildPackages(db *sql.DB, root, product, version string) ([]*model.Package, error) {
    cp := root + ":" + product + ":common"
    gp := root + ":common"
    var rows *sql.Rows
    var err error
    if version == "_" || version == "" {
        pp := root + ":" + product
        rows, err = db.Query(`SELECT`+packageSelectCols+`
            FROM packages
            WHERE is_release = 0
              AND (  (project = ? OR project LIKE ? || ':%')
                  OR (project = ? OR project LIKE ? || ':%') )
            ORDER BY project, name`,
            pp, pp, gp, gp,
        )
    } else {
        vp := root + ":" + product + ":" + version
        rows, err = db.Query(`SELECT`+packageSelectCols+`
            FROM packages
            WHERE is_release = 0
              AND (  (project = ? OR project LIKE ? || ':%')
                  OR (project = ? OR project LIKE ? || ':%')
                  OR (project = ? OR project LIKE ? || ':%') )
            ORDER BY project, name`,
            vp, vp, cp, cp, gp, gp,
        )
    }
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    return scanPackages(rows)
}
```

- [ ] **Step 5: Update `AppendEvent` and `QueryEvents` in `store/events.go`**

In `AppendEvent`, replace `scope` with `tags`:

```go
func AppendEvent(db *sql.DB, e *model.Event) error {
    tagsJSON, err := json.Marshal(e.Tags)
    if err != nil {
        return err
    }
    if tagsJSON == nil || string(tagsJSON) == "null" {
        tagsJSON = []byte("[]")
    }
    _, err = db.Exec(`
        INSERT INTO events (id, type, tags, project, package, repo, arch, what, why, url, at, version)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
        e.ID, string(e.Type), string(tagsJSON),
        e.Project, e.Package, nullStr(e.Repo), nullStr(e.Arch),
        e.What, e.Why, e.URL, e.At, e.Version,
    )
    return err
}
```

Add `"encoding/json"` to imports if not already present.

In `QueryEvents`, replace `scope` with `tags` in the SELECT and scan:

```go
func QueryEvents(db *sql.DB, projectPrefix string, from, to time.Time) ([]*model.Event, error) {
    rows, err := db.Query(`
        SELECT id, type, tags, project, package,
               COALESCE(repo,''), COALESCE(arch,''),
               what, why, url, at, COALESCE(version,'')
        FROM events
        WHERE project LIKE ? AND at >= ? AND at <= ?
        ORDER BY at DESC`,
        projectPrefix+"%", from, to,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    events := make([]*model.Event, 0)
    for rows.Next() {
        e := &model.Event{}
        var tagsJSON string
        if err := rows.Scan(
            &e.ID, &e.Type, &tagsJSON, &e.Project, &e.Package,
            &e.Repo, &e.Arch, &e.What, &e.Why, &e.URL, &e.At, &e.Version,
        ); err != nil {
            return nil, err
        }
        if tagsJSON != "" && tagsJSON != "[]" {
            _ = json.Unmarshal([]byte(tagsJSON), &e.Tags)
        }
        events = append(events, e)
    }
    return events, rows.Err()
}
```

- [ ] **Step 6: Update `packages_test.go` — replace all `Scope:` fields**

Remove `Scope: model.ScopeXxx` from every `model.Package{}` literal. Since `Scope` no longer exists, the compiler will flag each one. Replace them:
- `Scope: model.ScopeVersion` → remove (tags default to `[]`)
- `Scope: model.ScopeCommon` → remove
- `Scope: model.ScopeContainer` → remove
- `Scope: model.ScopeRelease` → remove (use `IsRelease: true, Tags: []string{"ppg","release"}` if the test checks release filtering)

Also fix raw SQL INSERT statements in tests that include `scope` in the column list — remove `scope` from both the column list and the values.

Example: change
```go
db.Exec(`INSERT INTO packages (project, name, scope, rollup_state, ok_targets, total_targets, targets_json, updated_at)
    VALUES (?,?,?,?,?,?,?,?)`, "isv:percona:ppg:17", "pkg", "version", "succeeded", 1, 1, "[]", now)
```
to:
```go
db.Exec(`INSERT INTO packages (project, name, rollup_state, ok_targets, total_targets, targets_json, updated_at)
    VALUES (?,?,?,?,?,?,?)`, "isv:percona:ppg:17", "pkg", "succeeded", 1, 1, "[]", now)
```

Add a test for container tag auto-injection:

```go
func TestUpsertContainerTagInjection(t *testing.T) {
    db, err := Open(":memory:")
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    trueVal := true
    now := time.Now().UTC().Truncate(time.Second)
    p := &model.Package{
        Project:     "isv:percona:ppg:17:containers:ubi9",
        Name:        "percona-postgresql17",
        Tags:        []string{"ppg"},
        IsContainer: &trueVal,
        RollupState: model.RollupSucceeded,
        Targets:     []model.Target{},
        UpdatedAt:   now,
    }
    if err := UpsertPackageState(db, p, now); err != nil {
        t.Fatal(err)
    }

    pkgs, err := QueryPackages(db, "isv:percona:ppg:17:containers:ubi9")
    if err != nil || len(pkgs) != 1 {
        t.Fatalf("expected 1 package, got %d", len(pkgs))
    }
    tags := pkgs[0].Tags
    found := false
    for _, tg := range tags {
        if tg == "container" {
            found = true
        }
    }
    if !found {
        t.Errorf("expected container tag in %v", tags)
    }
}
```

- [ ] **Step 7: Update `events_test.go` — replace `Scope:` field**

In `TestAppendQueryPruneEvents`, replace `Scope: model.ScopeVersion` with `Tags: []string{"ppg"}` and add an assertion that the queried event has the correct tags:

```go
e := &model.Event{
    ID:      "evt_01",
    Type:    model.EventFailed,
    Tags:    []string{"ppg"},
    Project: "isv:percona:ppg:17",
    Package: "pg_tde",
    What:    "build failed",
    Why:     "openssl bump",
    URL:     "https://build.opensuse.org/package/show/isv:percona:ppg:17/pg_tde",
    At:      now,
}
// ... after QueryEvents:
if len(events[0].Tags) != 1 || events[0].Tags[0] != "ppg" {
    t.Errorf("expected tags [ppg], got %v", events[0].Tags)
}
```

In `TestEventVersionRoundtrip`, same change: `Scope: model.ScopeVersion` → `Tags: []string{"ppg"}`.

- [ ] **Step 8: Run tests and commit**

```bash
cd backend && go test ./internal/store/... -count=1 -v
```

Expected: all store tests PASS (the build may still fail for obs/worker/mq — that's expected; fix those in later tasks).

```bash
git add backend/internal/model/types.go \
        backend/internal/store/packages.go \
        backend/internal/store/events.go \
        backend/internal/store/packages_test.go \
        backend/internal/store/events_test.go
git commit -s -m "feat(model,store): replace scope with tags, add container tag splice, fix QueryBuildPackages"
```

---

### Task 3: Backend obs package — poller, tasks, classifier

**Goal:** Remove `InferScope`/`EventScope()`, update `buildPackage` to accept `tags`, fix all call sites in `poller.go` and `tasks.go`, replace `TestInferScope` with `TestProjectTags`.

**Files:**
- Modify: `backend/internal/obs/poller.go`
- Modify: `backend/internal/obs/tasks.go`
- Modify: `backend/internal/obs/classifier.go`
- Modify: `backend/internal/obs/poller_test.go`
- Modify: `backend/internal/obs/tasks_test.go`

**Acceptance Criteria:**
- [ ] `InferScope` function removed from `poller.go`.
- [ ] `EventScope()` method removed from `classifier.go`.
- [ ] `buildPackage` signature is `buildPackage(project, name string, tags []string, targets []PackageBuildState) *model.Package`.
- [ ] Poller call site: `tags := ProjectTags(p.root, project)` passed to `buildPackage`.
- [ ] `tasks.go` call site: `buildPackage(pkg.Project, pkg.Name, pkg.Tags, results)`.
- [ ] `stateChangeEvent` sets `Tags: pkg.Tags` (no `Scope`).
- [ ] `TestInferScope` replaced by `TestProjectTags`.
- [ ] All `Scope:` fields removed from `tasks_test.go` package literals.
- [ ] `go build ./internal/obs/...` passes.
- [ ] `go test ./internal/obs/... -count=1` passes.

**Verify:** `cd backend && go test ./internal/obs/... -count=1` → all PASS

**Steps:**

- [ ] **Step 1: Remove `EventScope()` from `classifier.go`**

Delete the `EventScope()` method entirely:

```go
// DELETE this method:
func (k ProjectKind) EventScope() model.Scope {
    ...
}
```

- [ ] **Step 2: Update `buildPackage` signature in `poller.go`**

Change the function signature and body:

```go
func buildPackage(project, name string, tags []string, targets []PackageBuildState) *model.Package {
```

Inside, replace `Scope: scope` with `Tags: tags`:

```go
return &model.Package{
    Project:      project,
    Name:         name,
    Tags:         tags,
    RollupState:  rollup,
    // ... rest unchanged
}
```

- [ ] **Step 3: Update `InferScope` call site in `poller.go` and remove `InferScope`**

In `tick`/`discoverProjects`, replace:

```go
scope := InferScope(project) // kept for Package.Scope backward compat
// ...
pkg := buildPackage(project, pkgName, scope, targets)
```

with:

```go
tags := ProjectTags(p.root, project)
// ...
pkg := buildPackage(project, pkgName, tags, targets)
```

Then delete the entire `InferScope` function (lines ~182–209 in the original file).

- [ ] **Step 4: Update `stateChangeEvent` in `poller.go`**

In the `stateChangeEvent` function, replace `Scope: pkg.Scope` with `Tags: pkg.Tags`:

```go
return &model.Event{
    ID:      "evt_" + ulid.Make().String(),
    Type:    evtType,
    Tags:    pkg.Tags,
    Project: pkg.Project,
    Package: pkg.Name,
    What:    what,
    Why:     "",
    URL:     fmt.Sprintf("%s/package/show/%s/%s", obsBase, pkg.Project, pkg.Name),
    At:      now,
}
```

- [ ] **Step 5: Fix `tasks.go` call site**

In `BuildStateTask.Run` (line ~19), change:

```go
updated := buildPackage(pkg.Project, pkg.Name, pkg.Tags, results)
```

- [ ] **Step 6: Update `poller_test.go` — replace `TestInferScope` with `TestProjectTags`**

Delete `TestInferScope` and add:

```go
func TestProjectTags(t *testing.T) {
    cases := []struct {
        project string
        want    []string
    }{
        {"isv:percona:ppg:17", []string{"ppg"}},
        {"isv:percona:ppg:17:containers:ubi9", []string{"ppg"}},
        {"isv:percona:ppg:releases:17", []string{"ppg", "release"}},
        {"isv:percona:PR:pr-42:ppg:17", []string{"ppg", "pr"}},
        {"isv:percona:ppg:common", []string{"ppg", "common"}},
        {"isv:percona:ppgcommon", []string{"ppg", "common"}},
        {"isv:percona:common", []string{"common"}},
    }
    for _, c := range cases {
        got := ProjectTags(root, c.project)
        if len(got) != len(c.want) {
            t.Errorf("ProjectTags(%q): got %v, want %v", c.project, got, c.want)
            continue
        }
        for i := range c.want {
            if got[i] != c.want[i] {
                t.Errorf("ProjectTags(%q)[%d]: got %q, want %q", c.project, i, got[i], c.want[i])
            }
        }
    }
}
```

- [ ] **Step 7: Fix `tasks_test.go` — remove all `Scope:` fields**

Every `model.Package{}` in `tasks_test.go` that has `Scope: model.ScopeXxx` — remove that line. Replace with `Tags:` if the test logic depends on the classification:
- `Scope: model.ScopeCommon` → remove (or `Tags: []string{"common"}` if tested)
- `Scope: model.ScopeVersion` → remove (or `Tags: []string{"ppg"}`)
- `Scope: model.ScopeContainer` → remove + add `IsContainer: &trueVal` if the test is about container detection

- [ ] **Step 8: Run tests and commit**

```bash
cd backend && go build ./internal/obs/... && go test ./internal/obs/... -count=1 -v
```

Expected: all PASS.

```bash
git add backend/internal/obs/poller.go \
        backend/internal/obs/tasks.go \
        backend/internal/obs/classifier.go \
        backend/internal/obs/poller_test.go \
        backend/internal/obs/tasks_test.go
git commit -s -m "feat(obs): replace InferScope/EventScope with ProjectTags, remove scope from poller events"
```

---

### Task 4: Backend worker + MQ consumer

**Goal:** Remove all `Scope`/`pkg.Scope` references from `worker.go` and `consumer.go`; use `Tags` throughout. Full `go build ./...` and `go test ./...` must pass after this task.

**Files:**
- Modify: `backend/internal/worker/worker.go`
- Modify: `backend/internal/worker/worker_test.go`
- Modify: `backend/internal/mq/consumer.go`

**Acceptance Criteria:**
- [ ] All four event structs in `emitBuildEvents` use `Tags: pkg.Tags` (not `Scope`).
- [ ] `consumer.go` has no `scope := kind.EventScope()` call; events use `Tags: obs.ProjectTags(c.root, m.Project)`.
- [ ] `mergePackageTarget` has no `scope model.Scope` parameter; sets `Tags: obs.ProjectTags(c.root, m.Project)`.
- [ ] `worker_test.go` has no `Scope:` fields on package literals.
- [ ] `cd backend && go build ./...` exits 0.
- [ ] `cd backend && go test ./... -count=1` all PASS.

**Verify:** `cd backend && go build ./... && go test ./... -count=1` → all PASS

**Steps:**

- [ ] **Step 1: Update `emitBuildEvents` in `worker/worker.go`**

In all four event creation blocks in `emitBuildEvents`, replace `Scope: pkg.Scope` with `Tags: pkg.Tags`:

For `EventBuildStarted`:
```go
p.appendEvent(&model.Event{
    ID:      "evt_" + ulid.Make().String(),
    Type:    model.EventBuildStarted,
    Tags:    pkg.Tags,
    Project: pkg.Project,
    Package: pkg.Name,
    Repo:    t.Repo,
    Arch:    t.Arch,
    What:    fmt.Sprintf("%s build started", pkg.Name),
    Why:     why,
    URL:     fmt.Sprintf("%s/package/live_build_log/%s/%s/%s/%s", obsBase, pkg.Project, pkg.Name, t.Repo, t.Arch),
    At:      now,
})
```

For `EventFailed`, `EventSucceeded`, `EventPublished`: same pattern — `Tags: pkg.Tags` instead of `Scope: pkg.Scope`.

- [ ] **Step 2: Update `consumer.go`**

Remove `scope := kind.EventScope()` and replace all `Scope: scope` in event literals with `Tags: obs.ProjectTags(c.root, m.Project)`.

Update `mergePackageTarget` signature from:
```go
func (c *Consumer) mergePackageTarget(m mqMessage, scope model.Scope, newState model.RollupState) *model.Package {
```
to:
```go
func (c *Consumer) mergePackageTarget(m mqMessage, newState model.RollupState) *model.Package {
```

Inside `mergePackageTarget`, replace `Scope: scope` with `Tags: obs.ProjectTags(c.root, m.Project)`.

Update all call sites of `mergePackageTarget` — remove the `scope` argument:
```go
pkg := c.mergePackageTarget(m, rollup)
```

- [ ] **Step 3: Fix `worker_test.go` — remove all `Scope:` fields**

Every `model.Package{}` literal that has `Scope: model.ScopeXxx` — remove that line. The field no longer exists.

- [ ] **Step 4: Build and test everything**

```bash
cd backend && go build ./...
```

Expected: exits 0 (no compile errors anywhere).

```bash
cd backend && go test ./... -count=1
```

Expected: all packages PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/worker/worker.go \
        backend/internal/worker/worker_test.go \
        backend/internal/mq/consumer.go
git commit -s -m "feat(worker,mq): replace Scope with Tags in events"
```

---

### Task 5: Frontend composables — types, filtering, display helpers

**Goal:** Update TypeScript types and all composables to use `tags` and `is_release` instead of `scope`; add `published` to `BuildState`; replace scope display maps with tag maps.

**Files:**
- Modify: `frontend/src/types/api.ts`
- Modify: `frontend/src/composables/usePackages.ts`
- Modify: `frontend/src/composables/useEvents.ts`
- Modify: `frontend/src/composables/useArtifacts.ts`
- Modify: `frontend/src/composables/useEventDisplay.ts`

**Acceptance Criteria:**
- [ ] `PackageScope` type removed; `Package.scope` removed; `Package.tags?: string[]` and `Package.is_release?: boolean` added.
- [ ] `Event.scope` removed; `Event.tags?: string[]` added.
- [ ] `BuildState` includes `'published'`.
- [ ] `SEVERITY` in `usePackages` has `published: -1`.
- [ ] `sorted` filter uses `!pkg.is_release` not `pkg.scope !== 'release'`.
- [ ] `filterByScope` removed; `filterByTags(tags: string[])` added with AND semantics.
- [ ] `filterEvents` uses `tags: string[]` parameter and AND logic.
- [ ] `PackageRow.scope` → `tags: string[]` in `useArtifacts`; container filter uses `is_container`.
- [ ] `SCOPE_STYLE`/`SCOPE_LABEL` replaced by `TAG_STYLE`/`TAG_LABEL` in `useEventDisplay`.
- [ ] `cd frontend && npm run build` exits 0.

**Verify:** `cd frontend && npm run build` → exits 0

**Steps:**

- [ ] **Step 1: Update `frontend/src/types/api.ts`**

```typescript
export type BuildState = 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'scheduled' | 'building' | 'finished' | 'published'
export type EventType = 'triggered' | 'started' | 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'published' | 'created' | 'deleted' | 'build_started' | 'build_finished' | 'version_change' | 'updated'

export interface Context {
  label: string
  apiBase: string
  prefix: string
}

export interface Trigger {
  what: string
  kind: string
  at: string
}

export interface Target {
  repo: string
  arch: string
  state: BuildState
  details?: string
  blocked_by?: string
  build_reason?: string
  build_reason_packages?: string[]
  published?: boolean
}

export interface Package {
  project: string
  name: string
  tags?: string[]
  is_release?: boolean
  rollup_state: BuildState
  ok_targets: number
  total_targets: number
  is_container?: boolean
  version?: string
  trigger?: Trigger
  targets: Target[]
  updated_at: string
  state_changed_at?: string
  container_tags?: string[]
}

export interface PRGroup {
  pr: string
  rollup_state: BuildState
  packages: Package[]
}

export interface Event {
  id: string
  type: EventType
  tags?: string[]
  project: string
  package: string
  repo?: string
  arch?: string
  what: string
  why: string
  version?: string
  url: string
  at: string
}
```

- [ ] **Step 2: Update `usePackages.ts`**

Replace `SEVERITY` to add `published`:

```typescript
const SEVERITY: Record<string, number> = {
  broken: 5,
  failed: 4,
  unresolvable: 3,
  blocked: 2,
  building: 1,
  finished: 1,
  scheduled: 1,
  succeeded: 0,
  published: -1,
}
```

In `sorted` computed, replace the filter:

```typescript
.filter(pkg => !pkg.is_release && !pkg.project.toLowerCase().includes(':releases:') && matchesVersion(pkg, ver, depth, knownVersions))
```

Wait — with `is_release` now on the type, and the backend filtering out releases from `QueryBuildPackages`, we can simplify to just:

```typescript
.filter(pkg => !pkg.is_release && matchesVersion(pkg, ver, depth, knownVersions))
```

Replace `filterByScope` with `filterByTags`:

```typescript
function filterByTags(tags: string[]): Package[] {
  if (tags.length === 0) return sorted.value
  return sorted.value.filter(p =>
    tags.every(t => (p.tags ?? []).includes(t))
  )
}

return { data: sorted, rawData: data, availableVersions, loading, error, refresh, filterByTags }
```

- [ ] **Step 3: Update `useEvents.ts`**

Rename `scopes` parameter to `tags` and update the filter logic:

```typescript
function filterEvents(tags: string[], version: string, prefixDepth: number, prefix: string): Event[] {
  return data.value.filter(e => {
    if (prefix && e.project !== prefix && !e.project.startsWith(prefix + ':')) return false
    if (tags.length > 0 && !tags.every(t => (e.tags ?? []).includes(t))) return false
    return matchesEventVersion(e, version, prefixDepth)
  })
}
```

- [ ] **Step 4: Update `useArtifacts.ts`**

In `PackageRow` interface, replace `scope`:

```typescript
export interface PackageRow {
  project: string
  name: string
  version: string
  tags: string[]
  state: string
  published: boolean
  repo: RepoInfo
  arch: string
}
```

In `packageRows` computed, replace `scope: pkg.scope as ...` with `tags: pkg.tags ?? []`.

In `containerImages` computed, replace `pkg.scope === 'container'` with `pkg.is_container === true`:

```typescript
return pkgs
  .filter(pkg =>
    pkg.is_container === true &&
    pkg.project.startsWith(`${prefix}:${ver}:`)
  )
```

- [ ] **Step 5: Update `useEventDisplay.ts`**

Replace `SCOPE_STYLE` and `SCOPE_LABEL` with `TAG_STYLE` and `TAG_LABEL`. Remove `displayVersion`'s `isContainer: boolean` parameter — now derive it from tags in the caller. Actually keep `displayVersion(version, isContainer)` signature unchanged so callers can pass `tags.includes('container')`:

```typescript
export const TAG_STYLE: Record<string, string> = {
  ppg:       'background: var(--brand-purple-tint); color: var(--brand-purple);',
  common:    'background: var(--blocked-tint); color: var(--blocked);',
  container: 'background: var(--info-tint); color: var(--info);',
  pr:        'background: var(--warn-tint); color: var(--warn);',
  release:   'background: var(--ok-tint); color: var(--ok);',
}

export const TAG_LABEL: Record<string, string> = {
  ppg: 'PPG', common: 'Common', container: 'Container', pr: 'PR', release: 'Release',
}
```

Remove `SCOPE_STYLE` and `SCOPE_LABEL` entirely.

- [ ] **Step 6: Build frontend**

```bash
cd frontend && npm run build
```

Expected: exits 0. TypeScript errors will guide any missed references.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/types/api.ts \
        frontend/src/composables/usePackages.ts \
        frontend/src/composables/useEvents.ts \
        frontend/src/composables/useArtifacts.ts \
        frontend/src/composables/useEventDisplay.ts
git commit -s -m "feat(frontend): replace scope with tags in types and composables"
```

---

### Task 6: Frontend components — tag pills, published state, cleanup

**Goal:** Update all Vue components to use `tags` instead of `scope`, add `published` state display, replace scope chips with tag pills in `ContextBar`, and delete `ScopeChip.vue`.

**Files:**
- Modify: `frontend/src/components/ContextBar.vue`
- Modify: `frontend/src/App.vue`
- Modify: `frontend/src/components/EventLog.vue`
- Modify: `frontend/src/components/PackageEventGroup.vue`
- Modify: `frontend/src/components/EventRow.vue`
- Modify: `frontend/src/components/PackageCard.vue`
- Modify: `frontend/src/components/HealthHeader.vue`
- Modify: `frontend/src/components/FailureBoard.vue`
- Delete: `frontend/src/components/ScopeChip.vue`

**Acceptance Criteria:**
- [ ] `ContextBar` shows three fixed tag pills: `ppg`("PPG"), `common`("Common"), `container`("Container"). Prop renamed `activeTags`. Emit renamed `toggle-tag`. Label reads "Tags".
- [ ] `App.vue` uses `activeTags`/`toggleTag`/`filterByTags`/`filterEvents(activeTags.value, ...)`.
- [ ] `EventLog` group type uses `tags: string[]`; passes `:tags` to `PackageEventGroup`.
- [ ] `PackageEventGroup` prop is `tags: string[]`; renders one pill per tag; `scope === 'container'` → `tags.includes('container')`.
- [ ] `EventRow` renders one pill per tag from `event.tags`; `event.scope === 'container'` → `(event.tags ?? []).includes('container')`.
- [ ] `PackageCard` has `published` in `STATE_COLOR`/`STATE_BG`/`STATE_LABEL`; renders tag pills from `pkg.tags`.
- [ ] `HealthHeader` counts `published` as success in `okCount`.
- [ ] `FailureBoard` excludes `published` from `failingPackages`; includes in `okPackages`.
- [ ] `ScopeChip.vue` deleted.
- [ ] `cd frontend && npm run build` exits 0.

**Verify:** `cd frontend && npm run build` → exits 0

**Steps:**

- [ ] **Step 1: Update `ContextBar.vue`**

Replace the `SCOPES` array and scope-related props/emits. The new script section:

```typescript
defineProps<{
  version: string
  updatedAt: string | null
  activeTags: string[]
  contexts: Context[]
  selectedContext: Context
  availableVersions: string[]
}>()

const emit = defineEmits<{
  'update:version': [version: string]
  'toggle-tag': [tag: string]
  'update:context': [ctx: Context]
}>()

const TAGS = [
  { id: 'ppg', label: 'PPG' },
  { id: 'common', label: 'Common' },
  { id: 'container', label: 'Container' },
]
```

Replace the scope pills section in the template:

```html
<!-- Tag pills -->
<div style="display: flex; align-items: center; gap: 9px; flex-wrap: wrap; border-top: 1px solid var(--border); padding-top: 12px;">
  <span style="font-size: 11px; color: var(--text-muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; margin-right: 2px;">Tags</span>
  <button
    v-for="t in TAGS"
    :key="t.id"
    @click="emit('toggle-tag', t.id)"
    :style="activeTags.includes(t.id)
      ? 'background: var(--brand-purple-tint); color: var(--brand-purple); padding: 3px 10px; border-radius: 8px; border: 2px solid var(--brand-purple); font-size: 11.5px; font-weight: 600; cursor: pointer; font-family: inherit;'
      : 'background: transparent; color: var(--text-secondary); padding: 4px 11px; border-radius: 8px; border: 1px solid var(--border); font-size: 11.5px; font-weight: 500; cursor: pointer; font-family: inherit;'"
  >{{ t.label }}</button>
</div>
```

- [ ] **Step 2: Update `App.vue`**

Rename `activeScopes` → `activeTags` and `toggleScope` → `toggleTag` throughout:

```typescript
const activeTags = ref<string[]>([])

function toggleTag(tag: string) {
  const idx = activeTags.value.indexOf(tag)
  if (idx >= 0) {
    activeTags.value = activeTags.value.filter(s => s !== tag)
  } else {
    activeTags.value = [...activeTags.value, tag]
  }
}

function selectContext(ctx: Context) {
  selectedContext.value = ctx
  activeTags.value = []
  refresh()
}
```

Update the `usePackages` destructure:

```typescript
const { data: allPackages, rawData: rawPackages, availableVersions, refresh: refreshPackages, filterByTags } = usePackages(apiBase, version, prefixDepth)
```

Update computed values:

```typescript
const filteredPackages = computed(() => filterByTags(activeTags.value))
const filteredEvents = computed(() => filterEvents(activeTags.value, version.value, prefixDepth.value, selectedContext.value.prefix))
```

Update the `ContextBar` usage in template:

```html
<ContextBar
  :version="version"
  :updated-at="updatedAt"
  :active-tags="activeTags"
  :contexts="contexts"
  :selected-context="selectedContext"
  :available-versions="availableVersions"
  @update:version="version = $event"
  @toggle-tag="toggleTag"
  @update:context="selectContext"
/>
```

- [ ] **Step 3: Update `EventLog.vue`**

Find the group type definition (line ~159) and change `scope: string` to `tags: string[]`.

Find where groups are built (line ~179):
```typescript
result.push({ key, project: sorted[0].project, pkg: sorted[0].package, tags: sorted[0].tags ?? [], events: sorted })
```

Update the `PackageEventGroup` usage in template (line ~386):
```html
:tags="group.tags"
```

- [ ] **Step 4: Update `PackageEventGroup.vue`**

Change the prop and all usages:

```typescript
const props = defineProps<{
  // ... other props unchanged ...
  tags: string[]
}>()
```

Import `TAG_STYLE`, `TAG_LABEL`, `displayVersion` from `useEventDisplay`:

```typescript
import { TAG_STYLE, TAG_LABEL, displayVersion } from '../composables/useEventDisplay'
```

Replace the scope badge in the template with one pill per tag:

```html
<!-- Row 3: tag pills + version badge + project path -->
<div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap;">
  <span
    v-for="tag in props.tags"
    :key="tag"
    :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${TAG_STYLE[tag] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`"
  >{{ TAG_LABEL[tag] ?? tag }}</span>
  <span
    v-if="displayVersion(head.version, props.tags.includes('container'))"
    ...
  >{{ displayVersion(head.version, props.tags.includes('container')) }}</span>
```

Replace all `scope === 'container'` with `props.tags.includes('container')`.

- [ ] **Step 5: Update `EventRow.vue`**

Import `TAG_STYLE`, `TAG_LABEL` from `useEventDisplay` (remove `SCOPE_STYLE`, `SCOPE_LABEL` imports):

```typescript
import { GLYPH, GLYPH_COLOR, GLYPH_BG, TAG_STYLE, TAG_LABEL, eventTitle, timeStr, showReason as _showReason, displayVersion } from '../composables/useEventDisplay'
```

Replace the single scope badge with one pill per tag:

```html
<span
  v-for="tag in (props.event.tags ?? [])"
  :key="tag"
  :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${TAG_STYLE[tag] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`"
>{{ TAG_LABEL[tag] ?? tag }}</span>
```

Replace `event.scope === 'container'` with `(event.tags ?? []).includes('container')` in both the `v-if` and style conditional for the version badge.

- [ ] **Step 6: Update `PackageCard.vue`**

Add `published` to the state maps:

```typescript
const STATE_COLOR: Record<string, string> = {
  succeeded: 'var(--ok)',
  published: 'var(--ok)',
  failed: 'var(--fail)',
  // ... rest unchanged
}

const STATE_BG: Record<string, string> = {
  succeeded: 'var(--ok-tint)',
  published: 'var(--ok-tint)',
  failed: 'var(--fail-tint)',
  // ... rest unchanged
}

const STATE_LABEL: Record<string, string> = {
  succeeded: 'Succeeded',
  published: 'Published',
  failed: 'Failed',
  // ... rest unchanged
}
```

Remove `SCOPE_LABEL` map. Replace the scope badge with tag pills (import `TAG_STYLE`, `TAG_LABEL` from `useEventDisplay`):

```html
<span
  v-for="tag in (props.pkg.tags ?? [])"
  :key="tag"
  :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${TAG_STYLE[tag] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`"
>{{ TAG_LABEL[tag] ?? tag }}</span>
```

- [ ] **Step 7: Update `HealthHeader.vue`**

Update `okCount` to include `published`:

```typescript
const okCount = computed(() => props.packages.filter(p =>
  p.rollup_state === 'succeeded' || p.rollup_state === 'published'
).length)
```

- [ ] **Step 8: Update `FailureBoard.vue`**

```typescript
const failingPackages = computed(() => props.packages.filter(p =>
  p.rollup_state !== 'succeeded' && p.rollup_state !== 'published'
))
const okPackages = computed(() => props.packages.filter(p =>
  p.rollup_state === 'succeeded' || p.rollup_state === 'published'
))
```

- [ ] **Step 9: Delete `ScopeChip.vue`**

```bash
rm frontend/src/components/ScopeChip.vue
```

- [ ] **Step 10: Build and commit**

```bash
cd frontend && npm run build
```

Expected: exits 0. Fix any remaining TypeScript type errors that the compiler surfaces.

```bash
git add frontend/src/components/ContextBar.vue \
        frontend/src/App.vue \
        frontend/src/components/EventLog.vue \
        frontend/src/components/PackageEventGroup.vue \
        frontend/src/components/EventRow.vue \
        frontend/src/components/PackageCard.vue \
        frontend/src/components/HealthHeader.vue \
        frontend/src/components/FailureBoard.vue
git rm frontend/src/components/ScopeChip.vue
git commit -s -m "feat(frontend): replace scope chips with tag pills, add published state display"
```
