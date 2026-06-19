# Package Version Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show RPM version and container image tag on each PackageCard and in succeeded/published event log entries, fetched from OBS and persisted in SQLite.

**Architecture:** Three new OBS worker tasks (`PackageTypeTask`, `VersionTask`, `ContainerTagsTask`) join the existing pipeline; they populate two new fields (`IsContainer`, `Version`) on `model.Package`, which are persisted by the existing upsert path and broadcast via the SSE hub. Events emitted for `succeeded` and `published` transitions carry the package's current `Version`. The frontend renders a version badge between the scope tag and project path in PackageCard and event rows.

**Tech Stack:** Go 1.25, SQLite via `modernc.org/sqlite`, Vue 3 Composition API + TypeScript.

**User decisions (already made):**
- Backend stores full `versrel` for RPMs (e.g. `17.5-1`); frontend strips release suffix for display (`17.5`).
- Container packages store only the tag portion of `tags[0]` (e.g. `18.4-1-1.7`); frontend prepends `Tag: `.
- Version badge placement: meta row between scope tag and project path (Option B).
- `version` on events: only `succeeded` and `published` event types.
- Fetching strategy: three tasks in the regular worker pipeline (Approach A).

---

## File Map

**Create:**
_(none)_

**Modify:**
- `backend/internal/model/types.go` — add `IsContainer bool`, `Version string` to Package; `Version string` to Event
- `backend/internal/store/db.go` — add new columns to schema + idempotent migrations
- `backend/internal/store/packages.go` — update upsert, scan, all three query SELECT lists
- `backend/internal/store/events.go` — update `AppendEvent`, `QueryEvents`
- `backend/internal/obs/client.go` — add `versrel` attr to `buildStatus`; add `getFile` helper; add 4 new methods
- `backend/internal/obs/tasks.go` — add `PackageTypeTask`, `VersionTask`, `ContainerTagsTask`
- `backend/cmd/obsboard/main.go` — prepend `PackageTypeTask{}`, append `VersionTask{}`, `ContainerTagsTask{}` to task slice
- `backend/internal/worker/worker.go` — populate `Version` field when emitting `succeeded`/`published` events
- `frontend/src/types/api.ts` — add `is_container?`, `version?` to Package; `version?` to Event
- `frontend/src/composables/useEventDisplay.ts` — add `displayVersion()`
- `frontend/src/components/PackageCard.vue` — version badge in meta row
- `frontend/src/components/EventRow.vue` — version badge in meta row
- `frontend/src/components/PackageEventGroup.vue` — version badge in header + child rows

**Tests (modify):**
- `backend/internal/store/packages_test.go`
- `backend/internal/store/events_test.go`
- `backend/internal/obs/client_test.go`
- `backend/internal/obs/tasks_test.go`

---

### Task 1: Backend model + store layer

**Goal:** Add `IsContainer`/`Version` to the Package model and `Version` to Event, persist them in SQLite, and update all query/scan paths.

**Files:**
- Modify: `backend/internal/model/types.go`
- Modify: `backend/internal/store/db.go`
- Modify: `backend/internal/store/packages.go`
- Modify: `backend/internal/store/events.go`
- Test: `backend/internal/store/packages_test.go`
- Test: `backend/internal/store/events_test.go`

**Acceptance Criteria:**
- [ ] `model.Package` has `IsContainer bool` and `Version string` fields with `json` tags.
- [ ] `model.Event` has `Version string` field with `json` tag.
- [ ] `store.Open` applies idempotent migrations for new columns; calling `Open` twice on the same DB does not error.
- [ ] `UpsertPackageState` + `QueryPackages` round-trips `IsContainer` and `Version` without data loss.
- [ ] `AppendEvent` + `QueryEvents` round-trips `Version`.
- [ ] `go test ./internal/store/...` passes.

**Verify:** `cd backend && go test ./internal/store/... -v` → all tests pass including `TestVersionRoundtrip` and `TestEventVersionRoundtrip`.

**Steps:**

- [ ] **Step 1: Add fields to model types**

In `backend/internal/model/types.go`, add two fields to `Package` and one to `Event`:

```go
type Package struct {
	Project        string      `json:"project"`
	Name           string      `json:"name"`
	Scope          Scope       `json:"scope"`
	RollupState    RollupState `json:"rollup_state"`
	OKTargets      int         `json:"ok_targets"`
	TotalTargets   int         `json:"total_targets"`
	IsContainer    bool        `json:"is_container"`
	Version        string      `json:"version,omitempty"`
	Trigger        *Trigger    `json:"trigger,omitempty"`
	Targets        []Target    `json:"targets"`
	UpdatedAt      time.Time   `json:"updated_at"`
	StateChangedAt *time.Time  `json:"state_changed_at,omitempty"`
}

type Event struct {
	ID      string    `json:"id"`
	Type    EventType `json:"type"`
	Scope   Scope     `json:"scope"`
	Project string    `json:"project"`
	Package string    `json:"package"`
	Repo    string    `json:"repo,omitempty"`
	Arch    string    `json:"arch,omitempty"`
	What    string    `json:"what"`
	Why     string    `json:"why"`
	Version string    `json:"version,omitempty"`
	URL     string    `json:"url"`
	At      time.Time `json:"at"`
}
```

- [ ] **Step 2: Write failing store tests**

Add to `backend/internal/store/packages_test.go`:

```go
func TestVersionRoundtrip(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	p := &model.Package{
		Project:     "isv:percona:ppg:17",
		Name:        "percona-pg_tde",
		Scope:       model.ScopeVersion,
		RollupState: model.RollupSucceeded,
		IsContainer: false,
		Version:     "17.5-1",
		Targets:     []model.Target{{Repo: "UBI_9", Arch: "x86_64", State: "succeeded"}},
		UpdatedAt:   now,
	}
	if err := UpsertPackageState(db, p, now); err != nil {
		t.Fatal(err)
	}
	pkgs, err := QueryPackages(db, "isv:percona")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0].Version != "17.5-1" {
		t.Errorf("Version: got %q, want %q", pkgs[0].Version, "17.5-1")
	}
	if pkgs[0].IsContainer {
		t.Error("IsContainer: expected false")
	}

	// Container package
	c := &model.Package{
		Project:     "isv:percona:ppg:17:containers",
		Name:        "percona-distribution-postgresql",
		Scope:       model.ScopeContainer,
		RollupState: model.RollupSucceeded,
		IsContainer: true,
		Version:     "18.4-1-1.7",
		Targets:     []model.Target{{Repo: "images", Arch: "x86_64", State: "succeeded"}},
		UpdatedAt:   now,
	}
	if err := UpsertPackageState(db, c, now); err != nil {
		t.Fatal(err)
	}
	pkgs2, err := QueryPackages(db, "isv:percona")
	if err != nil {
		t.Fatal(err)
	}
	var found *model.Package
	for _, p := range pkgs2 {
		if p.Name == "percona-distribution-postgresql" {
			found = p
		}
	}
	if found == nil {
		t.Fatal("container package not found")
	}
	if found.Version != "18.4-1-1.7" {
		t.Errorf("Version: got %q, want %q", found.Version, "18.4-1-1.7")
	}
	if !found.IsContainer {
		t.Error("IsContainer: expected true")
	}
}
```

Add to `backend/internal/store/events_test.go` (create file if missing):

```go
package store

import (
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

func TestEventVersionRoundtrip(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	evt := &model.Event{
		ID:      "evt_01JTEST",
		Type:    model.EventSucceeded,
		Scope:   model.ScopeVersion,
		Project: "isv:percona:ppg:17",
		Package: "percona-pg_tde",
		Repo:    "UBI_9",
		Arch:    "x86_64",
		What:    "percona-pg_tde succeeded",
		Why:     "",
		Version: "17.5-1",
		URL:     "https://build.opensuse.org/package/show/isv:percona:ppg:17/percona-pg_tde",
		At:      now,
	}
	if err := AppendEvent(db, evt); err != nil {
		t.Fatal(err)
	}
	evts, err := QueryEvents(db, "isv:percona", now.Add(-time.Second), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	if evts[0].Version != "17.5-1" {
		t.Errorf("Version: got %q, want %q", evts[0].Version, "17.5-1")
	}

	// Event without version (e.g. build_started)
	evt2 := &model.Event{
		ID:      "evt_02JTEST",
		Type:    model.EventBuildStarted,
		Scope:   model.ScopeVersion,
		Project: "isv:percona:ppg:17",
		Package: "percona-pg_tde",
		Repo:    "UBI_9",
		Arch:    "x86_64",
		What:    "percona-pg_tde build started",
		Why:     "source change",
		URL:     "https://build.opensuse.org/package/live_build_log/isv:percona:ppg:17/percona-pg_tde/UBI_9/x86_64",
		At:      now.Add(-time.Minute),
	}
	if err := AppendEvent(db, evt2); err != nil {
		t.Fatal(err)
	}
	evts2, err := QueryEvents(db, "isv:percona", now.Add(-2*time.Minute), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var foundBuildStarted bool
	for _, e := range evts2 {
		if e.Type == model.EventBuildStarted && e.Version != "" {
			t.Errorf("build_started event should have empty version, got %q", e.Version)
		}
		if e.Type == model.EventBuildStarted {
			foundBuildStarted = true
		}
	}
	if !foundBuildStarted {
		t.Error("build_started event not found")
	}
}
```

- [ ] **Step 3: Run tests, confirm they fail**

```bash
cd backend && go test ./internal/store/... -v -run "TestVersionRoundtrip|TestEventVersionRoundtrip"
```

Expected: compile error or FAIL (columns don't exist yet).

- [ ] **Step 4: Update `store/db.go` — schema + migrations**

Replace the `schema` const and `Open` function body:

```go
const schema = `
CREATE TABLE IF NOT EXISTS packages (
    project          TEXT NOT NULL,
    name             TEXT NOT NULL,
    scope            TEXT NOT NULL,
    rollup_state     TEXT NOT NULL,
    ok_targets       INTEGER NOT NULL DEFAULT 0,
    total_targets    INTEGER NOT NULL DEFAULT 0,
    trigger_what     TEXT,
    trigger_kind     TEXT,
    trigger_at       DATETIME,
    targets_json     TEXT NOT NULL DEFAULT '[]',
    updated_at       DATETIME NOT NULL,
    state_changed_at DATETIME,
    is_container     INTEGER NOT NULL DEFAULT 0,
    version          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project, name)
);

