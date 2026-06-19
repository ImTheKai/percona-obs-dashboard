# Backend Workflow Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace scattered scope inference with a unified `ProjectClassifier`, add `tags`/`is_release` tracking, persist build-target state duration history, and split the worker pipeline so release packages only run the binaries-check task while real-time packages continue to get the full build-event pipeline.

**Architecture:** A new `internal/obs/classifier.go` provides `Classify(root, project)` and `ProjectTags(root, project)` as the single authority for project classification. The `Package` model gains `Tags []string` and `IsRelease bool` alongside the existing `Scope` field (Scope removal is a separate cleanup after the frontend is updated). A new `target_state_durations` table records per-target state transitions inline in `UpsertPackageState`. Release packages are isolated end-to-end: no SSE broadcast, no build events, and a dedicated `BinariesCheckTask` in place of the full worker pipeline.

**Tech Stack:** Go 1.25, chi router, SQLite (WAL mode via `modernc.org/sqlite`), RabbitMQ AMQP, Vue 3 + TypeScript (frontend not changed in this plan).

**User decisions (already made):**
- `isv:common` does not exist — poller uses a single configurable root (default `isv:percona`), not a hardcoded list.
- Container project kinds dropped (Option 2): container detection stays per-package via `is_container`, not project-level; no `KindDevContainer` etc.
- `Tags` replace `Scope` on packages: `["ppg"]`, `["ppg","pr"]`, `["ppg","common"]`, `["common"]`, `["ppg","release"]`.
- `is_release INTEGER` column added for fast SQL filtering without JSON parsing.
- `published` is the single terminal `RollupState` for both real-time and release packages.
- Release packages enter the working set when `rollup != published OR is_container IS NULL`; removed when both conditions are false.
- State duration recording happens inside `UpsertPackageState` (the only place where old targets are available before overwrite).
- `BinariesCheckTask` uses `RepoPublishStates` (the `_result` API), not `PackageBinaries`.
- MQ is the only consumer that deletes release packages; all other release MQ messages are silently dropped.

---

## File Map

| File | Change |
|------|--------|
| `backend/internal/config/config.go` | Add `OBSRoot string` to `Config` struct |
| `backend/internal/obs/classifier.go` | **Create** — `ProjectKind`, `Classify`, `ProjectTags`, `IsRealTime`, `EventScope` |
| `backend/internal/obs/classifier_test.go` | **Create** — unit tests |
| `backend/internal/model/types.go` | Add `RollupPublished`; add `Tags []string`, `IsRelease bool` to `Package` |
| `backend/internal/store/db.go` | Migrations: `tags`, `is_release`, `target_state_durations`; data backfills |
| `backend/internal/store/packages.go` | Update `scanPackages`, `UpsertPackageState`, `GetActivePackages`; add `QueryBuildPackages`, `QueryReleasePackages`, `recordStateTransitions`; cascade deletes |
| `backend/internal/obs/poller.go` | Single `root`, use `Classify`, split real-time vs release, no broadcast for releases |
| `backend/internal/obs/tasks.go` | Update `PublishStateTask` to promote to `published`; add `BinariesCheckTask` |
| `backend/internal/worker/worker.go` | Two task slices, release suppression, updated auto-remove condition |
| `backend/internal/mq/consumer.go` | Add `root`, filter on `cfg.OBSRoot`, suppress non-delete release messages |
| `backend/internal/api/server.go` | Accept `root string`; pass to handlers |
| `backend/internal/api/handlers.go` | `packagesHandler` → `QueryBuildPackages`; `releasesPackagesHandler` → DB; `releasesReposHandler` → DB |
| `backend/cmd/obsboard/main.go` | Wire `cfg.OBSRoot` through all three constructors |

---

## Task 1: Config + ProjectClassifier

**Goal:** Add a configurable `OBSRoot` to the config and create `internal/obs/classifier.go` — the single authority for project-kind detection.

**Files:**
- Modify: `backend/internal/config/config.go`
- Create: `backend/internal/obs/classifier.go`
- Create: `backend/internal/obs/classifier_test.go`

**Acceptance Criteria:**
- [ ] `Config.OBSRoot` defaults to `"isv:percona"` and is overridable via `OBS_ROOT` env var
- [ ] `Classify("isv:percona", "isv:percona:ppg:17")` returns `KindDev`
- [ ] `Classify("isv:percona", "isv:percona:ppg:17:containers:ubi9")` returns `KindDev`
- [ ] `Classify("isv:percona", "isv:percona:ppg:releases:17")` returns `KindRelease`
- [ ] `Classify("isv:percona", "isv:percona:PR:pr-42:ppg:17")` returns `KindPR`
- [ ] `Classify("isv:percona", "isv:percona:ppg:common")` returns `KindPPGCommon`
- [ ] `Classify("isv:percona", "isv:percona:ppg:common:deps")` returns `KindPPGCommon`
- [ ] `Classify("isv:percona", "isv:percona:common")` returns `KindCommon`
- [ ] `Classify("isv:percona", "isv:percona:common:containers:ubi9")` returns `KindCommon`
- [ ] `KindDev.IsRealTime()` → true; `KindRelease.IsRealTime()` → false
- [ ] `KindDev.EventScope()` → `model.ScopeVersion`; `KindPR.EventScope()` → `model.ScopePR`
- [ ] `ProjectTags("isv:percona", "isv:percona:ppg:17")` → `["ppg"]`
- [ ] `ProjectTags("isv:percona", "isv:percona:ppg:releases:17")` → `["ppg","release"]`
- [ ] `go test ./internal/obs/ -run TestClassify` passes

**Verify:** `cd backend && go test ./internal/obs/ -run TestClassify -v` → all subtests PASS

**Steps:**

- [ ] **Step 1: Add `OBSRoot` to config**

In `backend/internal/config/config.go`, add `OBSRoot string` to the `Config` struct and wire the default + env binding:

```go
// In Config struct (top-level, alongside OBS/MQ/Poller fields):
type Config struct {
    OBSRoot    string        // NEW
    OBS        OBSConfig
    MQ         MQConfig
    Poller     PollerConfig
    Store      StoreConfig
    Server     ServerConfig
    WorkerPool WorkerPoolConfig
}
```

In `Load()`, add before the `cfg := &Config{...}` block:
```go
v.SetDefault("obs_root", "isv:percona")
_ = v.BindEnv("obs_root", "OBS_ROOT")
```

In the returned `cfg`:
```go
cfg := &Config{
    OBSRoot: v.GetString("obs_root"),    // NEW
    OBS: OBSConfig{ ... },
    // ...
}
```

- [ ] **Step 2: Write classifier tests first (TDD)**

Create `backend/internal/obs/classifier_test.go`:

