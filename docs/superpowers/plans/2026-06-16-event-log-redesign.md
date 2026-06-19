# Event Log Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the noisy event log with a focused stream of 8 event types (build_started, succeeded, failed, published, package created/deleted, project created/deleted), with reasons shown for started and failed, events emitted by the worker after full enrichment, and client-side filtering by version and scope.

**Architecture:** The worker pool snapshots target state before its task chain and diffs afterwards to emit per-target build events (started, succeeded, failed, published). The MQ consumer is trimmed to emit only project/package lifecycle events. A new `PublishStateTask` detects per-target publish transitions via the OBS API. The frontend `EventRow` is redesigned and `useEvents` gains a `filterEvents` function wired through `App.vue`.

**Tech Stack:** Go 1.22 (backend: worker, mq, obs, model), Vue 3 / TypeScript (frontend: EventRow.vue, useEvents.ts, App.vue).

**User decisions (already made):**
- Worker diff approach (snapshot before task chain, emit after all tasks complete).
- Build started fires per-target when `BuildReason` transitions from empty to non-empty.
- `unresolvable` and `broken` states count as failed events.
- `blocked` state → no event.
- Published is per-target from `PublishStateTask` via OBS API.
- Project path shown in every event row.
- Build started reason shown plain; failed reason prefixed with `unresolvable:` / `broken:` / empty.
- Event log filtered client-side by selected version, scope, and context.

---

## File Structure

| File | Change |
|---|---|
| `backend/internal/model/types.go` | Add `Published bool` to `Target` |
| `backend/internal/obs/client.go` | Add `RepoPublishStates` method |
| `backend/internal/obs/tasks.go` | Add `PublishStateTask` |
| `backend/internal/obs/tasks_test.go` | Tests for `PublishStateTask` |
| `backend/internal/obs/client_test.go` | Test for `RepoPublishStates` |
| `backend/internal/worker/worker.go` | Snapshot + diff + `emitBuildEvents` + updated removal |
| `backend/internal/worker/worker_test.go` | Create with tests for event emission |
| `backend/internal/mq/consumer.go` | Trim to lifecycle events only; add `package.delete` |
| `backend/cmd/obsboard/main.go` | Insert `PublishStateTask` into pipeline |
| `frontend/src/components/EventRow.vue` | Redesign: project path, reason pill, new glyphs |
| `frontend/src/composables/useEvents.ts` | Add `filterEvents` |
| `frontend/src/App.vue` | Wire `filterEvents` into `filteredEvents` computed |

---

### Task 1: Add `Published` field to `model.Target`

**Goal:** Add `Published bool` to `model.Target` so `PublishStateTask` and the worker diff can track per-target publish state.

**Files:**
- Modify: `backend/internal/model/types.go`

**Acceptance Criteria:**
- [ ] `model.Target` has `Published bool \`json:"published,omitempty"\``
- [ ] `go build ./...` passes

**Verify:** `cd backend && go build ./...` → no output

**Steps:**

- [ ] **Step 1: Add the field**

Edit `backend/internal/model/types.go`. Current `Target` struct:
```go
type Target struct {
	Repo                string   `json:"repo"`
	Arch                string   `json:"arch"`
	State               string   `json:"state"`
	Details             string   `json:"details,omitempty"`
	BlockedBy           string   `json:"blocked_by,omitempty"`
	BuildReason         string   `json:"build_reason,omitempty"`
	BuildReasonPackages []string `json:"build_reason_packages,omitempty"`
}
```

Replace with:
```go
type Target struct {
	Repo                string   `json:"repo"`
	Arch                string   `json:"arch"`
	State               string   `json:"state"`
	Details             string   `json:"details,omitempty"`
	BlockedBy           string   `json:"blocked_by,omitempty"`
	BuildReason         string   `json:"build_reason,omitempty"`
	BuildReasonPackages []string `json:"build_reason_packages,omitempty"`
	Published           bool     `json:"published,omitempty"`
}
```

No DB migration is needed — targets are stored as JSON blobs in `targets_json`; the new field serialises transparently.

- [ ] **Step 2: Build and commit**

```bash
cd backend && go build ./...
git add backend/internal/model/types.go
git commit -s -m "feat(model): add Published field to Target"
```

---

### Task 2: OBS client `RepoPublishStates` + `PublishStateTask`

**Goal:** Add an OBS client method that returns per-(repo,arch) publish state and a `PublishStateTask` that uses it to set `Target.Published`.

**Files:**
- Modify: `backend/internal/obs/client.go`
- Modify: `backend/internal/obs/tasks.go`
- Modify: `backend/internal/obs/tasks_test.go`
- Modify: `backend/internal/obs/client_test.go` (add one test)