CREATE TABLE IF NOT EXISTS events (
    id       TEXT PRIMARY KEY,
    type     TEXT NOT NULL,
    scope    TEXT NOT NULL,
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
`

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	// Idempotent additive migrations for existing databases.
	db.Exec(`ALTER TABLE packages ADD COLUMN state_changed_at DATETIME`)
	db.Exec(`ALTER TABLE packages ADD COLUMN is_container INTEGER NOT NULL DEFAULT 0`)
	db.Exec(`ALTER TABLE packages ADD COLUMN version TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE events ADD COLUMN version TEXT NOT NULL DEFAULT ''`)
	return db, nil
}
```

- [ ] **Step 5: Update `store/packages.go`**

Replace the file content entirely:

```go
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

// UpsertPackageState inserts or replaces a package row.
func UpsertPackageState(db *sql.DB, p *model.Package, now time.Time) error {
	targetsJSON, err := json.Marshal(p.Targets)
	if err != nil {
		return err
	}
	var trigWhat, trigKind sql.NullString
	var trigAt sql.NullTime
	if p.Trigger != nil {
		trigWhat = sql.NullString{String: p.Trigger.What, Valid: true}
		trigKind = sql.NullString{String: p.Trigger.Kind, Valid: true}
		trigAt = sql.NullTime{Time: p.Trigger.At, Valid: true}
	}
	isContainerInt := 0
	if p.IsContainer {
		isContainerInt = 1
	}
	_, err = db.Exec(`
		INSERT INTO packages
			(project, name, scope, rollup_state, ok_targets, total_targets,
			 trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
			 state_changed_at, is_container, version)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project, name) DO UPDATE SET
			scope=excluded.scope, rollup_state=excluded.rollup_state,
			ok_targets=excluded.ok_targets, total_targets=excluded.total_targets,
			trigger_what=excluded.trigger_what, trigger_kind=excluded.trigger_kind,
			trigger_at=excluded.trigger_at, targets_json=excluded.targets_json,
			updated_at=excluded.updated_at,
			is_container=excluded.is_container,
			version=excluded.version,
			state_changed_at = CASE
				WHEN excluded.rollup_state != rollup_state THEN excluded.state_changed_at
				WHEN state_changed_at IS NULL             THEN excluded.state_changed_at
				ELSE state_changed_at
			END`,
		p.Project, p.Name, string(p.Scope), string(p.RollupState),
		p.OKTargets, p.TotalTargets,
		trigWhat, trigKind, trigAt,
		string(targetsJSON), p.UpdatedAt,
		now,
		isContainerInt, p.Version,
	)
	return err
}

// DeletePackagesByProject removes all package rows for an exact project name.
func DeletePackagesByProject(db *sql.DB, project string) error {
	_, err := db.Exec(`DELETE FROM packages WHERE project = ?`, project)
	return err
}

// DeletePackage removes a single package row.
func DeletePackage(db *sql.DB, project, name string) error {
	_, err := db.Exec(`DELETE FROM packages WHERE project = ? AND name = ?`, project, name)
	return err
}

const packageSelectCols = `
	project, name, scope, rollup_state, ok_targets, total_targets,
	trigger_what, trigger_kind, trigger_at, targets_json, updated_at,
	state_changed_at, is_container, version`

func scanPackages(rows *sql.Rows) ([]*model.Package, error) {
	pkgs := make([]*model.Package, 0)
	for rows.Next() {
		p := &model.Package{}
		var trigWhat, trigKind sql.NullString
		var trigAt sql.NullTime
		var targetsJSON string
		var stateChangedAt sql.NullTime
		var isContainerInt int
		if err := rows.Scan(
			&p.Project, &p.Name, &p.Scope, &p.RollupState,
			&p.OKTargets, &p.TotalTargets,
			&trigWhat, &trigKind, &trigAt,
			&targetsJSON, &p.UpdatedAt,
			&stateChangedAt, &isContainerInt, &p.Version,
		); err != nil {
			return nil, err
		}
		p.IsContainer = isContainerInt != 0
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
		pkgs = append(pkgs, p)
	}
	return pkgs, rows.Err()
}

// QueryPackages returns all packages for a given OBS project prefix.
func QueryPackages(db *sql.DB, projectPrefix string) ([]*model.Package, error) {
	rows, err := db.Query(`SELECT`+packageSelectCols+`
		FROM packages WHERE project LIKE ? ORDER BY project, name`,
		projectPrefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPackages(rows)
}

// GetActivePackages returns all packages where rollup_state is not 'succeeded'.
func GetActivePackages(db *sql.DB) ([]*model.Package, error) {
	rows, err := db.Query(`SELECT`+packageSelectCols+`
		FROM packages WHERE rollup_state != 'succeeded' ORDER BY project, name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPackages(rows)
}