```go
package obs

import (
    "testing"

    "github.com/percona/obs-dashboard/internal/model"
)

const root = "isv:percona"

func TestClassify(t *testing.T) {
    cases := []struct {
        project string
        want    ProjectKind
    }{
        {"isv:percona:ppg:17", KindDev},
        {"isv:percona:ppg:17:containers:ubi9", KindDev},
        {"isv:percona:ppg:releases:17", KindRelease},
        {"isv:percona:ppg:releases:17:containers:ubi9", KindRelease},
        {"isv:percona:PR:pr-42:ppg:17", KindPR},
        {"isv:percona:PR:pr-42:ppg:17:containers:ubi9", KindPR},
        {"isv:percona:ppg:common", KindPPGCommon},
        {"isv:percona:ppg:common:deps", KindPPGCommon},
        {"isv:percona:common", KindCommon},
        {"isv:percona:common:containers:ubi9", KindCommon},
        {"isv:other:project", KindUnknown},
        {"isv:percona", KindUnknown},
    }
    for _, c := range cases {
        if got := Classify(root, c.project); got != c.want {
            t.Errorf("Classify(%q) = %v, want %v", c.project, got, c.want)
        }
    }
}

func TestIsRealTime(t *testing.T) {
    if !KindDev.IsRealTime() { t.Error("KindDev.IsRealTime() should be true") }
    if !KindPR.IsRealTime()  { t.Error("KindPR.IsRealTime() should be true") }
    if !KindPPGCommon.IsRealTime() { t.Error("KindPPGCommon.IsRealTime() should be true") }
    if !KindCommon.IsRealTime()    { t.Error("KindCommon.IsRealTime() should be true") }
    if KindRelease.IsRealTime()    { t.Error("KindRelease.IsRealTime() should be false") }
    if KindUnknown.IsRealTime()    { t.Error("KindUnknown.IsRealTime() should be false") }
}

func TestEventScope(t *testing.T) {
    cases := []struct {
        kind ProjectKind
        want model.Scope
    }{
        {KindDev, model.ScopeVersion},
        {KindPR, model.ScopePR},
        {KindPPGCommon, model.ScopePPGCommon},
        {KindCommon, model.ScopeCommon},
        {KindRelease, model.ScopeRelease},
        {KindUnknown, model.ScopeCommon},
    }
    for _, c := range cases {
        if got := c.kind.EventScope(); got != c.want {
            t.Errorf("%v.EventScope() = %q, want %q", c.kind, got, c.want)
        }
    }
}

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
        {"isv:percona:common", []string{"common"}},
        {"isv:other", []string{}},
    }
    for _, c := range cases {
        got := ProjectTags(root, c.project)
        if len(got) != len(c.want) {
            t.Errorf("ProjectTags(%q) = %v, want %v", c.project, got, c.want)
            continue
        }
        for i := range got {
            if got[i] != c.want[i] {
                t.Errorf("ProjectTags(%q)[%d] = %q, want %q", c.project, i, got[i], c.want[i])
            }
        }
    }
}
```

Run: `cd backend && go test ./internal/obs/ -run TestClassify` → **FAIL** (classifier.go not created yet)

- [ ] **Step 3: Implement classifier**

Create `backend/internal/obs/classifier.go`:

```go
package obs

import (
    "strings"

    "github.com/percona/obs-dashboard/internal/model"
)

// ProjectKind categorises an OBS project relative to the configured root.
type ProjectKind int

const (
    KindUnknown   ProjectKind = iota
    KindDev       // <root>:ppg:<version>[:<subproject>]
    KindPR        // <root>:PR:pr-<n>:ppg:<version>[:<subproject>]
    KindPPGCommon // <root>:ppg:common[:<subproject>]
    KindCommon    // <root>:common[:<subproject>]
    KindRelease   // <root>:ppg:releases:<version>[:<subproject>]
)

func (k ProjectKind) IsRealTime() bool {
    switch k {
    case KindDev, KindPR, KindPPGCommon, KindCommon:
        return true
    }
    return false
}

// EventScope returns the model.Scope to use for SSE events from this project kind.
func (k ProjectKind) EventScope() model.Scope {
    switch k {
    case KindDev:
        return model.ScopeVersion
    case KindPR:
        return model.ScopePR
    case KindPPGCommon:
        return model.ScopePPGCommon
    case KindCommon:
        return model.ScopeCommon
    case KindRelease:
        return model.ScopeRelease
    default:
        return model.ScopeCommon
    }
}

// Classify returns the ProjectKind for project relative to root.
// root is the top-level namespace, e.g. "isv:percona".
func Classify(root, project string) ProjectKind {
    prefix := root + ":"
    if !strings.HasPrefix(project, prefix) {
        return KindUnknown
    }
    rel := project[len(prefix):]
    parts := strings.SplitN(rel, ":", -1)
    if len(parts) == 0 {
        return KindUnknown
    }
    switch parts[0] {
    case "ppg":
        if len(parts) < 2 {
            return KindUnknown
        }
        switch parts[1] {
        case "common":
            return KindPPGCommon
        case "releases":
            if len(parts) >= 3 {
                return KindRelease
            }
            return KindUnknown
        default:
            return KindDev
        }
    case "PR":
        return KindPR
    case "common":
        return KindCommon
    }
    return KindUnknown
}

// ProjectTags returns the tag slice to store on packages belonging to project.
func ProjectTags(root, project string) []string {
    switch Classify(root, project) {
    case KindDev:
        return []string{"ppg"}
    case KindPR:
        return []string{"ppg", "pr"}
    case KindPPGCommon:
        return []string{"ppg", "common"}
    case KindCommon:
        return []string{"common"}
    case KindRelease:
        return []string{"ppg", "release"}
    default:
        return []string{}
    }
}
```

- [ ] **Step 4: Run tests — expect PASS**

```
cd backend && go test ./internal/obs/ -run TestClassify -v
cd backend && go test ./internal/config/ -v
cd backend && go build ./...
```

Expected: all green, no compile errors.

- [ ] **Step 5: Commit**

```bash
cd backend && git add internal/obs/classifier.go internal/obs/classifier_test.go internal/config/config.go
git commit -s -m "feat: add OBSRoot config and ProjectClassifier"
```

---

## Task 2: DB Schema + Migrations

**Goal:** Add `tags`, `is_release`, and `target_state_durations` to the schema and backfill existing rows.

**Files:**
- Modify: `backend/internal/store/db.go`

**Acceptance Criteria:**
- [ ] Fresh DB opened with `store.Open` has `tags`, `is_release`, `target_state_durations` columns/table
- [ ] Existing DB (missing columns) has them added without error
- [ ] Rows previously with `scope='release'` have `is_release=1` after backfill
- [ ] Rows previously with `scope='version'` have `tags='["ppg"]'` after backfill
- [ ] `go test ./internal/store/ -run TestOpen` passes

**Verify:** `cd backend && go test ./internal/store/ -run TestOpen -v` → PASS

**Steps:**

- [ ] **Step 1: Add columns to schema constant and additive migrations**

In `backend/internal/store/db.go`, add two columns to the `schema` constant inside the `packages` table definition (before the closing `);`):

```sql
    tags           TEXT NOT NULL DEFAULT '[]',
    is_release     INTEGER NOT NULL DEFAULT 0,
```

So the full packages table ends with:
```sql
    container_tags TEXT NOT NULL DEFAULT '[]',
    tags           TEXT NOT NULL DEFAULT '[]',
    is_release     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (project, name)
);
```

Also add the `target_state_durations` table to the schema constant after the events table:

```sql
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
```

In the `Open` function, add to the additive migration block (after the existing `db.Exec` calls):

```go
db.Exec(`ALTER TABLE packages ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`)
db.Exec(`ALTER TABLE packages ADD COLUMN is_release INTEGER NOT NULL DEFAULT 0`)
```

- [ ] **Step 2: Add data backfill migrations**

Add a `migrateTagsAndIsRelease` call after the structural migration call in `Open`:

```go
if err := migrateTagsAndIsRelease(db); err != nil {
    db.Close()
    return nil, fmt.Errorf("migrate tags and is_release: %w", err)
}
if err := migrateSucceededToPublished(db); err != nil {
    db.Close()
    return nil, fmt.Errorf("migrate succeeded to published: %w", err)
}
```

Implement the two functions at the bottom of `db.go`:

```go
// migrateTagsAndIsRelease backfills tags JSON and is_release from the scope column.
// Idempotent: only updates rows where tags is still the default '[]'.
func migrateTagsAndIsRelease(db *sql.DB) error {
    _, err := db.Exec(`
        UPDATE packages SET tags = CASE
            WHEN scope = 'version'                          THEN '["ppg"]'
            WHEN scope = 'pr'                               THEN '["ppg","pr"]'
            WHEN scope = 'ppgcommon'                        THEN '["ppg","common"]'
            WHEN scope = 'common'                           THEN '["common"]'
            WHEN scope = 'release'                          THEN '["ppg","release"]'
            WHEN scope = 'container' AND project LIKE '%:PR:%' THEN '["ppg","pr"]'
            WHEN scope = 'container'                        THEN '["ppg"]'
            ELSE '[]'
        END
        WHERE tags = '[]'
    `)
    if err != nil {
        return err
    }
    _, err = db.Exec(`UPDATE packages SET is_release = 1 WHERE scope = 'release' AND is_release = 0`)
    return err
}

// migrateSucceededToPublished promotes packages where rollup_state = 'succeeded'
// and all targets in targets_json already have published=true to rollup_state = 'published'.
// Idempotent: only processes 'succeeded' rows.
func migrateSucceededToPublished(db *sql.DB) error {
    rows, err := db.Query(`SELECT project, name, targets_json FROM packages WHERE rollup_state = 'succeeded'`)
    if err != nil {
        return err
    }
    defer rows.Close()

    type row struct{ project, name, targetsJSON string }
    var candidates []row
    for rows.Next() {
        var r row
        if err := rows.Scan(&r.project, &r.name, &r.targetsJSON); err != nil {
            return err
        }
        candidates = append(candidates, r)
    }
    if err := rows.Err(); err != nil {
        return err
    }
    rows.Close()

    for _, c := range candidates {
        var targets []struct {
            State     string `json:"state"`
            Published bool   `json:"published"`
        }
        if err := json.Unmarshal([]byte(c.targetsJSON), &targets); err != nil {
            continue
        }
        if len(targets) == 0 {
            continue
        }
        allPublished := true
        for _, t := range targets {
            if t.State == "succeeded" && !t.Published {
                allPublished = false
                break
            }
        }
        if allPublished {
            db.Exec(`UPDATE packages SET rollup_state = 'published' WHERE project = ? AND name = ?`,
                c.project, c.name)
        }
    }
    return nil
}
```

Add `"encoding/json"` to the import block if not already present.

- [ ] **Step 3: Write/extend DB test**

In `backend/internal/store/db_test.go`, add a test that verifies migration applies to an existing DB:

```go
func TestOpenMigrationAppliesTagsAndIsRelease(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "test.db")

    // Open a fresh DB (gets full schema including new columns).
    db, err := Open(path)
    if err != nil {
        t.Fatalf("Open: %v", err)
    }

    // Insert a row using the old-style scope column (simulating legacy data).
    _, err = db.Exec(`
        INSERT INTO packages (project, name, scope, rollup_state, ok_targets, total_targets, targets_json, updated_at, tags, is_release)
        VALUES ('isv:percona:ppg:17', 'pg_tde', 'release', 'succeeded', 1, 1, '[]', datetime('now'), '[]', 0)
    `)
    if err != nil {
        t.Fatalf("insert: %v", err)
    }
    db.Close()

    // Re-open: migrations should backfill tags and is_release.
    db2, err := Open(path)
    if err != nil {
        t.Fatalf("re-open: %v", err)
    }
    defer db2.Close()

    var tags string
    var isRelease int
    if err := db2.QueryRow(`SELECT tags, is_release FROM packages WHERE project='isv:percona:ppg:17' AND name='pg_tde'`).
        Scan(&tags, &isRelease); err != nil {
        t.Fatalf("scan: %v", err)
    }
    if tags != `["ppg","release"]` {
        t.Errorf("tags = %q, want [\"ppg\",\"release\"]", tags)
    }
    if isRelease != 1 {
        t.Errorf("is_release = %d, want 1", isRelease)
    }
}
```

- [ ] **Step 4: Run tests**

```
cd backend && go test ./internal/store/ -run TestOpen -v
cd backend && go build ./...
```

Expected: TestOpen passes, no compile errors.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/store/db.go backend/internal/store/db_test.go
git commit -s -m "feat(store): add tags, is_release, target_state_durations schema and migrations"
```

---

## Task 3: Model + Store Layer

**Goal:** Extend the `Package` model with `Tags`/`IsRelease`/`RollupPublished`, update all store functions to read/write the new columns, record per-target state duration transitions, and add the new query functions.

**Files:**
- Modify: `backend/internal/model/types.go`
- Modify: `backend/internal/store/packages.go`

**Acceptance Criteria:**
- [ ] `model.RollupPublished` constant exists with value `"published"`
- [ ] `model.Package` has `Tags []string` and `IsRelease bool` fields
- [ ] `store.UpsertPackageState` writes `tags` and `is_release` columns; preserves existing tags when new Tags is nil/empty
- [ ] `store.UpsertPackageState` inserts a row in `target_state_durations` for each new/changed target state
- [ ] `store.UpsertPackageState` closes (sets `exited_at`) the previous duration row when a target's state changes
- [ ] `store.GetActivePackages` returns packages where `rollup_state != 'published' OR is_container IS NULL`
- [ ] `store.QueryBuildPackages("isv:percona", "ppg", "17")` returns packages from `isv:percona:ppg:17*`, `isv:percona:ppg:common*`, and `isv:percona:common*`
- [ ] `store.QueryReleasePackages("isv:percona:ppg:releases")` returns only `is_release=1` packages
- [ ] Cascade: `DeletePackage` and `DeletePackagesByProject` also delete from `target_state_durations`
- [ ] `go test ./internal/store/ -v` passes

**Verify:** `cd backend && go test ./internal/store/ -v` → all tests PASS; `go build ./...` clean

**Steps:**

- [ ] **Step 1: Extend model/types.go**

In `backend/internal/model/types.go`, add `RollupPublished` to the RollupState constants block:

```go
const (
    RollupFailed       RollupState = "failed"
    RollupBroken       RollupState = "broken"
    RollupUnresolvable RollupState = "unresolvable"
    RollupBlocked      RollupState = "blocked"
    RollupBuilding     RollupState = "building"
    RollupFinished     RollupState = "finished"
    RollupScheduled    RollupState = "scheduled"
    RollupSucceeded    RollupState = "succeeded"
    RollupPublished    RollupState = "published"  // NEW — terminal: all targets built and repos published
)
```

Add `Tags []string` and `IsRelease bool` to the `Package` struct (keep `Scope` for now):

```go
type Package struct {
    Project        string      `json:"project"`
    Name           string      `json:"name"`
    Scope          Scope       `json:"scope"`
    Tags           []string    `json:"tags,omitempty"`   // NEW
    IsRelease      bool        `json:"is_release,omitempty"` // NEW
    RollupState    RollupState `json:"rollup_state"`
    OKTargets      int         `json:"ok_targets"`
    TotalTargets   int         `json:"total_targets"`
    IsContainer    *bool       `json:"is_container,omitempty"`
    Version        string      `json:"version,omitempty"`
    ContainerTags  []string    `json:"container_tags,omitempty"`
    Trigger        *Trigger    `json:"trigger,omitempty"`
    Targets        []Target    `json:"targets"`
    UpdatedAt      time.Time   `json:"updated_at"`
    StateChangedAt *time.Time  `json:"state_changed_at,omitempty"`
}
```

- [ ] **Step 2: Update packageSelectCols and scanPackages**

In `backend/internal/store/packages.go`, update `packageSelectCols` to include the new columns:

```go
const packageSelectCols = ` project, name, scope, rollup_state, ok_targets, total_targets,
    trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
    state_changed_at, is_container, version, container_tags, tags, is_release`