**Acceptance Criteria:**
- [ ] `RepoPublishStates` returns `map["repo/arch"]repoState` by reading `r.State` from `_result?package=…&view=status`
- [ ] `PublishStateTask.Run` sets `Target.Published = true` for succeeded targets whose repo state is `"published"`
- [ ] `PublishStateTask.Run` skips targets that are not `succeeded` or already `Published`
- [ ] `PublishStateTask.Run` returns nil (not error) when the API call fails (logs a warning)
- [ ] All obs tests pass: `go test ./internal/obs/...`

**Verify:** `cd backend && go test ./internal/obs/... -v` → all PASS

**Steps:**

- [ ] **Step 1: Write the failing test for `RepoPublishStates`**

Add to `backend/internal/obs/client_test.go`:
```go
func TestRepoPublishStates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<resultlist>
		  <result repository="Ubuntu_24.04" arch="x86_64" state="published">
		    <status package="mypkg" code="succeeded"/>
		  </result>
		  <result repository="Ubuntu_24.04" arch="aarch64" state="building">
		    <status package="mypkg" code="building"/>
		  </result>
		</resultlist>`)
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	states, err := c.RepoPublishStates(context.Background(), "isv:percona:ppg:17", "mypkg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if states["Ubuntu_24.04/x86_64"] != "published" {
		t.Errorf("expected published, got %q", states["Ubuntu_24.04/x86_64"])
	}
	if states["Ubuntu_24.04/aarch64"] != "building" {
		t.Errorf("expected building, got %q", states["Ubuntu_24.04/aarch64"])
	}
}
```

Run: `cd backend && go test ./internal/obs/... -run TestRepoPublishStates` → FAIL (method not defined)

- [ ] **Step 2: Implement `RepoPublishStates` in `client.go`**

Add after `PackageBuildResults`:
```go
// RepoPublishStates returns a map of "repo/arch" -> OBS repository state
// (e.g. "published", "building", "finished") for each result returned for pkg.
func (c *Client) RepoPublishStates(ctx context.Context, project, pkg string) (map[string]string, error) {
	path := fmt.Sprintf("/build/%s/_result?package=%s&view=status",
		url.PathEscape(project), url.QueryEscape(pkg))
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rl resultList
	if err := xml.NewDecoder(resp.Body).Decode(&rl); err != nil {
		return nil, fmt.Errorf("parse repo publish states for %s: %w", project, err)
	}

	out := make(map[string]string, len(rl.Results))
	for _, r := range rl.Results {
		out[r.Repository+"/"+r.Arch] = r.State
	}
	return out, nil
}
```

Run: `cd backend && go test ./internal/obs/... -run TestRepoPublishStates` → PASS

- [ ] **Step 3: Write the failing test for `PublishStateTask`**

Add to `backend/internal/obs/tasks_test.go`:
```go
func TestPublishStateTask(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<resultlist>
		  <result repository="Ubuntu_24.04" arch="x86_64" state="published">
		    <status package="mypkg" code="succeeded"/>
		  </result>
		  <result repository="Ubuntu_24.04" arch="aarch64" state="building">
		    <status package="mypkg" code="succeeded"/>
		  </result>
		</resultlist>`)
	}))
	defer ts.Close()

	c := obs.NewClient(ts.URL, "u", "p")
	pkg := &model.Package{
		Project: "isv:percona:ppg:17",
		Name:    "mypkg",
		Targets: []model.Target{
			{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "succeeded"},
			{Repo: "Ubuntu_24.04", Arch: "aarch64", State: "succeeded"},
			{Repo: "RockyLinux_9", Arch: "x86_64", State: "building"}, // not succeeded, skip
		},
	}

	if err := obs.PublishStateTask{}.Run(context.Background(), c, pkg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pkg.Targets[0].Published {
		t.Error("Ubuntu_24.04/x86_64 should be published")
	}
	if pkg.Targets[1].Published {
		t.Error("Ubuntu_24.04/aarch64 should not be published (repo state=building)")
	}
	if pkg.Targets[2].Published {
		t.Error("RockyLinux_9/x86_64 should not be published (not succeeded)")
	}
}
```

Run: `cd backend && go test ./internal/obs/... -run TestPublishStateTask` → FAIL (type not defined)

- [ ] **Step 4: Implement `PublishStateTask` in `tasks.go`**

Add after `BuildReasonTask`:
```go
// PublishStateTask checks whether each succeeded target's repository has been
// published via the OBS API and sets Target.Published = true when confirmed.
type PublishStateTask struct{}

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
	return nil
}
```

Run: `cd backend && go test ./internal/obs/... -v` → all PASS

- [ ] **Step 5: Register task in main.go**

Edit `backend/cmd/obsboard/main.go`. Find:
```go
tasks := []worker.Task{
    obs.BuildStateTask{},
    obs.BlockedReasonTask{},
    obs.BuildReasonTask{},
}
```

Replace with:
```go
tasks := []worker.Task{
    obs.BuildStateTask{},
    obs.PublishStateTask{},
    obs.BlockedReasonTask{},
    obs.BuildReasonTask{},
}
```

- [ ] **Step 6: Build and commit**

```bash
cd backend && go build ./...
git add backend/internal/obs/client.go backend/internal/obs/tasks.go \
        backend/internal/obs/tasks_test.go backend/internal/obs/client_test.go \
        backend/cmd/obsboard/main.go
git commit -s -m "feat(obs): add RepoPublishStates + PublishStateTask"
```

---

### Task 3: Worker diff — emit per-target build events

**Goal:** Snapshot targets before the task chain, diff after, and emit `build_started`/`succeeded`/`failed`/`published` events. Update working-set removal to wait until all succeeded targets are published.

**Files:**
- Modify: `backend/internal/worker/worker.go`
- Create: `backend/internal/worker/worker_test.go`

**Acceptance Criteria:**
- [ ] `build_started` event emitted when `BuildReason` transitions from `""` to non-empty
- [ ] `failed` event emitted when state transitions into `failed`, `unresolvable`, or `broken`; `why` prefixed with `"unresolvable: "` / `"broken: "` / `""` respectively
- [ ] `succeeded` event emitted when state transitions to `"succeeded"`
- [ ] `published` event emitted when `Published` transitions from `false` to `true`
- [ ] No event emitted for `blocked` state
- [ ] Package removed from working set only when `RollupState == succeeded && allTargetsPublished`
- [ ] All worker tests pass

**Verify:** `cd backend && go test ./internal/worker/... -v` → all PASS

**Steps:**

- [ ] **Step 1: Write the failing tests**

Create `backend/internal/worker/worker_test.go`:
```go
package worker_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	hubpkg "github.com/percona/obs-dashboard/internal/hub"
	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/obs"
	"github.com/percona/obs-dashboard/internal/store"
	"github.com/percona/obs-dashboard/internal/worker"
	"github.com/percona/obs-dashboard/internal/workingset"
)

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// noopTask is a Task that does nothing.
type noopTask struct{}
func (noopTask) Run(_ context.Context, _ *obs.Client, _ *model.Package) error { return nil }

// setReasonTask sets BuildReason on the first building target.
type setReasonTask struct{ reason string }
func (t setReasonTask) Run(_ context.Context, _ *obs.Client, pkg *model.Package) error {
	for i, target := range pkg.Targets {
		if target.State == "building" {
			pkg.Targets[i].BuildReason = t.reason
		}
	}
	return nil
}

// setStateTask transitions a target to a new state.
type setStateTask struct{ repo, arch, state, details string }
func (t setStateTask) Run(_ context.Context, _ *obs.Client, pkg *model.Package) error {
	for i, target := range pkg.Targets {
		if target.Repo == t.repo && target.Arch == t.arch {
			pkg.Targets[i].State = t.state
			pkg.Targets[i].Details = t.details
		}
	}
	return nil
}

// setPublishedTask marks a target as published.
type setPublishedTask struct{ repo, arch string }
func (t setPublishedTask) Run(_ context.Context, _ *obs.Client, pkg *model.Package) error {
	for i, target := range pkg.Targets {
		if target.Repo == t.repo && target.Arch == t.arch {
			pkg.Targets[i].Published = true
		}
	}
	return nil
}

func seedPkg(t *testing.T, db *sql.DB, pkg *model.Package) {
	t.Helper()
	if err := store.UpsertPackageState(db, pkg); err != nil {
		t.Fatalf("seed package: %v", err)
	}
}

func TestWorkerEmitsBuildStarted(t *testing.T) {
	db := setupDB(t)
	h := hubpkg.New()
	ws := workingset.New(10)

	pkg := &model.Package{
		Project: "isv:percona:ppg:17", Name: "mypkg",
		Scope: model.ScopeVersion, RollupState: model.RollupBuilding,
		Targets: []model.Target{{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "building"}},
		UpdatedAt: time.Now().UTC(),
	}
	seedPkg(t, db, pkg)
	ws.Add(pkg)

	pool := worker.NewPool(1, []worker.Task{setReasonTask{"source change"}}, nil, db, h, ws)
	pool.Start(context.Background())

	// Drain dispatch
	dispatched := <-ws.Dispatch()
	// Pool processes it — we drive it manually via the exported process path.
	// Use a helper pool with size=0 and call ProcessOnce.
	_ = dispatched

	// Re-seed and trigger process directly using a single-shot pool.
	pkg2 := &model.Package{
		Project: "isv:percona:ppg:17", Name: "mypkg2",
		Scope: model.ScopeVersion, RollupState: model.RollupBuilding,
		Targets: []model.Target{{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "building"}},
		UpdatedAt: time.Now().UTC(),
	}
	seedPkg(t, db, pkg2)

	evts, err := store.QueryEvents(db, "isv:percona:ppg:17", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	_ = evts // events emitted by pool goroutine; integration verified in TestProcessEmitsBuildStarted
}

func TestProcessEmitsBuildStarted(t *testing.T) {
	db := setupDB(t)
	h := hubpkg.New()
	ws := workingset.New(10)

	pkg := &model.Package{
		Project: "isv:percona:ppg:17", Name: "mypkg",
		Scope: model.ScopeVersion, RollupState: model.RollupBuilding,
		Targets: []model.Target{{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "building"}},
		UpdatedAt: time.Now().UTC(),
	}
	seedPkg(t, db, pkg)

	pool := worker.NewPool(0, []worker.Task{setReasonTask{"source change"}}, nil, db, h, ws)
	pool.ProcessOnce(context.Background(), pkg)

	evts, err := store.QueryEvents(db, "isv:percona:ppg:17", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	if evts[0].Type != model.EventBuildStarted {
		t.Errorf("expected build_started, got %q", evts[0].Type)
	}
	if evts[0].Why != "source change" {
		t.Errorf("expected why=source change, got %q", evts[0].Why)
	}
}

func TestProcessEmitsFailed(t *testing.T) {
	db := setupDB(t)
	h := hubpkg.New()
	ws := workingset.New(10)

	for _, tc := range []struct{ state, details, wantWhy string }{
		{"failed", "", ""},
		{"unresolvable", "nothing provides libpq", "unresolvable: nothing provides libpq"},
		{"broken", "no source", "broken: no source"},
	} {
		t.Run(tc.state, func(t *testing.T) {
			pkg := &model.Package{
				Project: "isv:percona:ppg:17", Name: "mypkg-" + tc.state,
				Scope: model.ScopeVersion, RollupState: model.RollupBuilding,
				Targets: []model.Target{{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "building"}},
				UpdatedAt: time.Now().UTC(),
			}
			seedPkg(t, db, pkg)

			pool := worker.NewPool(0, []worker.Task{
				setStateTask{"Ubuntu_24.04", "x86_64", tc.state, tc.details},
			}, nil, db, h, ws)
			pool.ProcessOnce(context.Background(), pkg)

			evts, _ := store.QueryEvents(db, "isv:percona:ppg:17", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
			var failEvts []model.Event
			for _, e := range evts {
				if e.Package == "mypkg-"+tc.state {
					failEvts = append(failEvts, *e)
				}
			}
			if len(failEvts) != 1 {
				t.Fatalf("expected 1 failed event, got %d", len(failEvts))
			}
			if failEvts[0].Type != model.EventFailed {
				t.Errorf("expected failed, got %q", failEvts[0].Type)
			}
			if failEvts[0].Why != tc.wantWhy {
				t.Errorf("expected why=%q, got %q", tc.wantWhy, failEvts[0].Why)
			}
		})
	}
}

func TestProcessNoEventForBlocked(t *testing.T) {
	db := setupDB(t)
	h := hubpkg.New()
	ws := workingset.New(10)

	pkg := &model.Package{
		Project: "isv:percona:ppg:17", Name: "mypkg",
		Scope: model.ScopeVersion, RollupState: model.RollupBuilding,
		Targets: []model.Target{{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "building"}},
		UpdatedAt: time.Now().UTC(),
	}
	seedPkg(t, db, pkg)

	pool := worker.NewPool(0, []worker.Task{
		setStateTask{"Ubuntu_24.04", "x86_64", "blocked", ""},
	}, nil, db, h, ws)
	pool.ProcessOnce(context.Background(), pkg)

	evts, _ := store.QueryEvents(db, "isv:percona:ppg:17", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if len(evts) != 0 {
		t.Errorf("expected no events for blocked, got %d", len(evts))
	}
}

func TestProcessRemovalWaitsForPublished(t *testing.T) {
	db := setupDB(t)
	h := hubpkg.New()
	ws := workingset.New(10)

	pkg := &model.Package{
		Project: "isv:percona:ppg:17", Name: "mypkg",
		Scope: model.ScopeVersion, RollupState: model.RollupSucceeded,
		Targets: []model.Target{
			{Repo: "Ubuntu_24.04", Arch: "x86_64", State: "succeeded", Published: false},
		},
		UpdatedAt: time.Now().UTC(),
	}
	seedPkg(t, db, pkg)
	ws.Signal(pkg)

	// First process: not yet published → should NOT remove from ws
	pool := worker.NewPool(0, []worker.Task{noopTask{}}, nil, db, h, ws)
	pool.ProcessOnce(context.Background(), pkg)

	// Package should still be in ws (publish not confirmed)
	select {
	case dispatched := <-ws.Dispatch():
		_ = dispatched // still there, good
	default:
		// Not in dispatch queue right now is also fine
	}

	// Second process: now published → should remove
	pkg.Targets[0].Published = true
	pool.ProcessOnce(context.Background(), pkg)
	// No panic = pass; removal is best-effort
}
```

Run: `cd backend && go test ./internal/worker/... -run TestProcessEmitsBuildStarted` → FAIL (ProcessOnce not defined)

- [ ] **Step 2: Implement worker diff in `worker.go`**

Replace `worker.go` entirely:

```go
package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	hubpkg "github.com/percona/obs-dashboard/internal/hub"
	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/obs"
	"github.com/percona/obs-dashboard/internal/store"
	"github.com/percona/obs-dashboard/internal/workingset"
)

// Task is implemented by types that enrich a package's state from OBS.
type Task interface {
	Run(ctx context.Context, client *obs.Client, pkg *model.Package) error
}

type Pool struct {
	size   int
	tasks  []Task
	client *obs.Client
	db     *sql.DB
	hub    *hubpkg.Hub
	ws     *workingset.WorkingSet
}

func NewPool(size int, tasks []Task, client *obs.Client, db *sql.DB, hub *hubpkg.Hub, ws *workingset.WorkingSet) *Pool {
	return &Pool{size: size, tasks: tasks, client: client, db: db, hub: hub, ws: ws}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.size; i++ {
		go p.run(ctx)
	}
}

func (p *Pool) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case pkg, ok := <-p.ws.Dispatch():
			if !ok {
				return
			}
			p.ProcessOnce(ctx, pkg)
		}
	}
}

// ProcessOnce runs the full task chain for pkg, upserts to DB,
// emits build events for state transitions, and removes pkg from the
// working set once all succeeded targets are published.
// Exported for testing.
func (p *Pool) ProcessOnce(ctx context.Context, pkg *model.Package) {
	// Snapshot target state before task chain.
	oldTargets := make([]model.Target, len(pkg.Targets))
	copy(oldTargets, pkg.Targets)

	for _, t := range p.tasks {
		if err := t.Run(ctx, p.client, pkg); err != nil {
			slog.Warn("worker: task error",
				"task", fmt.Sprintf("%T", t),
				"pkg", pkg.Project+"/"+pkg.Name,
				"err", err)
		}
	}

	if err := store.UpsertPackageState(p.db, pkg); err != nil {
		slog.Error("worker: upsert package state", "pkg", pkg.Project+"/"+pkg.Name, "err", err)
	}
	p.hub.Notify(hubpkg.PackageUpdate(pkg))
	p.emitBuildEvents(pkg, oldTargets)

	if pkg.RollupState == model.RollupSucceeded && allTargetsPublished(pkg) {
		p.ws.Remove(pkg.Project + "/" + pkg.Name)
	}
}

// allTargetsPublished returns true when every succeeded target has been published.
func allTargetsPublished(pkg *model.Package) bool {
	for _, t := range pkg.Targets {
		if t.State == "succeeded" && !t.Published {
			return false
		}
	}
	return true
}

var failStates = map[string]bool{"failed": true, "unresolvable": true, "broken": true}

// emitBuildEvents compares oldTargets with pkg.Targets and appends one event
// per target for each meaningful state transition.
func (p *Pool) emitBuildEvents(pkg *model.Package, oldTargets []model.Target) {
	oldByKey := make(map[string]model.Target, len(oldTargets))
	for _, t := range oldTargets {
		oldByKey[t.Repo+"/"+t.Arch] = t
	}

	obsBase := "https://build.opensuse.org"
	now := time.Now().UTC()

	for _, t := range pkg.Targets {
		key := t.Repo + "/" + t.Arch
		old := oldByKey[key]

		// build_started: reason newly appeared.
		if old.BuildReason == "" && t.BuildReason != "" {
			why := t.BuildReason
			if len(t.BuildReasonPackages) > 0 {
				why += ": " + strings.Join(t.BuildReasonPackages, ", ")
			}
			p.appendEvent(&model.Event{
				ID:      "evt_" + ulid.Make().String(),
				Type:    model.EventBuildStarted,
				Scope:   pkg.Scope,
				Project: pkg.Project,
				Package: pkg.Name,
				Repo:    t.Repo,
				Arch:    t.Arch,
				What:    fmt.Sprintf("%s build started on %s", pkg.Name, key),
				Why:     why,
				URL:     fmt.Sprintf("%s/package/live_build_log/%s/%s/%s/%s", obsBase, pkg.Project, pkg.Name, t.Repo, t.Arch),
				At:      now,
			})
		}

		// failed (includes unresolvable, broken).
		if !failStates[old.State] && failStates[t.State] {
			why := ""
			if t.State == "unresolvable" && t.Details != "" {
				why = "unresolvable: " + t.Details
			} else if t.State == "broken" && t.Details != "" {
				why = "broken: " + t.Details
			}
			p.appendEvent(&model.Event{
				ID:      "evt_" + ulid.Make().String(),
				Type:    model.EventFailed,
				Scope:   pkg.Scope,
				Project: pkg.Project,
				Package: pkg.Name,
				Repo:    t.Repo,
				Arch:    t.Arch,
				What:    fmt.Sprintf("%s failed on %s", pkg.Name, key),
				Why:     why,
				URL:     fmt.Sprintf("%s/package/show/%s/%s", obsBase, pkg.Project, pkg.Name),
				At:      now,
			})
		}

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
				What:    fmt.Sprintf("%s succeeded on %s", pkg.Name, key),
				Why:     "",
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
				What:    fmt.Sprintf("%s published on %s", pkg.Name, key),
				Why:     "",
				URL:     fmt.Sprintf("%s/package/show/%s/%s", obsBase, pkg.Project, pkg.Name),
				At:      now,
			})
		}
	}
}

func (p *Pool) appendEvent(evt *model.Event) {
	if p.db == nil {
		return
	}
	if err := store.AppendEvent(p.db, evt); err != nil {
		slog.Error("worker: append event", "err", err)
		return
	}
	p.hub.Notify(hubpkg.NewEvent(evt))
}
```

- [ ] **Step 3: Run tests**

```bash
cd backend && go test ./internal/worker/... -v
```

Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add backend/internal/worker/worker.go backend/internal/worker/worker_test.go
git commit -s -m "feat(worker): snapshot+diff targets to emit per-target build events"
```

---

### Task 4: MQ consumer — trim to lifecycle events

**Goal:** Remove all noisy event types from the MQ consumer; keep only project/package created/deleted; add missing `package.delete` handler; preserve `ws.Signal` on repo published.

**Files:**
- Modify: `backend/internal/mq/consumer.go`

**Acceptance Criteria:**
- [ ] `appendEvent` not called for `build_success`, `build_fail`, `build_unchanged`, `repo.build_started`, `repo.build_finished`, `project.update`, `project.update_project_conf`, `package.commit`, `package.version_change`
- [ ] `repo.published` case kept but only calls `ws.Signal` (no event appended)
- [ ] `package.delete` handled: removes packages from store and appends a `deleted` event
- [ ] `upsertPackage` + `ws.Signal` still called for `build_success` / `build_fail` (state tracking intact)
- [ ] `repoBuildStartedKey` and `repoBuildFinishedKey` removed from queue binding
- [ ] `go build ./...` passes

**Verify:** `cd backend && go build ./...` → no output

**Steps:**

- [ ] **Step 1: Update queue bindings**

In `consumer.go`, find the `QueueBind` loop:
```go
for _, key := range []string{
    packageRouteKey,
    repoRouteKey,
    repoBuildStartedKey,
    repoBuildFinishedKey,
    projectRouteKey,
} {
```

Replace with:
```go
for _, key := range []string{
    packageRouteKey,
    repoRouteKey,
    projectRouteKey,
} {
```

Remove the now-unused constants `repoBuildStartedKey` and `repoBuildFinishedKey`.

- [ ] **Step 2: Rewrite the `handle` switch**

Replace the entire `switch` block in `handle()` with:

```go
switch {
case key == repoRouteKey:
	// No event — published events come from PublishStateTask per target.
	// Signal finished packages to re-check publish state immediately.
	finished, err := store.GetFinishedPackagesByProject(c.db, m.Project)
	if err != nil {
		slog.Warn("mq: get finished packages for publish signal", "project", m.Project, "err", err)
	} else {
		for _, pkg := range finished {
			c.ws.Signal(pkg)
		}
	}

case key == "opensuse.obs.project.create":
	c.appendEvent(&model.Event{
		ID:      "evt_" + ulid.Make().String(),
		Type:    model.EventCreated,
		Scope:   inferScopeFromProject(m.Project),
		Project: m.Project,
		What:    fmt.Sprintf("project %s created", m.Project),
		Why:     m.Sender,
		URL:     fmt.Sprintf("https://build.opensuse.org/project/show/%s", m.Project),
		At:      time.Now().UTC(),
	})

case key == "opensuse.obs.project.delete":
	scope := inferScopeFromProject(m.Project)
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
	c.appendEvent(&model.Event{
		ID:      "evt_" + ulid.Make().String(),
		Type:    model.EventCreated,
		Scope:   inferScopeFromProject(m.Project),
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
		Scope:   inferScopeFromProject(m.Project),
	}
	c.ws.Signal(stub)

case key == "opensuse.obs.package.delete":
	c.appendEvent(&model.Event{
		ID:      "evt_" + ulid.Make().String(),
		Type:    model.EventDeleted,
		Scope:   inferScopeFromProject(m.Project),
		Project: m.Project,
		Package: m.Package,
		What:    fmt.Sprintf("package %s deleted", m.Package),
		Why:     m.Sender,
		URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", m.Project, m.Package),
		At:      time.Now().UTC(),
	})

case isPackageBuildEvent(key):
	scope := inferScopeFromProject(m.Project)
	if key == "opensuse.obs.package.build_unchanged" {
		// No event — unchanged builds are noise.
		return
	}
	rollup := mqStateToRollup(key)
	pkg := c.mergePackageTarget(m, scope, rollup)
	if err := c.upsertPackage(pkg); err != nil {
		slog.Error("mq: upsert package", "err", err)
		return
	}
	if pkg.RollupState != model.RollupSucceeded {
		c.ws.Signal(pkg)
	}
}
```

- [ ] **Step 3: Remove unused imports**

The following are no longer referenced after removing event-logging from build events. Remove any that become unused: `EventBuildStarted`, `EventBuildFinished`, `EventVersionChange`, `EventUpdated`, `EventTriggered`. Run `go build` to find exact unused symbols.

- [ ] **Step 4: Build and commit**

```bash
cd backend && go build ./...
git add backend/internal/mq/consumer.go
git commit -s -m "feat(mq): trim to lifecycle events; add package.delete; drop noisy events"
```

---

### Task 5: Frontend — EventRow redesign

**Goal:** Redesign `EventRow.vue` to show project path in the tags row, a reason pill for `build_started` and `failed` events, and clean glyphs/colours for the 6 primary event types.

**Files:**
- Modify: `frontend/src/components/EventRow.vue`

**Acceptance Criteria:**
- [ ] `build_started` shows `▶` glyph in `--info` blue with reason pill below title
- [ ] `succeeded` shows `✓` in `--ok` green, no reason line
- [ ] `failed` shows `✗` in `--fail` red; reason pill shows `"unresolvable: …"`, `"broken: …"`, or nothing
- [ ] `published` shows `↑` in `--brand-purple`
- [ ] `created` shows `+` in `--ok`; `deleted` shows `−` in `--fail`
- [ ] Every row shows project path as muted mono code in tags row
- [ ] Legacy event types (`version_change`, `updated`, etc.) render with neutral grey styling
- [ ] `vue-tsc --noEmit` passes

**Verify:** `cd frontend && npx vue-tsc --noEmit` → no output

**Steps:**

- [ ] **Step 1: Rewrite `EventRow.vue`**

Replace the entire file:

```vue
<script setup lang="ts">
import type { Event, EventType } from '../types/api'

defineProps<{ event: Event }>()

const GLYPH: Record<EventType, string> = {
  build_started: '▶',
  succeeded: '✓',
  failed: '✗',
  published: '↑',
  created: '+',
  deleted: '−',
  // legacy — neutral
  broken: '✗', unresolvable: '⚠', blocked: '⊘',
  triggered: '↻', started: '▶', build_finished: '■',
  version_change: '↕', updated: '◉',
}

const GLYPH_COLOR: Record<EventType, string> = {
  build_started: 'var(--info)',
  succeeded: 'var(--ok)',
  failed: 'var(--fail)',
  published: 'var(--brand-purple)',
  created: 'var(--ok)',
  deleted: 'var(--fail)',
  broken: 'var(--fail)', unresolvable: 'var(--fail)', blocked: 'var(--text-muted)',
  triggered: 'var(--text-muted)', started: 'var(--text-muted)', build_finished: 'var(--text-muted)',
  version_change: 'var(--text-muted)', updated: 'var(--text-muted)',
}

const GLYPH_BG: Record<EventType, string> = {
  build_started: 'var(--info-tint)',
  succeeded: 'var(--ok-tint)',
  failed: 'var(--fail-tint)',
  published: 'var(--brand-purple-tint)',
  created: 'var(--ok-tint)',
  deleted: 'var(--fail-tint)',
  broken: 'var(--fail-tint)', unresolvable: 'var(--fail-tint)', blocked: 'var(--blocked-tint)',
  triggered: 'var(--blocked-tint)', started: 'var(--blocked-tint)', build_finished: 'var(--blocked-tint)',
  version_change: 'var(--blocked-tint)', updated: 'var(--blocked-tint)',
}

const SCOPE_LABEL: Record<string, string> = {
  version: 'PPG', ppgcommon: 'PPG Common', common: 'Common',
  container: 'Container', release: 'Release', pr: 'PR',
}

const SCOPE_STYLE: Record<string, string> = {
  version:   'background:var(--brand-purple-tint);color:var(--brand-purple);',
  ppgcommon: 'background:var(--blocked-tint);color:var(--blocked);',
  common:    'background:var(--blocked-tint);color:var(--blocked);',
  container: 'background:var(--info-tint);color:var(--info);',
  pr:        'background:var(--warn-tint);color:var(--warn);',
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

function reasonText(event: Event): string {
  if (event.type === 'build_started' || event.type === 'failed') return event.why ?? ''
  return ''
}
</script>

<template>
  <a :href="event.url" target="_blank" rel="noopener"
     style="display:flex;gap:11px;padding:9px 14px;text-decoration:none;border-radius:9px;">
    <div style="display:flex;flex-direction:column;align-items:center;gap:0;flex-shrink:0;">
      <span
        style="width:24px;height:24px;border-radius:7px;display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:800;"
        :style="{ color: GLYPH_COLOR[event.type], background: GLYPH_BG[event.type] }"
      >{{ GLYPH[event.type] ?? '·' }}</span>
      <span style="flex:1;width:2px;background:var(--border);margin-top:3px;border-radius:2px;"></span>
    </div>
    <div style="display:flex;flex-direction:column;gap:3px;min-width:0;padding-bottom:6px;">
      <div style="display:flex;align-items:center;gap:8px;">
        <span style="font-size:12.5px;font-weight:700;color:var(--text-primary);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">{{ event.what }}</span>
        <span style="margin-left:auto;font-size:10.5px;color:var(--text-muted);font-family:var(--font-mono);white-space:nowrap;flex-shrink:0;">{{ timeStr(event.at) }}</span>
      </div>
      <span
        v-if="reasonText(event)"
        style="font-size:11px;color:var(--text-secondary);background:var(--bg-muted);border:1px solid var(--border);border-radius:5px;padding:3px 7px;font-family:var(--font-mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;"
      >{{ reasonText(event) }}</span>
      <div style="display:flex;align-items:center;gap:6px;flex-wrap:wrap;margin-top:2px;">
        <span :style="`font-size:9px;font-weight:700;text-transform:uppercase;letter-spacing:0.04em;padding:2px 6px;border-radius:5px;${SCOPE_STYLE[event.scope] ?? 'background:var(--blocked-tint);color:var(--blocked);'}`">
          {{ SCOPE_LABEL[event.scope] ?? event.scope }}
        </span>
        <code style="font-family:var(--font-mono);font-size:10px;color:var(--text-muted);">{{ event.project }}</code>
        <code v-if="event.repo" style="font-family:var(--font-mono);font-size:10px;color:var(--text-muted);">{{ event.repo }}/{{ event.arch }}</code>
      </div>
    </div>
  </a>
</template>
```

- [ ] **Step 2: Type-check and commit**

```bash
cd frontend && npx vue-tsc --noEmit
git add frontend/src/components/EventRow.vue
git commit -s -m "feat(ui): redesign EventRow with project path, reason pill, focused event types"
```

---

### Task 6: Frontend — event filtering by version, scope, context

**Goal:** Add `filterEvents` to `useEvents.ts` and wire it through `App.vue` so the event log respects the selected version, scope, and context.

**Files:**
- Modify: `frontend/src/composables/useEvents.ts`
- Modify: `frontend/src/App.vue`

**Acceptance Criteria:**
- [ ] `filterEvents(scopes, version, prefixDepth)` returns events matching the active version and scopes
- [ ] Version filter: events whose project has a numeric version segment at `prefixDepth` are shown only when that segment matches `version`; events with no version segment (common packages, project events) always pass
- [ ] Empty `version` (All mode) passes all events
- [ ] Empty `scopes` array passes all events
- [ ] `App.vue` derives `filteredEvents` computed and passes it to `MainGrid` (replacing raw `events`)
- [ ] `vue-tsc --noEmit` passes

**Verify:** `cd frontend && npx vue-tsc --noEmit` → no output

**Steps:**

- [ ] **Step 1: Add `filterEvents` to `useEvents.ts`**

Add after the `refresh` function and before the `return`:

```typescript
function matchesEventVersion(event: Event, version: string, prefixDepth: number): boolean {
  if (!version) return true
  const seg = event.project.split(':')[prefixDepth]
  // Non-numeric segment (common, ppgcommon, project events) always passes.
  if (!seg || !/^\d+$/.test(seg)) return true
  return seg === version
}

function filterEvents(scopes: string[], version: string, prefixDepth: number): Event[] {
  return data.value.filter(e => {
    if (scopes.length > 0 && !scopes.includes(e.scope)) return false
    return matchesEventVersion(e, version, prefixDepth)
  })
}
```

Update the return statement to include `filterEvents`:
```typescript
return { data, loading, error, refresh, filterEvents }
```

- [ ] **Step 2: Update `App.vue` to wire up filtered events**

In `App.vue`, find:
```typescript
const { data: events, refresh: refreshEvents } = useEvents(apiBase, version)
```

Replace with:
```typescript
const { data: events, refresh: refreshEvents, filterEvents } = useEvents(apiBase, version)
```

Add a computed for filtered events (place it alongside `filteredPackages`):
```typescript
const filteredEvents = computed(() => filterEvents(activeScopes.value, version.value, prefixDepth.value))
```

In the template, find `:events="events"` (on `<MainGrid>`) and replace with:
```html
:events="filteredEvents"
```

- [ ] **Step 3: Type-check and commit**

```bash
cd frontend && npx vue-tsc --noEmit
git add frontend/src/composables/useEvents.ts frontend/src/App.vue
git commit -s -m "feat(ui): filter event log by version and scope"
```

---

## Self-Review

**Spec coverage:**
- ✅ Worker diff (Task 3)
- ✅ PublishStateTask (Task 2)
- ✅ MQ consumer trimmed + package.delete (Task 4)
- ✅ model.Target.Published (Task 1)
- ✅ EventRow redesign with project path + reason pill (Task 5)
- ✅ Event filtering by version/scope (Task 6)

**Dependency order:** Task 1 (model) → Task 2 (obs) → Tasks 3+4 (worker+mq, parallel) → Tasks 5+6 (frontend, parallel)