// GetFinishedPackagesByProject returns succeeded packages for a project.
func GetFinishedPackagesByProject(db *sql.DB, project string) ([]*model.Package, error) {
	rows, err := db.Query(`SELECT`+packageSelectCols+`
		FROM packages WHERE project = ? AND rollup_state = 'succeeded'`,
		project,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPackages(rows)
}
```

- [ ] **Step 6: Update `store/events.go`**

```go
package store

import (
	"database/sql"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

// AppendEvent inserts a new event row.
func AppendEvent(db *sql.DB, e *model.Event) error {
	_, err := db.Exec(`
		INSERT INTO events (id, type, scope, project, package, repo, arch, what, why, url, at, version)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, string(e.Type), string(e.Scope),
		e.Project, e.Package, nullStr(e.Repo), nullStr(e.Arch),
		e.What, e.Why, e.URL, e.At, e.Version,
	)
	return err
}

// QueryEvents returns events for a project prefix within [from, to], newest first.
func QueryEvents(db *sql.DB, projectPrefix string, from, to time.Time) ([]*model.Event, error) {
	rows, err := db.Query(`
		SELECT id, type, scope, project, package,
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
		if err := rows.Scan(
			&e.ID, &e.Type, &e.Scope, &e.Project, &e.Package,
			&e.Repo, &e.Arch, &e.What, &e.Why, &e.URL, &e.At, &e.Version,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// PruneEvents deletes events older than cutoff.
func PruneEvents(db *sql.DB, cutoff time.Time) error {
	_, err := db.Exec("DELETE FROM events WHERE at < ?", cutoff)
	return err
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
```

- [ ] **Step 7: Run tests, confirm they pass**

```bash
cd backend && go test ./internal/store/... -v
```

Expected: all tests pass, including `TestVersionRoundtrip` and `TestEventVersionRoundtrip`.

- [ ] **Step 8: Commit**

```bash
cd backend && git add internal/model/types.go internal/store/db.go internal/store/packages.go internal/store/events.go
git commit -s -m "feat(store): add IsContainer and Version fields to Package and Event"
```

---

### Task 2: OBS client methods

**Goal:** Add four OBS API methods for container detection, RPM versrel, and container image tag fetching, plus a `getFile` helper for binary artifact downloads.

**Files:**
- Modify: `backend/internal/obs/client.go`
- Test: `backend/internal/obs/client_test.go`

**Acceptance Criteria:**
- [ ] `PackageIsContainer` returns true when any source filename is `Dockerfile` or ends in `.kiwi`.
- [ ] `PackageVersionResult` returns the first non-empty `versrel` attribute from `_result?view=versrel`.
- [ ] `PackageContainerInfoFilename` returns the `.containerinfo` filename from the binary listing, or `""` if absent.
- [ ] `PackageContainerTags` fetches the containerinfo JSON and returns the tag portion of `tags[0]` (after the last `:`), or `""` if tags is empty.
- [ ] `go test ./internal/obs/... -v` passes.

**Verify:** `cd backend && go test ./internal/obs/... -v -run "TestPackageIsContainer|TestPackageVersionResult|TestPackageContainerInfoFilename|TestPackageContainerTags"` → all 4 tests pass.

**Steps:**

- [ ] **Step 1: Write failing tests**

Add to `backend/internal/obs/client_test.go`:

```go
func TestPackageIsContainer(t *testing.T) {
	tests := []struct {
		name     string
		xml      string
		expected bool
	}{
		{
			name: "dockerfile",
			xml:  `<sourceinfo><filename>Dockerfile</filename><filename>config.sh</filename></sourceinfo>`,
			expected: true,
		},
		{
			name: "kiwi",
			xml:  `<sourceinfo><filename>percona-distribution-postgresql.kiwi</filename></sourceinfo>`,
			expected: true,
		},
		{
			name: "rpm spec",
			xml:  `<sourceinfo><filename>percona-pg_tde.spec</filename></sourceinfo>`,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(tc.xml))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, "u", "p")
			got, err := c.PackageIsContainer(context.Background(), "isv:percona:ppg:17", "mypkg")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestPackageVersionResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("view") != "versrel" {
			http.Error(w, "bad view", http.StatusBadRequest)
			return
		}
		w.Write([]byte(`<resultlist>
			<result repository="UBI_9" arch="x86_64" state="published">
				<status package="percona-pg_tde" code="succeeded" versrel="17.5-1"/>
			</result>
			<result repository="UBI_9" arch="aarch64" state="published">
				<status package="percona-pg_tde" code="succeeded" versrel="17.5-1"/>
			</result>
		</resultlist>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	versrel, err := c.PackageVersionResult(context.Background(), "isv:percona:ppg:17", "percona-pg_tde")
	if err != nil {
		t.Fatal(err)
	}
	if versrel != "17.5-1" {
		t.Errorf("expected %q, got %q", "17.5-1", versrel)
	}
}

func TestPackageVersionResultEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<resultlist>
			<result repository="UBI_9" arch="x86_64" state="building">
				<status package="mypkg" code="building"/>
			</result>
		</resultlist>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	versrel, err := c.PackageVersionResult(context.Background(), "isv:percona:ppg:17", "mypkg")
	if err != nil {
		t.Fatal(err)
	}
	if versrel != "" {
		t.Errorf("expected empty string for unbuilt package, got %q", versrel)
	}
}

func TestPackageContainerInfoFilename(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<binarylist>
			<binary filename="percona-distribution-postgresql-18.4-1.x86_64-1.7.tar" size="305101312" mtime="1781559533"/>
			<binary filename="percona-distribution-postgresql.x86_64-1.7.containerinfo" size="1234" mtime="1781559533"/>
		</binarylist>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	filename, err := c.PackageContainerInfoFilename(context.Background(),
		"isv:percona:ppg:17:containers:ubi8", "images", "x86_64", "percona-distribution-postgresql")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "percona-distribution-postgresql.x86_64-1.7.containerinfo" {
		t.Errorf("unexpected filename: %q", filename)
	}
}

func TestPackageContainerInfoFilenameAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<binarylist></binarylist>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	filename, err := c.PackageContainerInfoFilename(context.Background(), "proj", "repo", "arch", "pkg")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "" {
		t.Errorf("expected empty, got %q", filename)
	}
}

func TestPackageContainerTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"version": "18.4-1",
			"tags": [
				"percona-distribution-postgresql:18.4-1-1.7",
				"percona-distribution-postgresql:18.4-1",
				"percona-distribution-postgresql:18.4",
				"percona-distribution-postgresql:18"
			]
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	tag, err := c.PackageContainerTags(context.Background(),
		"proj", "images", "x86_64", "pkg", "pkg.containerinfo")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "18.4-1-1.7" {
		t.Errorf("expected %q, got %q", "18.4-1-1.7", tag)
	}
}

func TestPackageContainerTagsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tags": []}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	tag, err := c.PackageContainerTags(context.Background(), "proj", "repo", "arch", "pkg", "file.containerinfo")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "" {
		t.Errorf("expected empty, got %q", tag)
	}
}
```

- [ ] **Step 2: Run tests, confirm they fail**

```bash
cd backend && go test ./internal/obs/... -v -run "TestPackageIsContainer|TestPackageVersionResult|TestPackageContainerInfoFilename|TestPackageContainerTags"
```

Expected: compile error (methods don't exist yet).

- [ ] **Step 3: Add `versrel` attribute to `buildStatus` struct and add `getFile` helper**

In `backend/internal/obs/client.go`, update `buildStatus` and add `getFile` after the existing `get` method:

Update `buildStatus`:
```go
type buildStatus struct {
	Package string `xml:"package,attr"`
	Code    string `xml:"code,attr"`
	Versrel string `xml:"versrel,attr"`
	Details string `xml:"details"`
}
```

Add after the `get` method (around line 48):
```go
// getFile fetches a binary artifact from OBS without setting an Accept header,
// so OBS serves the raw file content (e.g. JSON containerinfo files).
func (c *Client) getFile(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("OBS %s: %s — %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}
```

Also add `"encoding/json"` to the import block.

- [ ] **Step 4: Add the four new client methods**

Add at the end of `backend/internal/obs/client.go`:

```go
// PackageIsContainer returns true if the package's source contains a Dockerfile
// or a .kiwi file, indicating it produces a container image.
func (c *Client) PackageIsContainer(ctx context.Context, project, pkg string) (bool, error) {
	path := fmt.Sprintf("/source/%s/%s?view=info&repository=images",
		url.PathEscape(project), url.PathEscape(pkg))
	resp, err := c.get(ctx, path)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var info struct {
		Filenames []string `xml:"filename"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, fmt.Errorf("parse source info: %w", err)
	}
	for _, fn := range info.Filenames {
		if fn == "Dockerfile" || strings.HasSuffix(fn, ".kiwi") {
			return true, nil
		}
	}
	return false, nil
}

// PackageVersionResult returns the versrel string (e.g. "17.5-1") from the first
// successfully built target, or "" if the package has not been built yet.
func (c *Client) PackageVersionResult(ctx context.Context, project, pkg string) (string, error) {
	path := fmt.Sprintf("/build/%s/_result?view=versrel&package=%s",
		url.PathEscape(project), url.QueryEscape(pkg))
	resp, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var rl resultList
	if err := xml.NewDecoder(resp.Body).Decode(&rl); err != nil {
		return "", fmt.Errorf("parse versrel result: %w", err)
	}
	for _, r := range rl.Results {
		for _, s := range r.Statuses {
			if s.Versrel != "" {
				return s.Versrel, nil
			}
		}
	}
	return "", nil
}

// PackageContainerInfoFilename returns the filename of the .containerinfo binary
// artifact for the given package target, or "" if the build hasn't produced one yet.
func (c *Client) PackageContainerInfoFilename(ctx context.Context, project, repo, arch, pkg string) (string, error) {
	path := fmt.Sprintf("/build/%s/%s/%s/%s",
		url.PathEscape(project), url.PathEscape(repo),
		url.PathEscape(arch), url.PathEscape(pkg))
	resp, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var listing struct {
		Binaries []struct {
			Filename string `xml:"filename,attr"`
		} `xml:"binary"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return "", fmt.Errorf("parse binary list: %w", err)
	}
	for _, b := range listing.Binaries {
		if strings.HasSuffix(b.Filename, ".containerinfo") {
			return b.Filename, nil
		}
	}
	return "", nil
}

// PackageContainerTags fetches a .containerinfo JSON file and returns the tag
// portion of tags[0] (everything after the last ":"), e.g. "18.4-1-1.7".
// Returns "" if tags is empty.
func (c *Client) PackageContainerTags(ctx context.Context, project, repo, arch, pkg, filename string) (string, error) {
	path := fmt.Sprintf("/build/%s/%s/%s/%s/%s",
		url.PathEscape(project), url.PathEscape(repo),
		url.PathEscape(arch), url.PathEscape(pkg),
		url.PathEscape(filename))
	resp, err := c.getFile(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var info struct {
		Tags []string `json:"tags"`
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("parse containerinfo: %w", err)
	}
	if len(info.Tags) == 0 {
		return "", nil
	}
	tag := info.Tags[0]
	if idx := strings.LastIndex(tag, ":"); idx >= 0 {
		tag = tag[idx+1:]
	}
	return tag, nil
}
```

- [ ] **Step 5: Run tests, confirm they pass**

```bash
cd backend && go test ./internal/obs/... -v
```

Expected: all tests pass including the 6 new ones.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/obs/client.go backend/internal/obs/client_test.go
git commit -s -m "feat(obs): add container detection and version fetch client methods"
```

---

### Task 3: Worker tasks

**Goal:** Implement `PackageTypeTask`, `VersionTask`, and `ContainerTagsTask` in `obs/tasks.go`, each tested with a fake HTTP server.

**Files:**
- Modify: `backend/internal/obs/tasks.go`
- Test: `backend/internal/obs/tasks_test.go`

**Acceptance Criteria:**
- [ ] `PackageTypeTask` sets `pkg.IsContainer = true` when `PackageIsContainer` returns true; false otherwise; logs and ignores OBS errors (non-fatal).
- [ ] `VersionTask` sets `pkg.Version` for non-container packages when `PackageVersionResult` returns a non-empty string; skips container packages; skips if version unchanged.
- [ ] `ContainerTagsTask` sets `pkg.Version` for container packages with at least one target; skips non-container packages; skips if tag is empty or unchanged.
- [ ] `go test ./internal/obs/... -v` passes.

**Verify:** `cd backend && go test ./internal/obs/... -v -run "TestPackageTypeTask|TestVersionTask|TestContainerTagsTask"` → all tests pass.

**Steps:**

- [ ] **Step 1: Write failing tests**

Add to `backend/internal/obs/tasks_test.go`:

```go
func TestPackageTypeTask(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// source info endpoint — returns Dockerfile, making it a container
		fmt.Fprint(w, `<sourceinfo><filename>Dockerfile</filename></sourceinfo>`)
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project:     "isv:percona:ppg:17:containers",
		Name:        "percona-distribution-postgresql",
		Scope:       model.ScopeContainer,
		RollupState: model.RollupSucceeded,
		UpdatedAt:   time.Now().UTC(),
	}

	task := obs.PackageTypeTask{}
	if err := task.Run(context.Background(), c, pkg); err != nil {
		t.Fatal(err)
	}
	if !pkg.IsContainer {
		t.Error("expected IsContainer=true for Dockerfile package")
	}
}

func TestPackageTypeTaskRPM(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<sourceinfo><filename>percona-pg_tde.spec</filename></sourceinfo>`)
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project:     "isv:percona:ppg:17",
		Name:        "percona-pg_tde",
		Scope:       model.ScopeVersion,
		RollupState: model.RollupSucceeded,
		IsContainer: true, // should be reset to false
		UpdatedAt:   time.Now().UTC(),
	}

	task := obs.PackageTypeTask{}
	if err := task.Run(context.Background(), c, pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.IsContainer {
		t.Error("expected IsContainer=false for spec-only package")
	}
}

func TestVersionTask(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<resultlist>
			<result repository="UBI_9" arch="x86_64" state="published">
				<status package="percona-pg_tde" code="succeeded" versrel="17.5-1"/>
			</result>
		</resultlist>`)
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project:     "isv:percona:ppg:17",
		Name:        "percona-pg_tde",
		Scope:       model.ScopeVersion,
		RollupState: model.RollupSucceeded,
		IsContainer: false,
		UpdatedAt:   time.Now().UTC(),
	}

	task := obs.VersionTask{}
	if err := task.Run(context.Background(), c, pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != "17.5-1" {
		t.Errorf("expected %q, got %q", "17.5-1", pkg.Version)
	}
}

func TestVersionTaskSkipsContainers(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		fmt.Fprint(w, `<resultlist></resultlist>`)
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project:     "isv:percona",
		Name:        "mycontainer",
		IsContainer: true,
		UpdatedAt:   time.Now().UTC(),
	}

	task := obs.VersionTask{}
	if err := task.Run(context.Background(), c, pkg); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("VersionTask should not call OBS for container packages")
	}
}

func TestContainerTagsTask(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".containerinfo") {
			fmt.Fprint(w, `{"tags":["percona-distribution-postgresql:18.4-1-1.7","percona-distribution-postgresql:18.4-1"]}`)
		} else {
			// binary listing
			fmt.Fprint(w, `<binarylist>
				<binary filename="percona-distribution-postgresql.x86_64-1.7.containerinfo" size="1" mtime="1"/>
			</binarylist>`)
		}
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project:     "isv:percona:ppg:17:containers",
		Name:        "percona-distribution-postgresql",
		Scope:       model.ScopeContainer,
		RollupState: model.RollupSucceeded,
		IsContainer: true,
		Targets:     []model.Target{{Repo: "images", Arch: "x86_64", State: "succeeded"}},
		UpdatedAt:   time.Now().UTC(),
	}

	task := obs.ContainerTagsTask{}
	if err := task.Run(context.Background(), c, pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != "18.4-1-1.7" {
		t.Errorf("expected %q, got %q", "18.4-1-1.7", pkg.Version)
	}
}

func TestContainerTagsTaskSkipsNonContainers(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project:     "isv:percona",
		Name:        "mypkg",
		IsContainer: false,
		Targets:     []model.Target{{Repo: "UBI_9", Arch: "x86_64", State: "succeeded"}},
		UpdatedAt:   time.Now().UTC(),
	}

	task := obs.ContainerTagsTask{}
	if err := task.Run(context.Background(), c, pkg); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("ContainerTagsTask should not call OBS for non-container packages")
	}
}
```

Also add `"strings"` to the import block in `tasks_test.go` if not already present.

- [ ] **Step 2: Run failing tests**

```bash
cd backend && go test ./internal/obs/... -v -run "TestPackageTypeTask|TestVersionTask|TestContainerTagsTask"
```

Expected: compile error (tasks don't exist yet).

- [ ] **Step 3: Implement the three tasks**

Add to `backend/internal/obs/tasks.go`:

```go
// PackageTypeTask detects whether a package produces a container image by
// inspecting its source files. Sets pkg.IsContainer accordingly.
// Errors are logged and treated as non-fatal to preserve the existing value.
type PackageTypeTask struct{}

func (t PackageTypeTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
	isContainer, err := client.PackageIsContainer(ctx, pkg.Project, pkg.Name)
	if err != nil {
		slog.Warn("obs: package type detection", "pkg", pkg.Name, "err", err)
		return nil
	}
	pkg.IsContainer = isContainer
	return nil
}

// VersionTask fetches the latest versrel (e.g. "17.5-1") for RPM/DEB packages
// from the OBS _result?view=versrel endpoint. Skipped for container packages.
type VersionTask struct{}

func (t VersionTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
	if pkg.IsContainer {
		return nil
	}
	versrel, err := client.PackageVersionResult(ctx, pkg.Project, pkg.Name)
	if err != nil {
		slog.Warn("obs: version result", "pkg", pkg.Name, "err", err)
		return nil
	}
	if versrel == "" || versrel == pkg.Version {
		return nil
	}
	pkg.Version = versrel
	return nil
}

// ContainerTagsTask fetches the most specific image tag (e.g. "18.4-1-1.7")
// from the .containerinfo binary artifact. Skipped for non-container packages
// and packages with no targets.
type ContainerTagsTask struct{}

func (t ContainerTagsTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
	if !pkg.IsContainer || len(pkg.Targets) == 0 {
		return nil
	}
	target := pkg.Targets[0]
	filename, err := client.PackageContainerInfoFilename(ctx, pkg.Project, target.Repo, target.Arch, pkg.Name)
	if err != nil {
		slog.Warn("obs: container info filename", "pkg", pkg.Name, "err", err)
		return nil
	}
	if filename == "" {
		return nil
	}
	tag, err := client.PackageContainerTags(ctx, pkg.Project, target.Repo, target.Arch, pkg.Name, filename)
	if err != nil {
		slog.Warn("obs: container tags", "pkg", pkg.Name, "err", err)
		return nil
	}
	if tag == "" || tag == pkg.Version {
		return nil
	}
	pkg.Version = tag
	return nil
}
```

- [ ] **Step 4: Run tests, confirm they pass**

```bash
cd backend && go test ./internal/obs/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/obs/tasks.go backend/internal/obs/tasks_test.go
git commit -s -m "feat(obs): add PackageTypeTask, VersionTask, ContainerTagsTask"
```

---

### Task 4: Wire pipeline and emit version in events

**Goal:** Add the three new tasks to the worker pipeline in `main.go` and populate `Version` on `succeeded`/`published` events in `worker.go`.

**Files:**
- Modify: `backend/cmd/obsboard/main.go`
- Modify: `backend/internal/worker/worker.go`

**Acceptance Criteria:**
- [ ] `PackageTypeTask{}` is the first task in the pipeline slice.
- [ ] `VersionTask{}` and `ContainerTagsTask{}` appear after `PublishStateTask{}` in the pipeline.
- [ ] `emitBuildEvents` sets `Version: pkg.Version` on `succeeded` and `published` event structs.
- [ ] `go build ./...` compiles without error.
- [ ] `go test ./internal/worker/... -v` passes.

**Verify:** `cd backend && go build ./... && go test ./... ` → no errors.

**Steps:**

- [ ] **Step 1: Update `main.go` task slice**

In `backend/cmd/obsboard/main.go`, replace the `tasks` slice:

```go
tasks := []worker.Task{
    obs.PackageTypeTask{},
    obs.BuildStateTask{},
    obs.PublishStateTask{},
    obs.VersionTask{},
    obs.ContainerTagsTask{},
    obs.BlockedReasonTask{},
    obs.BuildReasonTask{},
}
```

- [ ] **Step 2: Update `emitBuildEvents` in `worker.go` to populate Version**

In `backend/internal/worker/worker.go`, in `emitBuildEvents`, update the `succeeded` and `published` event structs to include `Version: pkg.Version`:

```go
// succeeded.
if old.State != "succeeded" && t.State == "succeeded" {
    p.appendEvent(&model.Event{
        ID:      "evt_" + ulid.Make().String(),
        Type:    model.EventSucceeded,
        Scope:   pkg.Scope,
        Project: pkg.Project,
        Package: pkg.Name,
        Repo:    t.Repo,
        Arch:    t.Arch,
        What:    fmt.Sprintf("%s succeeded", pkg.Name),
        Why:     "",
        Version: pkg.Version,
        URL:     fmt.Sprintf("%s/package/show/%s/%s", obsBase, pkg.Project, pkg.Name),
        At:      now,
    })
}

// published.
if !old.Published && t.Published {
    p.appendEvent(&model.Event{
        ID:      "evt_" + ulid.Make().String(),
        Type:    model.EventPublished,
        Scope:   pkg.Scope,
        Project: pkg.Project,
        Package: pkg.Name,
        Repo:    t.Repo,
        Arch:    t.Arch,
        What:    fmt.Sprintf("%s published", pkg.Name),
        Why:     "",
        Version: pkg.Version,
        URL:     fmt.Sprintf("%s/package/show/%s/%s", obsBase, pkg.Project, pkg.Name),
        At:      now,
    })
}
```

- [ ] **Step 3: Build and test**

```bash
cd backend && go build ./... && go test ./...
```

Expected: all packages compile, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add backend/cmd/obsboard/main.go backend/internal/worker/worker.go
git commit -s -m "feat(worker): wire version tasks into pipeline, emit version on succeeded/published events"
```

---

### Task 5: Frontend types and displayVersion helper

**Goal:** Add `is_container` and `version` to the TypeScript API types and add the `displayVersion` formatting helper to `useEventDisplay.ts`.

**Files:**
- Modify: `frontend/src/types/api.ts`
- Modify: `frontend/src/composables/useEventDisplay.ts`

**Acceptance Criteria:**
- [ ] `Package` interface has `is_container?: boolean` and `version?: string`.
- [ ] `Event` interface has `version?: string`.
- [ ] `displayVersion(version, isContainer)` returns `null` for empty version, `"Tag: X"` for containers, stripped version for RPMs.
- [ ] `cd frontend && npm run build` compiles without type errors.

**Verify:** `cd frontend && npm run build` → exits 0 with no TypeScript errors.

**Steps:**

- [ ] **Step 1: Update `src/types/api.ts`**

```ts
export type BuildState = 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'scheduled' | 'building' | 'finished'
export type PackageScope = 'common' | 'ppgcommon' | 'version' | 'container' | 'release' | 'pr'
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
}

export interface Package {
  project: string
  name: string
  scope: PackageScope
  rollup_state: BuildState
  ok_targets: number
  total_targets: number
  is_container?: boolean
  version?: string
  trigger?: Trigger
  targets: Target[]
  updated_at: string
  state_changed_at?: string
}

export interface PRGroup {
  pr: string
  rollup_state: BuildState
  packages: Package[]
}

export interface Event {
  id: string
  type: EventType
  scope: string
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

- [ ] **Step 2: Add `displayVersion` to `useEventDisplay.ts`**

Add this function at the end of `frontend/src/composables/useEventDisplay.ts`:

```ts
// Returns the formatted version string for display, or null if unavailable.
// Containers: "Tag: 18.4-1-1.7"; RPMs: strips release suffix "17.5-1" → "17.5".
export function displayVersion(version: string | undefined, isContainer: boolean): string | null {
  if (!version) return null
  if (isContainer) return 'Tag: ' + version
  return version.replace(/-[^-]+$/, '')
}
```

- [ ] **Step 3: Build**

```bash
cd frontend && npm run build
```

Expected: exits 0, no TypeScript errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/api.ts frontend/src/composables/useEventDisplay.ts
git commit -s -m "feat(frontend): add version fields to API types and displayVersion helper"
```

---

### Task 6: PackageCard version badge

**Goal:** Add a version badge between the scope tag and project path in `PackageCard.vue`.

**Files:**
- Modify: `frontend/src/components/PackageCard.vue`

**Acceptance Criteria:**
- [ ] When `pkg.version` is non-empty, a version badge appears between the scope tag and `pkg.project` in row 2.
- [ ] RPM badge: grey background (`var(--bg-muted)`) with monospace text (e.g. `17.5`).
- [ ] Container badge: purple background (`var(--brand-purple-tint)`, color `var(--brand-purple)`) with text `Tag: 18.4-1-1.7`.
- [ ] When `pkg.version` is absent, row 2 is unchanged.
- [ ] `cd frontend && npm run build` exits 0.

**Verify:** `cd frontend && npm run build` → exits 0. Manually open the dashboard and confirm version badges appear on packages with known versions.

**Steps:**

- [ ] **Step 1: Add `displayVersion` import to script block**

In `frontend/src/components/PackageCard.vue`, update the script imports:

```ts
import { computed, ref } from 'vue'
import type { Package, Target } from '../types/api'
import { displayVersion } from '../composables/useEventDisplay'
```

- [ ] **Step 2: Add a computed for the version display**

Add after the `obsUrl` computed:

```ts
const versionLabel = computed(() => displayVersion(props.pkg.version, props.pkg.is_container ?? false))
```

- [ ] **Step 3: Update Row 2 in the template**

Replace the current Row 2 comment block (scope tag + project path) with:

```html
<!-- Row 2: scope tag + version badge (if any) + project path -->
<div style="display: flex; align-items: center; gap: 7px;">
  <span style="font-size: 9.5px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.05em; padding: 2px 7px; border-radius: 5px; background: var(--blocked-tint); color: var(--blocked);">{{ SCOPE_LABEL[pkg.scope] ?? pkg.scope }}</span>
  <span
    v-if="versionLabel"
    :style="{
      fontFamily: 'var(--font-mono)',
      fontSize: '10px',
      fontWeight: '700',
      padding: '2px 7px',
      borderRadius: '5px',
      background: pkg.is_container ? 'var(--brand-purple-tint)' : 'var(--bg-muted, var(--blocked-tint))',
      color: pkg.is_container ? 'var(--brand-purple)' : 'var(--text-secondary)',
      border: '1px solid var(--border)',
      whiteSpace: 'nowrap',
      flexShrink: '0',
    }"
  >{{ versionLabel }}</span>
  <code style="font-family: var(--font-mono); font-size: 10.5px; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">{{ pkg.project }}</code>
</div>
```

- [ ] **Step 4: Build**

```bash
cd frontend && npm run build
```

Expected: exits 0, no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/PackageCard.vue
git commit -s -m "feat(ui): show version badge in PackageCard meta row"
```

---

### Task 7: EventRow and PackageEventGroup version badges

**Goal:** Add the version badge to event rows in `EventRow.vue` and to the header and child rows of `PackageEventGroup.vue`.

**Files:**
- Modify: `frontend/src/components/EventRow.vue`
- Modify: `frontend/src/components/PackageEventGroup.vue`

**Acceptance Criteria:**
- [ ] `EventRow` shows a version badge in its meta row when `event.version` is non-empty; `event.scope === 'container'` drives the purple/grey style.
- [ ] `PackageEventGroup` header meta row shows the version badge from `head.version`.
- [ ] Expanded child rows in `PackageEventGroup` each show their own `event.version` badge.
- [ ] Events without `version` render identically to before.
- [ ] `cd frontend && npm run build` exits 0.

**Verify:** `cd frontend && npm run build` → exits 0. Open the dashboard, filter by succeeded events, confirm version badges appear on succeeded and published rows.

**Steps:**

- [ ] **Step 1: Update `EventRow.vue` imports and add version badge to meta row**

The current meta row in `EventRow.vue` (at the bottom of the row's text block):

```html
<div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 2px;">
  <span :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${SCOPE_STYLE[props.event.scope] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`">{{ SCOPE_LABEL[props.event.scope] ?? props.event.scope }}</span>
  <code style="font-family:var(--font-mono);font-size:10px;color:var(--text-muted);">{{ props.event.project }}</code>
</div>
```

Replace with:

```html
<div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 2px;">
  <span :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${SCOPE_STYLE[props.event.scope] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`">{{ SCOPE_LABEL[props.event.scope] ?? props.event.scope }}</span>
  <span
    v-if="displayVersion(props.event.version, props.event.scope === 'container')"
    :style="{
      fontFamily: 'var(--font-mono)',
      fontSize: '10px',
      fontWeight: '700',
      padding: '2px 7px',
      borderRadius: '5px',
      background: props.event.scope === 'container' ? 'var(--brand-purple-tint)' : 'var(--bg-muted, var(--blocked-tint))',
      color: props.event.scope === 'container' ? 'var(--brand-purple)' : 'var(--text-secondary)',
      border: '1px solid var(--border)',
      whiteSpace: 'nowrap',
      flexShrink: '0',
    }"
  >{{ displayVersion(props.event.version, props.event.scope === 'container') }}</span>
  <code style="font-family:var(--font-mono);font-size:10px;color:var(--text-muted);">{{ props.event.project }}</code>
</div>
```

Update the script import in `EventRow.vue` to include `displayVersion`:

```ts
import { computed } from 'vue'
import type { Event } from '../types/api'
import { GLYPH, GLYPH_COLOR, GLYPH_BG, SCOPE_STYLE, SCOPE_LABEL, eventTitle, timeStr, showReason as _showReason, displayVersion } from '../composables/useEventDisplay'
```

- [ ] **Step 2: Update `PackageEventGroup.vue` — group header meta row**

Update the script import in `PackageEventGroup.vue` to include `displayVersion`:

```ts
import { computed } from 'vue'
import type { Event } from '../types/api'
import { GLYPH, GLYPH_COLOR, GLYPH_BG, SCOPE_STYLE, SCOPE_LABEL, eventTitle, timeStr, showReason, displayVersion } from '../composables/useEventDisplay'
```

The group header meta row (Row 3, scope chip + project path):

```html
<!-- Row 3: scope chip + project path -->
<div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 1px;">
  <span :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${SCOPE_STYLE[scope] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`">{{ SCOPE_LABEL[scope] ?? scope }}</span>
  <code style="font-family: var(--font-mono); font-size: 10px; color: var(--text-muted);">{{ project }}</code>
</div>
```

Replace with:

```html
<!-- Row 3: scope chip + version badge + project path -->
<div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-top: 1px;">
  <span :style="`font-size: 9px; font-weight: 700; text-transform: uppercase; letter-spacing: 0.04em; padding: 2px 6px; border-radius: 5px; ${SCOPE_STYLE[scope] ?? 'background: var(--blocked-tint); color: var(--blocked);'}`">{{ SCOPE_LABEL[scope] ?? scope }}</span>
  <span
    v-if="displayVersion(head.version, scope === 'container')"
    :style="{
      fontFamily: 'var(--font-mono)',
      fontSize: '10px',
      fontWeight: '700',
      padding: '2px 7px',
      borderRadius: '5px',
      background: scope === 'container' ? 'var(--brand-purple-tint)' : 'var(--bg-muted, var(--blocked-tint))',
      color: scope === 'container' ? 'var(--brand-purple)' : 'var(--text-secondary)',
      border: '1px solid var(--border)',
      whiteSpace: 'nowrap',
      flexShrink: '0',
    }"
  >{{ displayVersion(head.version, scope === 'container') }}</span>
  <code style="font-family: var(--font-mono); font-size: 10px; color: var(--text-muted);">{{ project }}</code>
</div>
```

- [ ] **Step 3: Update child rows in `PackageEventGroup.vue`**

In the expanded child event rows, the child text block currently has:

```html
<code v-if="event.repo" style="font-family: var(--font-mono); font-size: 11px; font-weight: 600; color: var(--text-secondary);">{{ event.repo }}/{{ event.arch }}</code>
```

Add a version badge after the `why` pill and before the `repo/arch` code line:

```html
<span
  v-if="showReason(event)"
  style="font-size:11px;color:var(--text-secondary);background:var(--bg-muted,var(--blocked-tint));border:1px solid var(--border);border-radius:5px;padding:3px 7px;font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"
>{{ event.why }}</span>
<span
  v-if="displayVersion(event.version, scope === 'container')"
  :style="{
    fontFamily: 'var(--font-mono)',
    fontSize: '10px',
    fontWeight: '700',
    padding: '2px 7px',
    borderRadius: '5px',
    background: scope === 'container' ? 'var(--brand-purple-tint)' : 'var(--bg-muted, var(--blocked-tint))',
    color: scope === 'container' ? 'var(--brand-purple)' : 'var(--text-secondary)',
    border: '1px solid var(--border)',
    whiteSpace: 'nowrap',
    alignSelf: 'flex-start',
  }"
>{{ displayVersion(event.version, scope === 'container') }}</span>
<code v-if="event.repo" style="font-family: var(--font-mono); font-size: 11px; font-weight: 600; color: var(--text-secondary);">{{ event.repo }}/{{ event.arch }}</code>
```

- [ ] **Step 4: Build**

```bash
cd frontend && npm run build
```

Expected: exits 0, no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/EventRow.vue frontend/src/components/PackageEventGroup.vue
git commit -s -m "feat(ui): show version badge in EventRow and PackageEventGroup"
```