```

Update `scanPackages` to scan `tags` and `is_release` (add two new scan destinations after `containerTagsJSON`):

```go
func scanPackages(rows *sql.Rows) ([]*model.Package, error) {
    pkgs := make([]*model.Package, 0)
    for rows.Next() {
        p := &model.Package{}
        var trigWhat, trigKind sql.NullString
        var trigAt sql.NullTime
        var targetsJSON string
        var stateChangedAt sql.NullTime
        var isContainerNull sql.NullInt64
        var containerTagsJSON string
        var tagsJSON string
        var isRelease int
        if err := rows.Scan(
            &p.Project, &p.Name, &p.Scope, &p.RollupState,
            &p.OKTargets, &p.TotalTargets,
            &trigWhat, &trigKind, &trigAt,
            &targetsJSON, &p.UpdatedAt,
            &stateChangedAt, &isContainerNull, &p.Version,
            &containerTagsJSON, &tagsJSON, &isRelease,
        ); err != nil {
            return nil, err
        }
        if isContainerNull.Valid {
            v := isContainerNull.Int64 != 0
            p.IsContainer = &v
        }
        if trigWhat.Valid {
            p.Trigger = &model.Trigger{
                What: trigWhat.String,
                Kind: trigKind.String,
                At:   trigAt.Time,
            }
        }
        if stateChangedAt.Valid {
            t := stateChangedAt.Time
            p.StateChangedAt = &t
        }
        if err := json.Unmarshal([]byte(targetsJSON), &p.Targets); err != nil {
            return nil, err
        }
        if containerTagsJSON != "" && containerTagsJSON != "[]" {
            if err := json.Unmarshal([]byte(containerTagsJSON), &p.ContainerTags); err != nil {
                return nil, err
            }
        }
        if tagsJSON != "" && tagsJSON != "[]" {
            if err := json.Unmarshal([]byte(tagsJSON), &p.Tags); err != nil {
                return nil, err
            }
        }
        p.IsRelease = isRelease != 0
        pkgs = append(pkgs, p)
    }
    return pkgs, rows.Err()
}
```

- [ ] **Step 3: Update UpsertPackageState**

Replace the existing `UpsertPackageState` implementation:

```go
func UpsertPackageState(db *sql.DB, p *model.Package, now time.Time) error {
    // Read current targets for state-duration recording (before overwrite).
    var prevTargetsJSON string
    var prevTargets []model.Target
    if err := db.QueryRow(`SELECT targets_json FROM packages WHERE project = ? AND name = ?`,
        p.Project, p.Name).Scan(&prevTargetsJSON); err == nil {
        _ = json.Unmarshal([]byte(prevTargetsJSON), &prevTargets)
    }

    targetsJSON, err := json.Marshal(p.Targets)
    if err != nil {
        return err
    }
    tagsJSON, err := json.Marshal(p.Tags)
    if err != nil {
        return err
    }
    if tagsJSON == nil {
        tagsJSON = []byte("[]")
    }
    containerTagsJSON, err := json.Marshal(p.ContainerTags)
    if err != nil {
        return err
    }
    if containerTagsJSON == nil {
        containerTagsJSON = []byte("[]")
    }

    var trigWhat, trigKind sql.NullString
    var trigAt sql.NullTime
    if p.Trigger != nil {
        trigWhat = sql.NullString{String: p.Trigger.What, Valid: true}
        trigKind = sql.NullString{String: p.Trigger.Kind, Valid: true}
        trigAt = sql.NullTime{Time: p.Trigger.At, Valid: true}
    }
    var isContainerVal interface{}
    if p.IsContainer != nil {
        if *p.IsContainer {
            isContainerVal = 1
        } else {
            isContainerVal = 0
        }
    }
    isReleaseVal := 0
    if p.IsRelease {
        isReleaseVal = 1
    }

    _, err = db.Exec(`
        INSERT INTO packages
            (project, name, scope, rollup_state, ok_targets, total_targets,
             trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
             state_changed_at, is_container, version, container_tags, tags, is_release)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(project, name) DO UPDATE SET
            scope=excluded.scope,
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
        p.Project, p.Name, string(p.Scope), string(p.RollupState),
        p.OKTargets, p.TotalTargets,
        trigWhat, trigKind, trigAt,
        string(targetsJSON), p.UpdatedAt, now,
        isContainerVal, p.Version, string(containerTagsJSON),
        string(tagsJSON), isReleaseVal,
    )
    if err != nil {
        return err
    }

    // Record state duration transitions (non-fatal errors).
    recordStateTransitions(db, p.Project, p.Name, prevTargets, p.Targets, now)
    return nil
}

// recordStateTransitions updates target_state_durations whenever a target's
// state changes or a new target appears. Called from UpsertPackageState only.
func recordStateTransitions(db *sql.DB, project, pkg string, prev, next []model.Target, now time.Time) {
    oldByKey := make(map[string]model.Target, len(prev))
    for _, t := range prev {
        oldByKey[t.Repo+"/"+t.Arch] = t
    }
    for _, t := range next {
        key := t.Repo + "/" + t.Arch
        old, existed := oldByKey[key]
        if existed && old.State == t.State {
            continue // no change
        }
        if existed {
            // Close the previous open entry for this target.
            db.Exec(`
                UPDATE target_state_durations
                SET exited_at = ?,
                    duration_ms = CAST((julianday(?) - julianday(entered_at)) * 86400000 AS INTEGER)
                WHERE project = ? AND package = ? AND repo = ? AND arch = ?
                  AND state = ? AND exited_at IS NULL`,
                now, now, project, pkg, t.Repo, t.Arch, old.State,
            )
        }
        // Open a new entry for the new state.
        db.Exec(`
            INSERT INTO target_state_durations (project, package, repo, arch, state, entered_at)
            VALUES (?, ?, ?, ?, ?, ?)`,
            project, pkg, t.Repo, t.Arch, t.State, now,
        )
    }
}
```

- [ ] **Step 4: Update GetActivePackages and GetFinishedPackagesByProject**

Change the `WHERE` clause in `GetActivePackages`:

```go
// GetActivePackages returns packages that need worker attention.
func GetActivePackages(db *sql.DB) ([]*model.Package, error) {
    rows, err := db.Query(`SELECT`+packageSelectCols+`
        FROM packages
        WHERE rollup_state != 'published' OR is_container IS NULL
        ORDER BY project, name`,
    )
    // ... rest unchanged
}
```

- [ ] **Step 5: Add QueryBuildPackages and QueryReleasePackages**

Add these functions after `QueryPackages`:

```go
// QueryBuildPackages returns packages for the builds tab: the version-specific
// project and its container subprojects, plus the product-common subtree, plus
// the global common subtree.
// root is e.g. "isv:percona", product is e.g. "ppg", version is e.g. "17".
func QueryBuildPackages(db *sql.DB, root, product, version string) ([]*model.Package, error) {
    vp := root + ":" + product + ":" + version
    cp := root + ":" + product + ":common"
    gp := root + ":common"
    rows, err := db.Query(`SELECT`+packageSelectCols+`
        FROM packages
        WHERE (project = ? OR project LIKE ? || ':%')
           OR (project = ? OR project LIKE ? || ':%')
           OR (project = ? OR project LIKE ? || ':%')
        ORDER BY project, name`,
        vp, vp, cp, cp, gp, gp,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    return scanPackages(rows)
}

// QueryReleasePackages returns packages in the releases subtree (is_release=1).
// prefix is e.g. "isv:percona:ppg:releases".
func QueryReleasePackages(db *sql.DB, prefix string) ([]*model.Package, error) {
    rows, err := db.Query(`SELECT`+packageSelectCols+`
        FROM packages
        WHERE (project = ? OR project LIKE ? || ':%')
          AND is_release = 1
        ORDER BY project, name`,
        prefix, prefix,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    return scanPackages(rows)
}
```

- [ ] **Step 6: Update cascade deletes**

```go
func DeletePackagesByProject(db *sql.DB, project string) error {
    db.Exec(`DELETE FROM target_state_durations WHERE project = ?`, project)
    _, err := db.Exec(`DELETE FROM packages WHERE project = ?`, project)
    return err
}

func DeletePackage(db *sql.DB, project, name string) error {
    db.Exec(`DELETE FROM target_state_durations WHERE project = ? AND package = ?`, project, name)
    _, err := db.Exec(`DELETE FROM packages WHERE project = ? AND name = ?`, project, name)
    return err
}
```

- [ ] **Step 7: Extend packages_test.go**

Add to `backend/internal/store/packages_test.go`:

```go
func TestQueryBuildPackages(t *testing.T) {
    db, err := Open(":memory:")
    if err != nil { t.Fatal(err) }
    defer db.Close()

    // Insert packages across three namespaces.
    now := time.Now().UTC()
    insert := func(project, name string) {
        db.Exec(`INSERT INTO packages (project, name, scope, rollup_state, ok_targets, total_targets, targets_json, updated_at)
            VALUES (?, ?, 'version', 'building', 0, 0, '[]', ?)`, project, name, now)
    }
    insert("isv:percona:ppg:17", "pg_tde")
    insert("isv:percona:ppg:17:containers:ubi9", "pg_container")
    insert("isv:percona:ppg:common", "common_pkg")
    insert("isv:percona:common", "global_common")
    insert("isv:percona:ppg:releases:17", "release_pkg")

    pkgs, err := QueryBuildPackages(db, "isv:percona", "ppg", "17")
    if err != nil { t.Fatal(err) }

    names := make(map[string]bool)
    for _, p := range pkgs {
        names[p.Name] = true
    }
    for _, want := range []string{"pg_tde", "pg_container", "common_pkg", "global_common"} {
        if !names[want] { t.Errorf("missing expected package %q", want) }
    }
    if names["release_pkg"] {
        t.Error("release_pkg should not appear in build packages")
    }
}

func TestQueryReleasePackages(t *testing.T) {
    db, err := Open(":memory:")
    if err != nil { t.Fatal(err) }
    defer db.Close()

    now := time.Now().UTC()
    db.Exec(`INSERT INTO packages (project, name, scope, rollup_state, ok_targets, total_targets, targets_json, updated_at, is_release)
        VALUES ('isv:percona:ppg:releases:17', 'pg_tde', 'release', 'building', 0, 0, '[]', ?, 1)`, now)
    db.Exec(`INSERT INTO packages (project, name, scope, rollup_state, ok_targets, total_targets, targets_json, updated_at, is_release)
        VALUES ('isv:percona:ppg:17', 'pg_tde_dev', 'version', 'building', 0, 0, '[]', ?, 0)`, now)

    pkgs, err := QueryReleasePackages(db, "isv:percona:ppg:releases")
    if err != nil { t.Fatal(err) }
    if len(pkgs) != 1 { t.Fatalf("expected 1 release package, got %d", len(pkgs)) }
    if pkgs[0].Name != "pg_tde" { t.Errorf("expected pg_tde, got %q", pkgs[0].Name) }
}

func TestStateTransitionsRecorded(t *testing.T) {
    db, err := Open(":memory:")
    if err != nil { t.Fatal(err) }
    defer db.Close()

    pkg := &model.Package{
        Project:     "isv:percona:ppg:17",
        Name:        "pg_tde",
        Scope:       model.ScopeVersion,
        RollupState: model.RollupBuilding,
        Targets:     []model.Target{{Repo: "RockyLinux_9", Arch: "x86_64", State: "building"}},
        UpdatedAt:   time.Now().UTC(),
    }
    now := time.Now().UTC()
    if err := UpsertPackageState(db, pkg, now); err != nil { t.Fatal(err) }

    // Transition to succeeded.
    pkg.Targets[0].State = "succeeded"
    pkg.RollupState = model.RollupSucceeded
    if err := UpsertPackageState(db, pkg, now.Add(time.Minute)); err != nil { t.Fatal(err) }

    var count int
    db.QueryRow(`SELECT COUNT(*) FROM target_state_durations WHERE project = ? AND package = ?`,
        pkg.Project, pkg.Name).Scan(&count)
    if count != 2 { // one entry per state (building, succeeded)
        t.Errorf("expected 2 duration rows, got %d", count)
    }
    var durMs sql.NullInt64
    db.QueryRow(`SELECT duration_ms FROM target_state_durations WHERE state = 'building' AND exited_at IS NOT NULL`).Scan(&durMs)
    if !durMs.Valid || durMs.Int64 <= 0 {
        t.Error("expected positive duration_ms for closed building entry")
    }
}
```

- [ ] **Step 8: Run tests**

```
cd backend && go test ./internal/store/ -v
cd backend && go build ./...
```

Expected: all tests pass.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/model/types.go backend/internal/store/packages.go backend/internal/store/packages_test.go
git commit -s -m "feat(model,store): add Tags/IsRelease/RollupPublished; state duration tracking; QueryBuildPackages"
```

---

## Task 4: Poller Refactor

**Goal:** Replace the hardcoded two-root list with a single configurable root, classify each project with `Classify`, and split the tick loop so real-time projects get the existing broadcast pipeline while release projects are upserted silently.

**Files:**
- Modify: `backend/internal/obs/poller.go`

**Acceptance Criteria:**
- [ ] `NewPoller` signature is `NewPoller(client, db, interval, h, ws, root string) *Poller`
- [ ] Poller iterates only `root` (no `isv:common`)
- [ ] Real-time projects (`kind.IsRealTime()`) upsert, hub.Notify, ws.Add, AppendEvent (unchanged behavior)
- [ ] Release projects (`KindRelease`) upsert but do NOT call hub.Notify or AppendEvent
- [ ] Release packages set `pkg.Tags = ["ppg","release"]` and `pkg.IsRelease = true`
- [ ] Release packages added to ws only if `rollup != published OR pkg.IsContainer == nil`
- [ ] Published release package whose target set changes has rollup reset to `building`
- [ ] All real-time packages still get `pkg.Tags` populated (alongside existing `pkg.Scope`)
- [ ] `go test ./internal/obs/ -run TestPoller` passes
- [ ] `go build ./...` clean

**Verify:** `cd backend && go test ./internal/obs/ -run TestPoller -v && go build ./...`

**Steps:**

- [ ] **Step 1: Update Poller struct and NewPoller**

In `backend/internal/obs/poller.go`, change the struct and constructor:

```go
type Poller struct {
    client   *Client
    db       *sql.DB
    interval time.Duration
    root     string          // was: roots []string
    hub      *hubpkg.Hub
    ws       *workingset.WorkingSet
}

func NewPoller(client *Client, db *sql.DB, interval time.Duration, h *hubpkg.Hub, ws *workingset.WorkingSet, root string) *Poller {
    return &Poller{client: client, db: db, interval: interval, root: root, hub: h, ws: ws}
}
```

- [ ] **Step 2: Refactor tick**

Replace the entire `tick` method with the following (note: `Run` is unchanged):

```go
func (p *Poller) tick(ctx context.Context) {
    projects, err := p.discoverProjects(ctx, p.root)
    if err != nil {
        slog.Error("poller: discover projects", "root", p.root, "err", err)
        return
    }

    liveProjects := make(map[string]bool, len(projects))
    for _, proj := range projects {
        liveProjects[proj] = true
    }

    existing, err := store.QueryPackages(p.db, p.root)
    if err != nil {
        slog.Error("poller: query packages", "root", p.root, "err", err)
        return
    }
    byKey := make(map[string]*model.Package, len(existing))
    for _, pkg := range existing {
        byKey[pkg.Project+"/"+pkg.Name] = pkg
    }

    for _, project := range projects {
        if ctx.Err() != nil {
            return
        }
        kind := Classify(p.root, project)
        if kind == KindUnknown {
            continue
        }

        results, err := p.client.BuildResults(ctx, project)
        if err != nil {
            slog.Warn("poller: build results", "project", project, "err", err)
            continue
        }

        byPkg := map[string][]PackageBuildState{}
        for _, r := range results {
            byPkg[r.Package] = append(byPkg[r.Package], r)
        }

        for pkgName, targets := range byPkg {
            scope := InferScope(project) // kept for backward compat while Scope is on Package
            pkg := buildPackage(project, pkgName, scope, targets)
            pkg.Tags = ProjectTags(p.root, project)
            pkg.IsRelease = kind == KindRelease

            key := project + "/" + pkgName
            prev := byKey[key]

            rollupChanged := prev == nil || prev.RollupState != pkg.RollupState
            tagsChanged := prev != nil && len(prev.Tags) != len(pkg.Tags)

            if kind.IsRealTime() {
                if rollupChanged || targetsChanged(prev, pkg) || tagsChanged {
                    if err := store.UpsertPackageState(p.db, pkg, time.Now().UTC()); err != nil {
                        slog.Error("poller: upsert package", "pkg", pkgName, "err", err)
                        continue
                    }
                    p.hub.Notify(hubpkg.PackageUpdate(pkg))
                    p.ws.Add(pkg)
                    if rollupChanged && !isTransientRollup(pkg.RollupState) {
                        evt := stateChangeEvent(pkg, prev)
                        if err := store.AppendEvent(p.db, evt); err != nil {
                            slog.Error("poller: append event", "err", err)
                        } else {
                            p.hub.Notify(hubpkg.NewEvent(evt))
                        }
                    }
                }
            } else {
                // Release project: upsert silently, no SSE broadcast, no events.
                // Reset rollup to building if target set changed on an already-published package.
                if prev != nil && prev.RollupState == model.RollupPublished && targetsChanged(prev, pkg) {
                    pkg.RollupState = model.RollupBuilding
                }
                if rollupChanged || targetsChanged(prev, pkg) || tagsChanged {
                    if err := store.UpsertPackageState(p.db, pkg, time.Now().UTC()); err != nil {
                        slog.Error("poller: upsert release package", "pkg", pkgName, "err", err)
                        continue
                    }
                }
                // Add to working set only if there is work remaining.
                if pkg.RollupState != model.RollupPublished || pkg.IsContainer == nil {
                    p.ws.Add(pkg)
                }
            }
        }
    }

    // Garbage-collect packages for projects no longer in OBS.
    storedProjects := make(map[string]bool)
    for _, pkg := range existing {
        storedProjects[pkg.Project] = true
    }
    for proj := range storedProjects {
        if !liveProjects[proj] {
            slog.Info("poller: removing packages for deleted project", "project", proj)
            if err := store.DeletePackagesByProject(p.db, proj); err != nil {
                slog.Error("poller: delete packages", "project", proj, "err", err)
            }
        }
    }
}
```

Note: `Run` is unchanged — it already calls `p.tick(ctx)` immediately before the loop.

- [ ] **Step 3: Fix the existing poller tests**

`NewPoller` signature changed. In `backend/internal/obs/poller_test.go`, update all `NewPoller` calls to add the `root` argument:

Find: `NewPoller(client, db, interval, h, ws)` (or similar)  
Replace with: `NewPoller(client, db, interval, h, ws, "isv:percona")`

Check and update the file:
```bash
grep -n "NewPoller" backend/internal/obs/poller_test.go
```

- [ ] **Step 4: Run tests**

```
cd backend && go test ./internal/obs/ -run TestPoller -v
cd backend && go build ./...
```

Fix any compilation errors before proceeding.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/obs/poller.go backend/internal/obs/poller_test.go
git commit -s -m "feat(poller): single root, classify projects, silent release upsert"
```

---

## Task 5: Worker Pipeline Split + BinariesCheckTask

**Goal:** Split the worker into separate real-time and release task pipelines, add `BinariesCheckTask` for release packages, update `PublishStateTask` to promote rollup to `published`, suppress SSE broadcast and build events for release packages, and update the auto-remove condition to `published`.

**Files:**
- Modify: `backend/internal/obs/tasks.go`
- Modify: `backend/internal/worker/worker.go`

**Acceptance Criteria:**
- [ ] `NewPool` accepts `devTasks, releaseTasks []Task` as separate parameters
- [ ] Real-time packages (`!pkg.IsRelease`) run `devTasks`; release packages run `releaseTasks`
- [ ] Release packages: `hub.Notify` and `emitBuildEvents` are NOT called in `ProcessOnce`
- [ ] Auto-remove: `pkg.RollupState == model.RollupPublished && pkg.IsContainer != nil`
- [ ] `PublishStateTask` promotes `pkg.RollupState` to `model.RollupPublished` when all non-skipped targets are published
- [ ] `BinariesCheckTask` calls `client.RepoPublishStates`, sets `Target.Published`, promotes rollup to `RollupPublished` when all non-skipped targets are published
- [ ] `go test ./internal/worker/ -v` passes

**Verify:** `cd backend && go test ./internal/worker/ -v && go build ./...`

**Steps:**

- [ ] **Step 1: Update Pool struct and NewPool signature**

In `backend/internal/worker/worker.go`, replace the existing struct and constructor:

```go
type Pool struct {
    size         int
    devTasks     []Task
    releaseTasks []Task
    client       *obs.Client
    db           *sql.DB
    hub          *hubpkg.Hub
    ws           *workingset.WorkingSet
}

func NewPool(size int, devTasks, releaseTasks []Task, client *obs.Client, db *sql.DB, hub *hubpkg.Hub, ws *workingset.WorkingSet) *Pool {
    return &Pool{size: size, devTasks: devTasks, releaseTasks: releaseTasks,
        client: client, db: db, hub: hub, ws: ws}
}
```

- [ ] **Step 2: Update ProcessOnce**

Replace `ProcessOnce` with:

```go
func (p *Pool) ProcessOnce(ctx context.Context, pkg *model.Package) {
    now := time.Now().UTC()

    // Snapshot target state before task chain.
    oldTargets := make([]model.Target, len(pkg.Targets))
    for i, t := range pkg.Targets {
        c := t
        if len(t.BuildReasonPackages) > 0 {
            c.BuildReasonPackages = append([]string(nil), t.BuildReasonPackages...)
        }
        oldTargets[i] = c
    }

    tasks := p.devTasks
    if pkg.IsRelease {
        tasks = p.releaseTasks
    }
    for _, t := range tasks {
        if err := t.Run(ctx, p.client, pkg); err != nil {
            slog.Warn("worker: task error",
                "task", fmt.Sprintf("%T", t),
                "pkg", pkg.Project+"/"+pkg.Name,
                "err", err)
        }
    }

    if err := store.UpsertPackageState(p.db, pkg, now); err != nil {
        slog.Error("worker: upsert package state", "pkg", pkg.Project+"/"+pkg.Name, "err", err)
    }

    if !pkg.IsRelease {
        p.hub.Notify(hubpkg.PackageUpdate(pkg))
        p.emitBuildEvents(pkg, oldTargets)
    }

    if pkg.RollupState == model.RollupPublished && pkg.IsContainer != nil {
        p.ws.Remove(pkg.Project + "/" + pkg.Name)
    }
}
```

Remove the now-unused `allTargetsPublished` helper function.

- [ ] **Step 3: Update PublishStateTask in obs/tasks.go**

Replace `PublishStateTask.Run` with:

```go
func (t PublishStateTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
    needsCheck := false
    for _, target := range pkg.Targets {
        if target.State == "succeeded" && !target.Published {
            needsCheck = true
            break
        }
    }
    if !needsCheck {
        return nil
    }

    states, err := client.RepoPublishStates(ctx, pkg.Project, pkg.Name)
    if err != nil {
        slog.Warn("obs: repo publish states", "pkg", pkg.Name, "err", err)
        return nil
    }

    for i, target := range pkg.Targets {
        if target.State == "succeeded" && !target.Published {
            if states[target.Repo+"/"+target.Arch] == "published" {
                pkg.Targets[i].Published = true
            }
        }
    }

    // Promote to published when all active (non-skipped) targets are published.
    allPublished := true
    activeCount := 0
    for _, target := range pkg.Targets {
        switch target.State {
        case "disabled", "excluded", "locked":
            continue
        }
        activeCount++
        if target.State != "succeeded" || !target.Published {
            allPublished = false
            break
        }
    }
    if allPublished && activeCount > 0 {
        pkg.RollupState = model.RollupPublished
    }
    return nil
}
```

- [ ] **Step 4: Add BinariesCheckTask in obs/tasks.go**

Add after `PublishStateTask`:

```go
// BinariesCheckTask is used for release packages. It calls RepoPublishStates
// to detect when all repos have published binaries, then promotes rollup to
// RollupPublished. Unlike PublishStateTask it does not require targets to be
// in "succeeded" state first — release packages use OBS repo state directly.
type BinariesCheckTask struct{}

func (t BinariesCheckTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
    states, err := client.RepoPublishStates(ctx, pkg.Project, pkg.Name)
    if err != nil {
        slog.Warn("obs: binaries check repo states", "pkg", pkg.Name, "err", err)
        return nil
    }

    for i, target := range pkg.Targets {
        if states[target.Repo+"/"+target.Arch] == "published" {
            pkg.Targets[i].Published = true
        }
    }

    // Promote to published when all active targets have binaries published.
    allPublished := true
    activeCount := 0
    for _, target := range pkg.Targets {
        switch target.State {
        case "disabled", "excluded", "locked":
            continue
        }
        activeCount++
        if !target.Published {
            allPublished = false
            break
        }
    }
    if allPublished && activeCount > 0 {
        pkg.RollupState = model.RollupPublished
    }
    return nil
}
```

- [ ] **Step 5: Fix worker tests**

`NewPool` signature changed. In `backend/internal/worker/worker_test.go`:

```bash
grep -n "NewPool" backend/internal/worker/worker_test.go
```

Update all `NewPool(size, tasks, ...)` calls to `NewPool(size, tasks, nil, ...)` where the second slice is `releaseTasks` (nil is fine for tests that only test real-time packages). If any test tests release packages, add a concrete `releaseTasks` slice.

- [ ] **Step 6: Run tests**

```
cd backend && go test ./internal/worker/ -v
cd backend && go test ./internal/obs/ -run TestTasks -v
cd backend && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add backend/internal/obs/tasks.go backend/internal/worker/worker.go backend/internal/worker/worker_test.go
git commit -s -m "feat(worker): BinariesCheckTask, release pipeline split, published terminal state"
```

---

## Task 6: MQ Consumer Refactor

**Goal:** Thread `root` through the MQ consumer, replace the hardcoded `isv:percona` prefix filter, and suppress all non-delete messages for release projects.

**Files:**
- Modify: `backend/internal/mq/consumer.go`

**Acceptance Criteria:**
- [ ] `NewConsumer` signature includes `root string` as the last parameter
- [ ] The root-filter in `handle` uses `root` (not `"isv:percona"`)
- [ ] Release projects (`KindRelease`): `package.delete` and `project.delete` still cascade DB deletes + emit events; all other message types are silently dropped
- [ ] Non-release projects: existing behavior unchanged
- [ ] `go build ./...` clean (consumer_test.go updated if it calls NewConsumer)

**Verify:** `cd backend && go build ./...`

**Steps:**

- [ ] **Step 1: Add root to Consumer and update NewConsumer**

In `backend/internal/mq/consumer.go`:

```go
type Consumer struct {
    url       string
    db        *sql.DB
    hub       *hubpkg.Hub
    obsClient *obs.Client
    ws        *workingset.WorkingSet
    root      string  // NEW
}

func NewConsumer(url string, db *sql.DB, h *hubpkg.Hub, obsClient *obs.Client, ws *workingset.WorkingSet, root string) *Consumer {
    return &Consumer{url: url, db: db, hub: h, obsClient: obsClient, ws: ws, root: root}
}
```

- [ ] **Step 2: Update handle to use root and classify release projects**

In `handle`, replace the hardcoded root filter and add release suppression:

```go
func (c *Consumer) handle(ctx context.Context, msg amqp.Delivery) {
    var m mqMessage
    if err := json.Unmarshal(msg.Body, &m); err != nil {
        slog.Debug("mq: unparseable message", "err", err)
        return
    }

    // Filter: only process projects under our root.
    if !strings.HasPrefix(m.Project, c.root+":") {
        return
    }

    var payload any
    if err := json.Unmarshal(msg.Body, &payload); err != nil {
        slog.Debug("mq: received raw message", "key", msg.RoutingKey, "message", string(msg.Body), "err", err)
    } else {
        slog.Debug("mq: received raw message", "key", msg.RoutingKey, "payload", payload)
    }

    kind := obs.Classify(c.root, m.Project)

    key := msg.RoutingKey
    switch {
    case key == repoRouteKey:
        // For release projects, repo.published is irrelevant (BinariesCheckTask handles it).
        if kind == obs.KindRelease {
            return
        }
        finished, err := store.GetFinishedPackagesByProject(c.db, m.Project)
        if err != nil {
            slog.Warn("mq: get finished packages for publish signal", "project", m.Project, "err", err)
        } else {
            for _, pkg := range finished {
                c.ws.Signal(pkg)
            }
        }

    case key == "opensuse.obs.project.create":
        if kind == obs.KindRelease {
            return // release project creation is handled by the poller
        }
        c.appendEvent(&model.Event{
            ID:      "evt_" + ulid.Make().String(),
            Type:    model.EventCreated,
            Scope:   kind.EventScope(),
            Project: m.Project,
            What:    fmt.Sprintf("project %s created", m.Project),
            Why:     m.Sender,
            URL:     fmt.Sprintf("https://build.opensuse.org/project/show/%s", m.Project),
            At:      time.Now().UTC(),
        })

    case key == "opensuse.obs.project.delete":
        scope := kind.EventScope()
        if err := store.DeletePackagesByProject(c.db, m.Project); err != nil {
            slog.Error("mq: delete packages for project", "project", m.Project, "err", err)
        }
        c.appendEvent(&model.Event{
            ID:      "evt_" + ulid.Make().String(),
            Type:    model.EventDeleted,
            Scope:   scope,
            Project: m.Project,
            What:    fmt.Sprintf("project %s deleted", m.Project),
            Why:     m.Comment,
            URL:     fmt.Sprintf("https://build.opensuse.org/project/show/%s", m.Project),
            At:      time.Now().UTC(),
        })

    case key == "opensuse.obs.package.create":
        if kind == obs.KindRelease {
            return // release packages are discovered by the poller
        }
        c.appendEvent(&model.Event{
            ID:      "evt_" + ulid.Make().String(),
            Type:    model.EventCreated,
            Scope:   kind.EventScope(),
            Project: m.Project,
            Package: m.Package,
            What:    fmt.Sprintf("package %s created", m.Package),
            Why:     m.Sender,
            URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", m.Project, m.Package),
            At:      time.Now().UTC(),
        })
        stub := &model.Package{
            Project: m.Project,
            Name:    m.Package,
            Scope:   kind.EventScope().AsScope(),
        }
        c.ws.Signal(stub)

    case key == "opensuse.obs.package.delete":
        if err := store.DeletePackage(c.db, m.Project, m.Package); err != nil {
            slog.Error("mq: delete package", "project", m.Project, "package", m.Package, "err", err)
        }
        c.appendEvent(&model.Event{
            ID:      "evt_" + ulid.Make().String(),
            Type:    model.EventDeleted,
            Scope:   kind.EventScope(),
            Project: m.Project,
            Package: m.Package,
            What:    fmt.Sprintf("package %s deleted", m.Package),
            Why:     m.Sender,
            URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", m.Project, m.Package),
            At:      time.Now().UTC(),
        })

    case isPackageBuildEvent(key):
        if kind == obs.KindRelease {
            return // release build events are ignored; poller owns release state
        }
        if key == "opensuse.obs.package.build_unchanged" {
            return
        }
        rollup := mqStateToRollup(key)
        pkg := c.mergePackageTarget(m, kind.EventScope().AsScope(), rollup)
        if err := c.upsertPackage(pkg); err != nil {
            slog.Error("mq: upsert package", "err", err)
            return
        }
        if pkg.RollupState != model.RollupSucceeded {
            c.ws.Signal(pkg)
        }
    }
}
```

- [ ] **Step 3: Add AsScope() helper on ProjectKind**

The MQ consumer calls `kind.EventScope().AsScope()` — but `EventScope()` already returns `model.Scope`. The existing `mergePackageTarget` takes `scope model.Scope`. Replace the `kind.EventScope().AsScope()` calls above with just `kind.EventScope()` since `EventScope()` returns `model.Scope` directly:

Correction — in Step 2 above, replace all `kind.EventScope().AsScope()` with `kind.EventScope()` since `EventScope() model.Scope` is already the right type. `inferScopeFromProject` and `inferScopeFromProject` are now unused.

- [ ] **Step 4: Remove inferScopeFromProject**

Remove the `inferScopeFromProject` function at the bottom of `consumer.go` (it is now replaced by `kind.EventScope()`). Also remove `mergePackageTarget`'s `scope` parameter since it's now derived inside:

Update `mergePackageTarget` to take the scope from the package kind:

```go
func (c *Consumer) mergePackageTarget(m mqMessage, scope model.Scope, newState model.RollupState) *model.Package {
    // ... existing implementation unchanged, scope is still a parameter
}
```

`inferScopeFromProject` can be deleted since it's no longer called anywhere in consumer.go.

- [ ] **Step 5: Verify compilation**

```
cd backend && go build ./...
```

If `obs.KindRelease` is not exported or `obs.Classify` import fails, check the import path is `"github.com/percona/obs-dashboard/internal/obs"` which is already imported in consumer.go.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/mq/consumer.go
git commit -s -m "feat(mq): thread root, suppress non-delete release messages"
```

---

## Task 7: API Handlers + Constructor Wiring

**Goal:** Swap the handlers to use `QueryBuildPackages` and `QueryReleasePackages`, accept `root` in `NewRouter`, and wire `cfg.OBSRoot` through all three constructors in `main.go`.

**Files:**
- Modify: `backend/internal/api/server.go`
- Modify: `backend/internal/api/handlers.go`
- Modify: `backend/cmd/obsboard/main.go`

**Acceptance Criteria:**
- [ ] `NewRouter` signature is `NewRouter(db, h, obsClient, root string) http.Handler`
- [ ] `packagesHandler` calls `store.QueryBuildPackages(db, root, product, version)`; no longer appends `isv:percona:common` or `isv:common` manually
- [ ] `releasesPackagesHandler` calls `store.QueryReleasePackages(db, root+":ppg:releases")`; no longer hits OBS live
- [ ] `releasesReposHandler` calls `store.QueryDistinctRepos(db, root+":ppg:releases:"+version)`; no longer hits OBS live
- [ ] `main.go` passes `cfg.OBSRoot` to `obs.NewPoller`, `mq.NewConsumer`, `api.NewRouter`
- [ ] `go test ./...` passes
- [ ] Application starts without error and `/api/products/ppg/17/packages` returns packages from DB

**Verify:** `cd backend && go test ./... && go build ./...`

**Steps:**

- [ ] **Step 1: Update NewRouter to accept root**

In `backend/internal/api/server.go`:

```go
func NewRouter(db *sql.DB, h *hub.Hub, obsClient *obs.Client, root string) http.Handler {
    r := chi.NewRouter()
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    r.Route("/api/products/{product}/{version}", func(r chi.Router) {
        r.Get("/packages", packagesHandler(db, root))
        r.Get("/events", eventsHandler(db))
        r.Get("/repos", reposHandler(db))
    })

    r.Route("/api/releases/ppg/{version}", func(r chi.Router) {
        r.Get("/packages", releasesPackagesHandler(db, root))
        r.Get("/repos",    releasesReposHandler(db, root))
    })

    r.Get("/api/pr/packages", prPackagesHandler(db))

    r.Route("/api/pr/{pr}/{subproject}/{version}", func(r chi.Router) {
        r.Get("/packages", prContextPackagesHandler(db))
        r.Get("/events",   prContextEventsHandler(db))
        r.Get("/repos",    prReposHandler(db))
    })

    r.Get("/api/stream", streamHandler(h))
    r.Get("/api/binaries", binariesHandler(obsClient))

    return r
}
```

- [ ] **Step 2: Update packagesHandler**

Replace the current `packagesHandler` implementation:

```go
func packagesHandler(db *sql.DB, root string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        product := chi.URLParam(r, "product")
        version := chi.URLParam(r, "version")

        pkgs, err := store.QueryBuildPackages(db, root, product, version)
        if err != nil {
            http.Error(w, "internal server error", http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        if err := json.NewEncoder(w).Encode(pkgs); err != nil {
            return
        }
    }
}
```

Note: `version` is now read from the URL param (it was previously ignored, relying on the prefix query).

- [ ] **Step 3: Update releasesPackagesHandler to serve from DB**

Replace the existing live-OBS `releasesPackagesHandler`:

```go
func releasesPackagesHandler(db *sql.DB, root string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        prefix := root + ":ppg:releases"
        pkgs, err := store.QueryReleasePackages(db, prefix)
        if err != nil {
            http.Error(w, "internal server error", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(pkgs)
    }
}
```

- [ ] **Step 4: Update releasesReposHandler to serve from DB**

Replace the existing live-OBS `releasesReposHandler`:

```go
func releasesReposHandler(db *sql.DB, root string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        version := chi.URLParam(r, "version")
        prefix := root + ":ppg:releases:" + version
        repos, err := store.QueryDistinctRepos(db, prefix)
        if err != nil {
            http.Error(w, "internal server error", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{"repos": repos})
    }
}
```

- [ ] **Step 5: Wire OBSRoot in main.go**

In `backend/cmd/obsboard/main.go`, update the three constructor calls:

```go
poller := obs.NewPoller(obsClient, db, cfg.Poller.Interval, h, ws, cfg.OBSRoot)
consumer := mq.NewConsumer(cfg.MQ.URL, db, h, obsClient, ws, cfg.OBSRoot)
// ...
router := api.NewRouter(db, h, obsClient, cfg.OBSRoot)
```

Also update the tasks slice to pass separate `devTasks` and `releaseTasks`:

```go
devTasks := []worker.Task{
    obs.PackageTypeTask{},
    obs.BuildStateTask{},
    obs.PublishStateTask{},
    obs.VersionTask{},
    obs.ContainerTagsTask{},
    obs.BlockedReasonTask{},
    obs.BuildReasonTask{},
}
releaseTasks := []worker.Task{
    obs.PackageTypeTask{},
    obs.BinariesCheckTask{},
}
pool := worker.NewPool(cfg.WorkerPool.Size, devTasks, releaseTasks, obsClient, db, h, ws)
```

- [ ] **Step 6: Update handler tests**

In `backend/internal/api/handlers_test.go`, update any `NewRouter` calls:

```bash
grep -n "NewRouter" backend/internal/api/handlers_test.go
```

Add `"isv:percona"` as the last argument to each `NewRouter(...)` call found.

- [ ] **Step 7: Run full test suite**

```
cd backend && go test ./... -v 2>&1 | tail -40
cd backend && go build ./...
```

Expected: all tests pass, binary builds cleanly.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/api/server.go backend/internal/api/handlers.go backend/cmd/obsboard/main.go
git commit -s -m "feat(api,main): wire OBSRoot; serve builds and releases packages from DB"
```

---

## Self-review notes

| Spec requirement | Covered in task |
|-----------------|----------------|
| Configurable OBSRoot, default `isv:percona` | Task 1 |
| `isv:common` removed from poller roots | Task 4 (single root) |
| ProjectClassifier with 5 kinds + IsRealTime + EventScope | Task 1 |
| `tags` column replaces `scope` for packages | Tasks 2, 3 |
| `is_release` column for fast filtering | Tasks 2, 3 |
| `target_state_durations` table + recording | Tasks 2, 3 |
| `published` as terminal RollupState | Tasks 3, 5 |
| Release packages: no SSE broadcast, no build events | Tasks 4, 5 |
| Release packages: enter WS when rollup≠published or is_container=nil | Tasks 4, 5 |
| Release WS reset when target set changes on published pkg | Task 4 |
| `BinariesCheckTask` using RepoPublishStates | Task 5 |
| `PublishStateTask` promotes to `published` | Task 5 |
| MQ suppresses non-delete release messages | Task 6 |
| `QueryBuildPackages` with union of three namespaces | Task 3 |
| `QueryReleasePackages` filtered by `is_release=1` | Task 3 |
| Handlers serve from DB (no live OBS for releases) | Task 7 |
| Cascade deletes from target_state_durations | Task 3 |
| `Package.Scope` removal | **Not in this plan** — deferred to a follow-up cleanup after the Vue frontend is updated to use `tags` instead of `scope`. |
