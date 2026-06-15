# Working Set Architecture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current monolithic polling model with a working set of actively-building packages and an async worker pool that enriches each package's state in a modular, extensible way.

**Architecture:** A `WorkingSet` (`internal/workingset/`) maintains an in-memory map of packages whose `rollup_state != succeeded`, seeded from the DB on startup. A fixed worker pool (`internal/worker/`) drains a dispatch channel running a registered list of `Task` implementations per package. Workers remove packages when they reach `succeeded`. The dispatch channel is fed by both an interval scheduler and immediate signals from the MQ consumer and poller. Three built-in tasks live in `internal/obs/tasks.go`: `BuildStateTask`, `BlockedReasonTask`, `BuildReasonTask`.

**Tech Stack:** Go standard library. No new dependencies, no new DB tables.

**User decisions (already made):**
- Broad poller keeps its discovery role; per-package detail polling moves to workers.
- Working set is derived from the existing DB on startup (no separate table); an index on `rollup_state` is added for the startup query.
- Worker pool size is configurable (`worker_pool.size`, default 5).
- Workers poll on a fixed interval (`worker_pool.poll_interval`, default 30s) AND are triggered immediately by MQ events — hybrid model.
- Dispatch channel is configurable (`worker_pool.queue_size`), default 512.
- `Task` is the interface name.
- `BuildReasonTask` fetches `_reason` for all non-succeeded targets (building, scheduled, blocked, failed).

---

## File Map

| File | Status | Responsibility |
|------|--------|----------------|
| `backend/internal/model/types.go` | Modify | Add `BuildReason`, `BuildReasonPackages` to `Target` |
| `backend/internal/config/config.go` | Modify | Add `WorkerPoolConfig` section |
| `backend/internal/store/db.go` | Modify | Add `rollup_state` index to schema |
| `backend/internal/store/packages.go` | Modify | Add `GetActivePackages()` |
| `backend/internal/workingset/workingset.go` | Create | Working set map, dispatch channel, scheduler |
| `backend/internal/workingset/workingset_test.go` | Create | Tests for WorkingSet |
| `backend/internal/worker/worker.go` | Create | `Task` interface, `Pool` struct |
| `backend/internal/worker/worker_test.go` | Create | Tests for Pool |
| `backend/internal/obs/client.go` | Modify | Add `PackageBuildResults`, `PackageBuildReason` |
| `backend/internal/obs/client_test.go` | Modify | Tests for new client methods |
| `backend/internal/obs/tasks.go` | Create | `BuildStateTask`, `BlockedReasonTask`, `BuildReasonTask` |
| `backend/internal/obs/tasks_test.go` | Create | Tests for all three tasks |
| `backend/internal/obs/poller.go` | Modify | Add `ws` field; call `ws.Add(pkg)`; remove `EnrichBlockedTargets` call |
| `backend/internal/mq/consumer.go` | Modify | Add `ws` field; call `ws.Signal(pkg)`; remove `EnrichBlockedTargets` call |
| `backend/cmd/obsboard/main.go` | Modify | Wire working set, worker pool, scheduler |
| `backend/config.yaml.example` | Modify | Add `worker_pool` section |

---

### Task 1: Data model and config additions

**Goal:** Add `BuildReason`/`BuildReasonPackages` to `Target`, add `WorkerPoolConfig` to config, and verify with tests.

**Files:**
- Modify: `backend/internal/model/types.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/config.yaml.example`

**Acceptance Criteria:**
- [ ] `Target` has `BuildReason string \`json:"build_reason,omitempty"\`` and `BuildReasonPackages []string \`json:"build_reason_packages,omitempty"\``
- [ ] `Config` has `WorkerPool WorkerPoolConfig` with `Size int`, `PollInterval time.Duration`, `QueueSize int`
- [ ] Defaults: size=5, poll_interval=30s, queue_size=512
- [ ] Env vars `WORKER_POOL_SIZE`, `WORKER_POOL_POLL_INTERVAL`, `WORKER_POOL_QUEUE_SIZE` are bound via viper
- [ ] `go build ./...` exits 0

**Verify:** `cd backend && go build ./...` → exits 0, no output

**Steps:**

- [ ] **Step 1: Extend Target in types.go**

  Read the current file first (`backend/internal/model/types.go`). The `Target` struct currently ends at `BlockedBy`. Add two fields:

  ```go
  type Target struct {
      Repo                string   `json:"repo"`
      Arch                string   `json:"arch"`
      State               string   `json:"state"`
      BlockedBy           string   `json:"blocked_by,omitempty"`
      BuildReason         string   `json:"build_reason,omitempty"`
      BuildReasonPackages []string `json:"build_reason_packages,omitempty"`
  }
  ```

- [ ] **Step 2: Add WorkerPoolConfig to config.go**

  Read `backend/internal/config/config.go`. The file uses viper with `mapstructure` tags and manual `viper.BindEnv` calls. The `Config` struct currently has `OBS`, `MQ`, `Poller`, `Store`, `Server` sections.

  Add the new type and field:

  ```go
  type WorkerPoolConfig struct {
      Size         int           `mapstructure:"size"`
      PollInterval time.Duration `mapstructure:"poll_interval"`
      QueueSize    int           `mapstructure:"queue_size"`
  }
  ```

  Add `WorkerPool WorkerPoolConfig` to the `Config` struct.

  In the `Load()` function, after the existing `viper.SetDefault` calls, add:

  ```go
  viper.SetDefault("worker_pool.size", 5)
  viper.SetDefault("worker_pool.poll_interval", 30*time.Second)
  viper.SetDefault("worker_pool.queue_size", 512)
  ```

  After the existing `viper.BindEnv` calls, add:

  ```go
  _ = viper.BindEnv("worker_pool.size", "WORKER_POOL_SIZE")
  _ = viper.BindEnv("worker_pool.poll_interval", "WORKER_POOL_POLL_INTERVAL")
  _ = viper.BindEnv("worker_pool.queue_size", "WORKER_POOL_QUEUE_SIZE")
  ```

- [ ] **Step 3: Update config.yaml.example**

  Read `backend/config.yaml.example`. Append the new section at the end:

  ```yaml
  worker_pool:
    size: 5
    poll_interval: 30s
    queue_size: 512
  ```

- [ ] **Step 4: Build check**

  ```bash
  cd backend && go build ./...
  ```
  Expected: exits 0, no output.

- [ ] **Step 5: Commit**

  ```bash
  cd backend && git add internal/model/types.go internal/config/config.go config.yaml.example
  git commit -s -m "feat: add BuildReason fields to Target and WorkerPoolConfig"
  ```

---

### Task 2: Store additions

**Goal:** Add `GetActivePackages()` to the store package and add a `rollup_state` index to the schema.

**Files:**
- Modify: `backend/internal/store/db.go`
- Modify: `backend/internal/store/packages.go`

**Acceptance Criteria:**
- [ ] `CREATE INDEX IF NOT EXISTS idx_packages_rollup_state ON packages(rollup_state)` is in the schema constant in `db.go`
- [ ] `GetActivePackages(db *sql.DB) ([]*model.Package, error)` exists in `packages.go`
- [ ] It queries `WHERE rollup_state != 'succeeded' ORDER BY project, name`
- [ ] The scan loop matches the existing `QueryPackages` scan pattern exactly (same columns, same JSON unmarshal for `targets_json`)
- [ ] `go test ./internal/store/... -v` exits 0

**Verify:** `cd backend && go test ./internal/store/... -v` → all PASS

**Steps:**

- [ ] **Step 1: Add rollup_state index to schema**

  Read `backend/internal/store/db.go`. The `schema` constant defines the CREATE TABLE statements. Append the index after the last `CREATE TABLE` statement:

  ```sql
  CREATE INDEX IF NOT EXISTS idx_packages_rollup_state ON packages(rollup_state);
  ```

- [ ] **Step 2: Write the failing test**

  Read `backend/internal/store/packages.go` to understand the scan loop pattern used by `QueryPackages`. Look at how it scans `targets_json` and unmarshals into `[]model.Target`.

  Create `backend/internal/store/packages_test.go` (or append to it if it exists). Add:

  ```go
  func TestGetActivePackages(t *testing.T) {
      db, err := Open(":memory:")
      if err != nil {
          t.Fatal(err)
      }
      defer db.Close()

      // Insert a succeeded package
      succeeded := &model.Package{
          Project: "isv:percona", Name: "pkg-ok", Scope: model.ScopeCommon,
          RollupState: model.RollupSucceeded, OKTargets: 1, TotalTargets: 1,
          Targets: []model.Target{{Repo: "repo", Arch: "x86_64", State: "succeeded"}},
          UpdatedAt: time.Now().UTC(),
      }
      if err := UpsertPackageState(db, succeeded); err != nil {
          t.Fatal(err)
      }

      // Insert a failing package
      failing := &model.Package{
          Project: "isv:percona", Name: "pkg-fail", Scope: model.ScopeCommon,
          RollupState: model.RollupFailed, OKTargets: 0, TotalTargets: 1,
          Targets: []model.Target{{Repo: "repo", Arch: "x86_64", State: "failed"}},
          UpdatedAt: time.Now().UTC(),
      }
      if err := UpsertPackageState(db, failing); err != nil {
          t.Fatal(err)
      }

      pkgs, err := GetActivePackages(db)
      if err != nil {
          t.Fatal(err)
      }
      if len(pkgs) != 1 {
          t.Fatalf("expected 1 active package, got %d", len(pkgs))
      }
      if pkgs[0].Name != "pkg-fail" {
          t.Errorf("expected pkg-fail, got %s", pkgs[0].Name)
      }
  }
  ```

  Run: `cd backend && go test ./internal/store/... -run TestGetActivePackages -v`
  Expected: FAIL with "undefined: GetActivePackages"

- [ ] **Step 3: Implement GetActivePackages**

  In `backend/internal/store/packages.go`, add after `QueryPackages`:

  ```go
  // GetActivePackages returns all packages with rollup_state != 'succeeded'.
  // Used to seed the working set on startup.
  func GetActivePackages(db *sql.DB) ([]*model.Package, error) {
      rows, err := db.Query(`
          SELECT project, name, scope, rollup_state, ok_targets, total_targets,
                 trigger_what, trigger_kind, trigger_at, targets_json, updated_at
          FROM packages WHERE rollup_state != 'succeeded' ORDER BY project, name`)
      if err != nil {
          return nil, err
      }
      defer rows.Close()
      return scanPackages(rows)
  }
  ```

  Then extract the scan loop from `QueryPackages` into a shared helper `scanPackages`. Read `QueryPackages` carefully — it has a scan loop that reads each row. Extract it:

  ```go
  func scanPackages(rows *sql.Rows) ([]*model.Package, error) {
      var pkgs []*model.Package
      for rows.Next() {
          var p model.Package
          var triggerWhat, triggerKind sql.NullString
          var triggerAt sql.NullTime
          var targetsJSON string
          err := rows.Scan(
              &p.Project, &p.Name, &p.Scope, &p.RollupState,
              &p.OKTargets, &p.TotalTargets,
              &triggerWhat, &triggerKind, &triggerAt,
              &targetsJSON, &p.UpdatedAt,
          )
          if err != nil {
              return nil, err
          }
          if triggerWhat.Valid {
              p.Trigger = &model.Trigger{
                  What: triggerWhat.String,
                  Kind: triggerKind.String,
                  At:   triggerAt.Time,
              }
          }
          if targetsJSON != "" {
              _ = json.Unmarshal([]byte(targetsJSON), &p.Targets)
          }
          pkgs = append(pkgs, &p)
      }
      return pkgs, rows.Err()
  }
  ```

  Update `QueryPackages` to call `scanPackages(rows)` instead of its inline loop.

  **Important:** Read the actual scan call in the existing `QueryPackages` before writing — the column order and nullable types must match exactly. If the scan in `QueryPackages` differs from the template above, match what's actually in the file.

- [ ] **Step 4: Run tests**

  ```bash
  cd backend && go test ./internal/store/... -v
  ```
  Expected: all PASS

- [ ] **Step 5: Commit**

  ```bash
  cd backend && git add internal/store/db.go internal/store/packages.go
  git commit -s -m "feat: add rollup_state index and GetActivePackages to store"
  ```

---

### Task 3: WorkingSet package

**Goal:** Create `internal/workingset/workingset.go` with the full WorkingSet API: `New`, `Seed`, `Add`, `Signal`, `Remove`, `Dispatch`, `StartScheduler`.

**Files:**
- Create: `backend/internal/workingset/workingset.go`
- Create: `backend/internal/workingset/workingset_test.go`

**Acceptance Criteria:**
- [ ] `New(queueSize int) *WorkingSet` allocates map and buffered channel
- [ ] `Seed(pkgs []*model.Package)` inserts all packages without sending to dispatch (no scheduler running yet)
- [ ] `Add(pkg)`: if key absent, inserts and attempts non-blocking send; if key present, no-op
- [ ] `Signal(pkg)`: inserts if absent; always attempts non-blocking send regardless of membership
- [ ] `Remove(key string)`: deletes from map
- [ ] `Dispatch() <-chan *model.Package`: returns the receive-only channel
- [ ] `StartScheduler(ctx, interval)`: spawns goroutine that non-blocking sends all map entries on each tick; exits on ctx cancel
- [ ] `go test ./internal/workingset/... -v` exits 0

**Verify:** `cd backend && go test ./internal/workingset/... -v` → all PASS

**Steps:**

- [ ] **Step 1: Create the package directory and write failing tests**

  ```bash
  mkdir -p backend/internal/workingset
  ```

  Create `backend/internal/workingset/workingset_test.go`:

  ```go
  package workingset_test

  import (
      "context"
      "testing"
      "time"

      "github.com/percona/obs-dashboard/internal/model"
      "github.com/percona/obs-dashboard/internal/workingset"
  )

  func pkg(project, name string, state model.RollupState) *model.Package {
      return &model.Package{Project: project, Name: name, RollupState: state}
  }

  func TestAddNewPackage(t *testing.T) {
      ws := workingset.New(10)
      ws.Add(pkg("proj", "pkg-a", model.RollupFailed))
      select {
      case p := <-ws.Dispatch():
          if p.Name != "pkg-a" {
              t.Errorf("unexpected package %s", p.Name)
          }
      case <-time.After(100 * time.Millisecond):
          t.Fatal("expected dispatch but nothing received")
      }
  }

  func TestAddExistingPackageIsNoop(t *testing.T) {
      ws := workingset.New(10)
      ws.Add(pkg("proj", "pkg-a", model.RollupFailed))
      <-ws.Dispatch() // drain first Add dispatch
      ws.Add(pkg("proj", "pkg-a", model.RollupFailed)) // second Add — no-op
      select {
      case <-ws.Dispatch():
          t.Fatal("expected no dispatch for existing package")
      case <-time.After(50 * time.Millisecond):
          // correct — no dispatch
      }
  }

  func TestSignalAlwaysDispatches(t *testing.T) {
      ws := workingset.New(10)
      ws.Add(pkg("proj", "pkg-a", model.RollupFailed))
      <-ws.Dispatch() // drain Add dispatch
      ws.Signal(pkg("proj", "pkg-a", model.RollupFailed)) // already in set — should still dispatch
      select {
      case p := <-ws.Dispatch():
          if p.Name != "pkg-a" {
              t.Errorf("unexpected package %s", p.Name)
          }
      case <-time.After(100 * time.Millisecond):
          t.Fatal("Signal did not dispatch for existing package")
      }
  }

  func TestSeedDoesNotDispatch(t *testing.T) {
      ws := workingset.New(10)
      ws.Seed([]*model.Package{
          pkg("proj", "pkg-a", model.RollupFailed),
          pkg("proj", "pkg-b", model.RollupBuilding),
      })
      select {
      case <-ws.Dispatch():
          t.Fatal("Seed should not dispatch to channel")
      case <-time.After(50 * time.Millisecond):
          // correct
      }
  }

  func TestRemove(t *testing.T) {
      ws := workingset.New(10)
      ws.Add(pkg("proj", "pkg-a", model.RollupFailed))
      <-ws.Dispatch()
      ws.Remove("proj/pkg-a")
      ws.Add(pkg("proj", "pkg-a", model.RollupFailed)) // should dispatch again (was removed)
      select {
      case <-ws.Dispatch():
          // correct
      case <-time.After(100 * time.Millisecond):
          t.Fatal("expected dispatch after Remove+Add")
      }
  }

  func TestStartScheduler(t *testing.T) {
      ws := workingset.New(10)
      ws.Seed([]*model.Package{pkg("proj", "pkg-a", model.RollupFailed)})
      ctx, cancel := context.WithCancel(context.Background())
      defer cancel()
      ws.StartScheduler(ctx, 20*time.Millisecond)
      select {
      case p := <-ws.Dispatch():
          if p.Name != "pkg-a" {
              t.Errorf("unexpected package %s", p.Name)
          }
      case <-time.After(200 * time.Millisecond):
          t.Fatal("scheduler did not dispatch seeded package")
      }
  }
  ```

  Run: `cd backend && go test ./internal/workingset/... -v`
  Expected: FAIL with package not found / undefined

- [ ] **Step 2: Implement workingset.go**

  Create `backend/internal/workingset/workingset.go`:

  ```go
  package workingset

  import (
      "context"
      "sync"
      "time"

      "github.com/percona/obs-dashboard/internal/model"
  )

  type WorkingSet struct {
      mu       sync.RWMutex
      packages map[string]*model.Package
      dispatch chan *model.Package
  }

  func New(queueSize int) *WorkingSet {
      return &WorkingSet{
          packages: make(map[string]*model.Package),
          dispatch: make(chan *model.Package, queueSize),
      }
  }

  func (ws *WorkingSet) Seed(pkgs []*model.Package) {
      ws.mu.Lock()
      defer ws.mu.Unlock()
      for _, p := range pkgs {
          ws.packages[p.Project+"/"+p.Name] = p
      }
  }

  func (ws *WorkingSet) Add(pkg *model.Package) {
      key := pkg.Project + "/" + pkg.Name
      ws.mu.Lock()
      defer ws.mu.Unlock()
      if _, exists := ws.packages[key]; exists {
          return
      }
      ws.packages[key] = pkg
      ws.send(pkg)
  }

  func (ws *WorkingSet) Signal(pkg *model.Package) {
      key := pkg.Project + "/" + pkg.Name
      ws.mu.Lock()
      defer ws.mu.Unlock()
      ws.packages[key] = pkg
      ws.send(pkg)
  }

  func (ws *WorkingSet) Remove(key string) {
      ws.mu.Lock()
      defer ws.mu.Unlock()
      delete(ws.packages, key)
  }

  func (ws *WorkingSet) Dispatch() <-chan *model.Package {
      return ws.dispatch
  }

  func (ws *WorkingSet) StartScheduler(ctx context.Context, interval time.Duration) {
      go func() {
          ticker := time.NewTicker(interval)
          defer ticker.Stop()
          for {
              select {
              case <-ctx.Done():
                  return
              case <-ticker.C:
                  ws.mu.RLock()
                  for _, p := range ws.packages {
                      ws.send(p)
                  }
                  ws.mu.RUnlock()
              }
          }
      }()
  }

  // send attempts a non-blocking send to the dispatch channel.
  // Must be called with ws.mu held (read or write lock).
  func (ws *WorkingSet) send(pkg *model.Package) {
      select {
      case ws.dispatch <- pkg:
      default:
      }
  }
  ```

  **Note on locking in send:** `send` is called from `StartScheduler` while holding the read lock (RLock). Since `send` itself only reads the channel (a non-blocking select), this is safe. Do NOT acquire another lock inside `send`.

- [ ] **Step 3: Run tests**

  ```bash
  cd backend && go test ./internal/workingset/... -v
  ```
  Expected: all PASS

- [ ] **Step 4: Commit**

  ```bash
  cd backend && git add internal/workingset/
  git commit -s -m "feat: add WorkingSet with dispatch channel and scheduler"
  ```

---

### Task 4: Worker Pool

**Goal:** Create `internal/worker/worker.go` defining the `Task` interface and `Pool` struct that reads from the working set dispatch channel and runs tasks per package.

**Files:**
- Create: `backend/internal/worker/worker.go`
- Create: `backend/internal/worker/worker_test.go`

**Acceptance Criteria:**
- [ ] `Task` interface: `Run(ctx context.Context, client *obs.Client, pkg *model.Package) error`
- [ ] `Pool` struct with `NewPool(size int, tasks []Task, client *obs.Client, db *sql.DB, hub *hub.Hub, ws *workingset.WorkingSet) *Pool`
- [ ] `Pool.Start(ctx)` spawns `size` goroutines, each reading from `ws.Dispatch()`
- [ ] Each goroutine runs all tasks in sequence; errors are logged as warnings and do not stop subsequent tasks
- [ ] After all tasks: calls `store.UpsertPackageState(db, pkg)` and `hub.Notify(hub.PackageUpdate(pkg))`
- [ ] If `pkg.RollupState == model.RollupSucceeded` after tasks: calls `ws.Remove(project/name)`
- [ ] Goroutines exit when ctx is cancelled
- [ ] `go test ./internal/worker/... -v` exits 0

**Verify:** `cd backend && go test ./internal/worker/... -v` → all PASS

**Steps:**

- [ ] **Step 1: Create worker directory and write failing tests**

  ```bash
  mkdir -p backend/internal/worker
  ```

  Create `backend/internal/worker/worker_test.go`:

  ```go
  package worker_test

  import (
      "context"
      "database/sql"
      "errors"
      "sync"
      "testing"
      "time"

      "github.com/percona/obs-dashboard/internal/hub"
      "github.com/percona/obs-dashboard/internal/model"
      "github.com/percona/obs-dashboard/internal/obs"
      "github.com/percona/obs-dashboard/internal/store"
      "github.com/percona/obs-dashboard/internal/worker"
      "github.com/percona/obs-dashboard/internal/workingset"
  )

  // captureTask records which packages it saw
  type captureTask struct {
      mu   sync.Mutex
      seen []*model.Package
  }

  func (t *captureTask) Run(_ context.Context, _ *obs.Client, pkg *model.Package) error {
      t.mu.Lock()
      t.seen = append(t.seen, pkg)
      t.mu.Unlock()
      return nil
  }

  // errorTask always returns an error
  type errorTask struct{}

  func (t errorTask) Run(_ context.Context, _ *obs.Client, pkg *model.Package) error {
      return errors.New("task error")
  }

  func openDB(t *testing.T) *sql.DB {
      t.Helper()
      db, err := store.Open(":memory:")
      if err != nil {
          t.Fatal(err)
      }
      t.Cleanup(func() { db.Close() })
      return db
  }

  func TestPoolRunsTasksForDispatchedPackage(t *testing.T) {
      db := openDB(t)
      h := hub.New()
      ws := workingset.New(10)
      capture := &captureTask{}

      ctx, cancel := context.WithCancel(context.Background())
      defer cancel()

      p := worker.NewPool(2, []worker.Task{capture}, nil, db, h, ws)
      p.Start(ctx)

      pkg := &model.Package{
          Project: "isv:percona", Name: "pkg-a", Scope: model.ScopeCommon,
          RollupState: model.RollupFailed, OKTargets: 0, TotalTargets: 1,
          Targets:   []model.Target{{Repo: "repo", Arch: "x86_64", State: "failed"}},
          UpdatedAt: time.Now().UTC(),
      }
      ws.Signal(pkg)

      // wait for task to run
      deadline := time.After(500 * time.Millisecond)
      for {
          capture.mu.Lock()
          n := len(capture.seen)
          capture.mu.Unlock()
          if n > 0 {
              break
          }
          select {
          case <-deadline:
              t.Fatal("task was never run")
          case <-time.After(10 * time.Millisecond):
          }
      }
  }

  func TestPoolRemovesSucceededPackageFromWorkingSet(t *testing.T) {
      db := openDB(t)
      h := hub.New()
      ws := workingset.New(10)

      // Task that marks the package as succeeded
      succeedTask := &succeedingTask{}
      ctx, cancel := context.WithCancel(context.Background())
      defer cancel()

      p := worker.NewPool(1, []worker.Task{succeedTask}, nil, db, h, ws)
      p.Start(ctx)

      pkg := &model.Package{
          Project: "isv:percona", Name: "pkg-a", Scope: model.ScopeCommon,
          RollupState: model.RollupFailed, OKTargets: 0, TotalTargets: 1,
          Targets:   []model.Target{{Repo: "repo", Arch: "x86_64", State: "failed"}},
          UpdatedAt: time.Now().UTC(),
      }
      ws.Add(pkg) // Add to set
      <-ws.Dispatch() // drain the initial dispatch from Add

      ws.Signal(pkg) // trigger worker

      time.Sleep(200 * time.Millisecond)

      // After the worker ran succeedTask, pkg.RollupState == succeeded → ws.Remove was called
      // Verify: a new Add should dispatch (package was removed from set)
      ws.Add(pkg)
      select {
      case <-ws.Dispatch():
          // correct — package was removed, so Add dispatched again
      case <-time.After(100 * time.Millisecond):
          t.Fatal("package was not removed from working set after success")
      }
  }

  func TestPoolContinuesAfterTaskError(t *testing.T) {
      db := openDB(t)
      h := hub.New()
      ws := workingset.New(10)
      errTask := errorTask{}
      capture := &captureTask{}

      ctx, cancel := context.WithCancel(context.Background())
      defer cancel()

      p := worker.NewPool(1, []worker.Task{errTask, capture}, nil, db, h, ws)
      p.Start(ctx)

      pkg := &model.Package{
          Project: "isv:percona", Name: "pkg-a", Scope: model.ScopeCommon,
          RollupState: model.RollupFailed, OKTargets: 0, TotalTargets: 1,
          Targets:   []model.Target{{Repo: "repo", Arch: "x86_64", State: "failed"}},
          UpdatedAt: time.Now().UTC(),
      }
      ws.Signal(pkg)

      deadline := time.After(500 * time.Millisecond)
      for {
          capture.mu.Lock()
          n := len(capture.seen)
          capture.mu.Unlock()
          if n > 0 {
              break
          }
          select {
          case <-deadline:
              t.Fatal("second task was never run after first task error")
          case <-time.After(10 * time.Millisecond):
          }
      }
  }

  // succeedingTask sets RollupState to succeeded
  type succeedingTask struct{}

  func (t succeedingTask) Run(_ context.Context, _ *obs.Client, pkg *model.Package) error {
      pkg.RollupState = model.RollupSucceeded
      return nil
  }
  ```

  Run: `cd backend && go test ./internal/worker/... -v`
  Expected: FAIL with package not found

- [ ] **Step 2: Implement worker.go**

  Create `backend/internal/worker/worker.go`:

  ```go
  package worker

  import (
      "context"
      "database/sql"
      "fmt"
      "log/slog"

      hubpkg "github.com/percona/obs-dashboard/internal/hub"
      "github.com/percona/obs-dashboard/internal/model"
      "github.com/percona/obs-dashboard/internal/obs"
      "github.com/percona/obs-dashboard/internal/store"
      "github.com/percona/obs-dashboard/internal/workingset"
  )

  // Task is implemented by types that enrich a package's state from OBS.
  // Implementations live in obs/tasks.go to avoid circular imports.
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
              p.process(ctx, pkg)
          }
      }
  }

  func (p *Pool) process(ctx context.Context, pkg *model.Package) {
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
      if pkg.RollupState == model.RollupSucceeded {
          p.ws.Remove(pkg.Project + "/" + pkg.Name)
      }
  }
  ```

- [ ] **Step 3: Run tests**

  ```bash
  cd backend && go test ./internal/worker/... -v
  ```
  Expected: all PASS

- [ ] **Step 4: Commit**

  ```bash
  cd backend && git add internal/worker/
  git commit -s -m "feat: add worker Pool with Task interface"
  ```

---

### Task 5: OBS client additions (PackageBuildResults and PackageBuildReason)

**Goal:** Add `PackageBuildResults` and `PackageBuildReason` to the OBS client with tests.

**Files:**
- Modify: `backend/internal/obs/client.go`
- Modify: `backend/internal/obs/client_test.go`

**Acceptance Criteria:**
- [ ] `PackageBuildResults(ctx, project, pkg string) ([]PackageBuildState, error)` calls `GET /build/{project}/_result?package={pkg}` and returns per-target states
- [ ] `BuildReasonResult` struct: `Explain string`, `Packages []string`
- [ ] `PackageBuildReason(ctx, project, repo, arch, pkg string) (BuildReasonResult, error)` calls `GET /build/{project}/{repo}/{arch}/{package}/_reason`
- [ ] XML parsing verified against a mock HTTP server
- [ ] `go test ./internal/obs/... -run TestPackageBuildResults -v` exits 0
- [ ] `go test ./internal/obs/... -run TestPackageBuildReason -v` exits 0

**Verify:** `cd backend && go test ./internal/obs/... -run "TestPackageBuildResults|TestPackageBuildReason" -v` → all PASS

**Steps:**

- [ ] **Step 1: Write failing tests**

  Read `backend/internal/obs/client_test.go` to understand the test pattern. The tests use `httptest.NewServer` to mock OBS responses. The client is constructed with `NewClient(ts.URL, "user", "pass")`.

  Add to `client_test.go`:

  ```go
  func TestPackageBuildResults(t *testing.T) {
      ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          if r.URL.Path == "/build/isv:percona/_result" && r.URL.Query().Get("package") == "mypkg" {
              fmt.Fprint(w, `<resultlist>
                <result project="isv:percona" repository="openSUSE_Tumbleweed" arch="x86_64" state="building">
                  <status package="mypkg" code="building"/>
                </result>
                <result project="isv:percona" repository="openSUSE_Tumbleweed" arch="aarch64" state="failed">
                  <status package="mypkg" code="failed"/>
                </result>
              </resultlist>`)
          } else {
              http.NotFound(w, r)
          }
      }))
      defer ts.Close()
      c := NewClient(ts.URL, "user", "pass")
      results, err := c.PackageBuildResults(context.Background(), "isv:percona", "mypkg")
      if err != nil {
          t.Fatal(err)
      }
      if len(results) != 2 {
          t.Fatalf("expected 2 results, got %d", len(results))
      }
      found := map[string]string{}
      for _, r := range results {
          found[r.Arch] = r.State
      }
      if found["x86_64"] != "building" {
          t.Errorf("x86_64 expected building, got %s", found["x86_64"])
      }
      if found["aarch64"] != "failed" {
          t.Errorf("aarch64 expected failed, got %s", found["aarch64"])
      }
  }

  func TestPackageBuildReason(t *testing.T) {
      ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          if r.URL.Path == "/build/isv:percona/openSUSE_Tumbleweed/x86_64/mypkg/_reason" {
              fmt.Fprint(w, `<reason>
                <explain>meta change</explain>
                <time>1234567890</time>
                <packagechange>
                  <change revision="abc">libfoo</change>
                  <change revision="def">libbar</change>
                </packagechange>
              </reason>`)
          } else {
              http.NotFound(w, r)
          }
      }))
      defer ts.Close()
      c := NewClient(ts.URL, "user", "pass")
      res, err := c.PackageBuildReason(context.Background(), "isv:percona", "openSUSE_Tumbleweed", "x86_64", "mypkg")
      if err != nil {
          t.Fatal(err)
      }
      if res.Explain != "meta change" {
          t.Errorf("expected 'meta change', got %q", res.Explain)
      }
      if len(res.Packages) != 2 {
          t.Fatalf("expected 2 packages, got %d", len(res.Packages))
      }
      if res.Packages[0] != "libfoo" || res.Packages[1] != "libbar" {
          t.Errorf("unexpected packages: %v", res.Packages)
      }
  }

  func TestPackageBuildReasonNonMeta(t *testing.T) {
      ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          fmt.Fprint(w, `<reason><explain>source change</explain></reason>`)
      }))
      defer ts.Close()
      c := NewClient(ts.URL, "user", "pass")
      res, err := c.PackageBuildReason(context.Background(), "isv:percona", "repo", "arch", "pkg")
      if err != nil {
          t.Fatal(err)
      }
      if res.Explain != "source change" {
          t.Errorf("expected 'source change', got %q", res.Explain)
      }
      if len(res.Packages) != 0 {
          t.Errorf("expected no packages for non-meta reason, got %v", res.Packages)
      }
  }
  ```

  Run: `cd backend && go test ./internal/obs/... -run "TestPackageBuildResults|TestPackageBuildReason" -v`
  Expected: FAIL with "undefined: PackageBuildResults"

- [ ] **Step 2: Implement PackageBuildResults**

  Read `backend/internal/obs/client.go` to understand the existing patterns (`BuildResults`, `PackageBlockedReason`). The file uses `c.get(ctx, path)` which returns `([]byte, error)`, and XML unmarshaling with `encoding/xml`.

  The existing `BuildResults` calls `/build/{project}/_result` and returns `[]PackageBuildState`. `PackageBuildResults` is a scoped version:

  ```go
  // PackageBuildResults fetches build results for a single package.
  func (c *Client) PackageBuildResults(ctx context.Context, project, pkg string) ([]PackageBuildState, error) {
      path := fmt.Sprintf("/build/%s/_result?package=%s", url.PathEscape(project), url.QueryEscape(pkg))
      body, err := c.get(ctx, path)
      if err != nil {
          return nil, err
      }
      return parseBuildResults(body)
  }
  ```

  **Note:** `parseBuildResults` is likely the private helper already used by `BuildResults`. Read the file to confirm. If `BuildResults` already extracts a `parseBuildResults` helper, reuse it. If not, extract one from the existing `BuildResults` implementation and use it in both.

  The `url` package must be imported (`"net/url"`). Check if it's already imported.

- [ ] **Step 3: Implement PackageBuildReason and BuildReasonResult**

  Add the types and method to `client.go`:

  ```go
  type BuildReasonResult struct {
      Explain  string
      Packages []string
  }

  type buildReasonChangeXML struct {
      Name string `xml:",chardata"`
  }

  type buildReasonXML struct {
      Explain string                 `xml:"explain"`
      Changes []buildReasonChangeXML `xml:"packagechange>change"`
  }

  // PackageBuildReason fetches the build trigger reason for a specific target.
  func (c *Client) PackageBuildReason(ctx context.Context, project, repo, arch, pkg string) (BuildReasonResult, error) {
      path := fmt.Sprintf("/build/%s/%s/%s/%s/_reason",
          url.PathEscape(project), url.PathEscape(repo),
          url.PathEscape(arch), url.PathEscape(pkg))
      body, err := c.get(ctx, path)
      if err != nil {
          return BuildReasonResult{}, err
      }
      var raw buildReasonXML
      if err := xml.Unmarshal(body, &raw); err != nil {
          return BuildReasonResult{}, fmt.Errorf("parse build reason: %w", err)
      }
      result := BuildReasonResult{Explain: raw.Explain}
      if raw.Explain == "meta change" {
          for _, ch := range raw.Changes {
              if ch.Name != "" {
                  result.Packages = append(result.Packages, ch.Name)
              }
          }
      }
      return result, nil
  }
  ```

  **Note on XML structure:** The test above assumes `<packagechange><change>name</change></packagechange>`. The XPath `packagechange>change` captures `<change>` elements that are direct children of `<packagechange>`. The `xml:",chardata"` tag captures the text content of `<change>`. If the real OBS API uses a different structure (e.g., `<change>` with a `name` attribute), adjust accordingly — the tests will catch it.

- [ ] **Step 4: Run tests**

  ```bash
  cd backend && go test ./internal/obs/... -run "TestPackageBuildResults|TestPackageBuildReason" -v
  ```
  Expected: all PASS

  Also run the full obs test suite to ensure no regressions:
  ```bash
  cd backend && go test ./internal/obs/... -v
  ```

- [ ] **Step 5: Commit**

  ```bash
  cd backend && git add internal/obs/client.go internal/obs/client_test.go
  git commit -s -m "feat: add PackageBuildResults and PackageBuildReason to OBS client"
  ```

---

### Task 6: Built-in tasks (BuildStateTask, BlockedReasonTask, BuildReasonTask)

**Goal:** Create `internal/obs/tasks.go` with three Task implementations and tests.

**Files:**
- Create: `backend/internal/obs/tasks.go`
- Create: `backend/internal/obs/tasks_test.go`

**Acceptance Criteria:**
- [ ] `BuildStateTask{}` implements `worker.Task` — calls `PackageBuildResults` and updates `pkg.Targets`, `pkg.RollupState`, `pkg.OKTargets`, `pkg.TotalTargets`
- [ ] `BlockedReasonTask{}` implements `worker.Task` — wraps `EnrichBlockedTargets(ctx, client, pkg)` and returns nil
- [ ] `BuildReasonTask{}` implements `worker.Task` — iterates non-succeeded targets, calls `PackageBuildReason`, updates `Target.BuildReason` and `Target.BuildReasonPackages`
- [ ] `go test ./internal/obs/... -run "TestBuildStateTask|TestBlockedReasonTask|TestBuildReasonTask" -v` exits 0
- [ ] `go build ./...` exits 0

**Verify:** `cd backend && go test ./internal/obs/... -run "TestBuildStateTask|TestBlockedReasonTask|TestBuildReasonTask" -v` → all PASS

**Steps:**

- [ ] **Step 1: Write failing tests**

  Create `backend/internal/obs/tasks_test.go`:

  ```go
  package obs_test

  import (
      "context"
      "fmt"
      "net/http"
      "net/http/httptest"
      "testing"
      "time"

      "github.com/percona/obs-dashboard/internal/model"
      "github.com/percona/obs-dashboard/internal/obs"
  )

  func TestBuildStateTask(t *testing.T) {
      ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          fmt.Fprint(w, `<resultlist>
            <result project="isv:percona" repository="repo" arch="x86_64" state="succeeded">
              <status package="mypkg" code="succeeded"/>
            </result>
          </resultlist>`)
      }))
      defer ts.Close()

      c := obs.NewClient(ts.URL, "u", "p")
      pkg := &model.Package{
          Project: "isv:percona", Name: "mypkg", Scope: model.ScopeCommon,
          RollupState: model.RollupFailed,
          Targets: []model.Target{{Repo: "repo", Arch: "x86_64", State: "failed"}},
          UpdatedAt: time.Now().UTC(),
      }

      task := obs.BuildStateTask{}
      if err := task.Run(context.Background(), c, pkg); err != nil {
          t.Fatal(err)
      }
      if pkg.RollupState != model.RollupSucceeded {
          t.Errorf("expected succeeded rollup, got %s", pkg.RollupState)
      }
      if pkg.OKTargets != 1 {
          t.Errorf("expected 1 OK target, got %d", pkg.OKTargets)
      }
  }

  func TestBlockedReasonTask(t *testing.T) {
      ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          fmt.Fprint(w, `<builddepinfo>
            <package name="mypkg">
              <pkgdep>dep-a</pkgdep>
            </package>
            <package name="dep-a">
              <error>not installable</error>
            </package>
          </builddepinfo>`)
      }))
      defer ts.Close()

      c := obs.NewClient(ts.URL, "u", "p")
      pkg := &model.Package{
          Project: "isv:percona", Name: "mypkg", Scope: model.ScopeCommon,
          RollupState: model.RollupBlocked,
          Targets: []model.Target{{Repo: "repo", Arch: "x86_64", State: "blocked"}},
          UpdatedAt: time.Now().UTC(),
      }

      task := obs.BlockedReasonTask{}
      if err := task.Run(context.Background(), c, pkg); err != nil {
          t.Fatal(err)
      }
      if pkg.Targets[0].BlockedBy == "" {
          t.Error("expected BlockedBy to be set")
      }
  }

  func TestBuildReasonTask(t *testing.T) {
      ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          fmt.Fprint(w, `<reason>
            <explain>meta change</explain>
            <packagechange>
              <change revision="abc">libfoo</change>
            </packagechange>
          </reason>`)
      }))
      defer ts.Close()

      c := obs.NewClient(ts.URL, "u", "p")
      pkg := &model.Package{
          Project: "isv:percona", Name: "mypkg", Scope: model.ScopeCommon,
          RollupState: model.RollupBuilding,
          Targets: []model.Target{
              {Repo: "repo", Arch: "x86_64", State: "building"},
              {Repo: "repo", Arch: "aarch64", State: "succeeded"}, // should be skipped
          },
          UpdatedAt: time.Now().UTC(),
      }

      task := obs.BuildReasonTask{}
      if err := task.Run(context.Background(), c, pkg); err != nil {
          t.Fatal(err)
      }
      if pkg.Targets[0].BuildReason != "meta change" {
          t.Errorf("expected 'meta change', got %q", pkg.Targets[0].BuildReason)
      }
      if len(pkg.Targets[0].BuildReasonPackages) != 1 || pkg.Targets[0].BuildReasonPackages[0] != "libfoo" {
          t.Errorf("unexpected BuildReasonPackages: %v", pkg.Targets[0].BuildReasonPackages)
      }
      if pkg.Targets[1].BuildReason != "" {
          t.Error("succeeded target should have no BuildReason")
      }
  }
  ```

  Run: `cd backend && go test ./internal/obs/... -run "TestBuildStateTask|TestBlockedReasonTask|TestBuildReasonTask" -v`
  Expected: FAIL with "undefined: obs.BuildStateTask"

- [ ] **Step 2: Implement tasks.go**

  Create `backend/internal/obs/tasks.go`:

  ```go
  package obs

  import (
      "context"
      "log/slog"

      "github.com/percona/obs-dashboard/internal/model"
  )

  // BuildStateTask refreshes the package's targets, rollup state, and counts
  // by fetching current build results from OBS for the specific package.
  type BuildStateTask struct{}

  func (t BuildStateTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
      results, err := client.PackageBuildResults(ctx, pkg.Project, pkg.Name)
      if err != nil {
          return err
      }
      updated := buildPackage(pkg.Project, pkg.Name, pkg.Scope, results)
      // Preserve existing BlockedBy and BuildReason data; only update state-derived fields.
      for i := range updated.Targets {
          for _, old := range pkg.Targets {
              if old.Repo == updated.Targets[i].Repo && old.Arch == updated.Targets[i].Arch {
                  updated.Targets[i].BlockedBy = old.BlockedBy
                  updated.Targets[i].BuildReason = old.BuildReason
                  updated.Targets[i].BuildReasonPackages = old.BuildReasonPackages
                  break
              }
          }
      }
      pkg.Targets = updated.Targets
      pkg.RollupState = updated.RollupState
      pkg.OKTargets = updated.OKTargets
      pkg.TotalTargets = updated.TotalTargets
      pkg.UpdatedAt = updated.UpdatedAt
      return nil
  }

  // BlockedReasonTask populates BlockedBy on blocked targets.
  type BlockedReasonTask struct{}

  func (t BlockedReasonTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
      EnrichBlockedTargets(ctx, client, pkg)
      return nil
  }

  // BuildReasonTask fetches the build trigger reason for non-succeeded targets.
  type BuildReasonTask struct{}

  func (t BuildReasonTask) Run(ctx context.Context, client *Client, pkg *model.Package) error {
      for i, target := range pkg.Targets {
          if target.State == "succeeded" {
              continue
          }
          result, err := client.PackageBuildReason(ctx, pkg.Project, target.Repo, target.Arch, pkg.Name)
          if err != nil {
              slog.Warn("worker: build reason", "pkg", pkg.Name, "repo", target.Repo, "arch", target.Arch, "err", err)
              continue
          }
          pkg.Targets[i].BuildReason = result.Explain
          if result.Explain == "meta change" {
              pkg.Targets[i].BuildReasonPackages = result.Packages
          } else {
              pkg.Targets[i].BuildReasonPackages = nil
          }
      }
      return nil
  }
  ```

  **Note on import cycle:** `tasks.go` is in package `obs` and calls `buildPackage` and `EnrichBlockedTargets` which are also in package `obs` (in `poller.go`). No import of `worker` package is needed here — Go's structural typing means `BuildStateTask`, `BlockedReasonTask`, and `BuildReasonTask` satisfy `worker.Task` as long as their `Run` method signatures match. `main.go` performs the assignment `[]worker.Task{obs.BuildStateTask{}, ...}` where the compiler verifies compatibility.

- [ ] **Step 3: Run tests**

  ```bash
  cd backend && go test ./internal/obs/... -run "TestBuildStateTask|TestBlockedReasonTask|TestBuildReasonTask" -v
  ```
  Expected: all PASS

  Full obs suite:
  ```bash
  cd backend && go test ./internal/obs/... -v
  ```

- [ ] **Step 4: Build check**

  ```bash
  cd backend && go build ./...
  ```
  Expected: exits 0

- [ ] **Step 5: Commit**

  ```bash
  cd backend && git add internal/obs/tasks.go internal/obs/tasks_test.go
  git commit -s -m "feat: add BuildStateTask, BlockedReasonTask, BuildReasonTask"
  ```

---

### Task 7: Wire everything together

**Goal:** Update `poller.go`, `consumer.go`, and `main.go` to wire the working set, worker pool, and scheduler into the running application.

**Files:**
- Modify: `backend/internal/obs/poller.go`
- Modify: `backend/internal/mq/consumer.go`
- Modify: `backend/cmd/obsboard/main.go`

**Acceptance Criteria:**
- [ ] `Poller` struct has `ws *workingset.WorkingSet` field; `NewPoller` accepts it as a new parameter
- [ ] `tick()` calls `ws.Add(pkg)` after `store.UpsertPackageState` when there is a state change (replaces `EnrichBlockedTargets` call — that is removed)
- [ ] `Consumer` struct has `ws *workingset.WorkingSet` field; `NewConsumer` accepts it as a new parameter
- [ ] `handle()` calls `ws.Signal(pkg)` after `c.upsertPackage(pkg)` in the `isPackageBuildEvent` branch (replaces `obs.EnrichBlockedTargets` call — that is removed)
- [ ] `main.go`: seeds working set from `store.GetActivePackages`, creates worker pool with all three tasks, starts pool and scheduler, passes `ws` to poller and consumer
- [ ] `go build ./...` exits 0

**Verify:** `cd backend && go build ./...` → exits 0, no output

**Steps:**

- [ ] **Step 1: Update poller.go**

  Read `backend/internal/obs/poller.go`. Currently:
  - `Poller` has fields: `client`, `db`, `interval`, `root`, `hub`
  - `NewPoller(client *Client, db *sql.DB, interval time.Duration, h *hubpkg.Hub) *Poller`
  - In `tick()`, line 88: `EnrichBlockedTargets(ctx, p.client, pkg)` — **remove this line**
  - After `store.UpsertPackageState` succeeds (inside the `if rollupChanged || targetsChanged` block, after the `p.hub.Notify` call) — **add `p.ws.Add(pkg)`**

  Changes:

  1. Add import: `"github.com/percona/obs-dashboard/internal/workingset"`

  2. Add field to `Poller`:
     ```go
     ws *workingset.WorkingSet
     ```

  3. Update `NewPoller` signature:
     ```go
     func NewPoller(client *Client, db *sql.DB, interval time.Duration, h *hubpkg.Hub, ws *workingset.WorkingSet) *Poller {
         return &Poller{client: client, db: db, interval: interval, root: "isv:percona", hub: h, ws: ws}
     }
     ```

  4. In `tick()`, remove the line:
     ```go
     EnrichBlockedTargets(ctx, p.client, pkg)
     ```

  5. In `tick()`, inside the `if rollupChanged || targetsChanged(prev, pkg)` block, after `p.hub.Notify(hubpkg.PackageUpdate(pkg))`:
     ```go
     p.ws.Add(pkg)
     ```

- [ ] **Step 2: Update consumer.go**

  Read `backend/internal/mq/consumer.go`. Currently:
  - `Consumer` has fields: `url`, `db`, `hub`, `obsClient`
  - `NewConsumer(url string, db *sql.DB, h *hubpkg.Hub, obsClient *obs.Client) *Consumer`
  - In `handle()`, line 303: `obs.EnrichBlockedTargets(ctx, c.obsClient, pkg)` — **remove this line**
  - After `c.upsertPackage(pkg)` succeeds — **add `c.ws.Signal(pkg)`**

  Changes:

  1. Add import: `"github.com/percona/obs-dashboard/internal/workingset"`

  2. Add field to `Consumer`:
     ```go
     ws *workingset.WorkingSet
     ```

  3. Update `NewConsumer` signature:
     ```go
     func NewConsumer(url string, db *sql.DB, h *hubpkg.Hub, obsClient *obs.Client, ws *workingset.WorkingSet) *Consumer {
         return &Consumer{url: url, db: db, hub: h, obsClient: obsClient, ws: ws}
     }
     ```

  4. Remove from `handle()`:
     ```go
     obs.EnrichBlockedTargets(ctx, c.obsClient, pkg)
     ```

  5. In `handle()`, after `c.upsertPackage(pkg)` check succeeds (i.e., after the `if err := c.upsertPackage(pkg); err != nil { ... return }` block), add:
     ```go
     c.ws.Signal(pkg)
     ```

     The resulting code should look like:
     ```go
     if err := c.upsertPackage(pkg); err != nil {
         slog.Error("mq: upsert package", "err", err)
         return
     }
     c.ws.Signal(pkg)
     evt := &model.Event{ ... }
     ```

- [ ] **Step 3: Update main.go**

  Read `backend/cmd/obsboard/main.go`. Currently it creates `obsClient`, `h`, `poller`, `consumer` then starts goroutines.

  Add imports:
  ```go
  "github.com/percona/obs-dashboard/internal/obs"   // already present
  "github.com/percona/obs-dashboard/internal/store" // already present
  "github.com/percona/obs-dashboard/internal/worker"
  "github.com/percona/obs-dashboard/internal/workingset"
  ```

  Replace the existing poller/consumer creation with the full wiring. Find these lines:
  ```go
  obsClient := obs.NewClient(cfg.OBS.BaseURL, cfg.OBS.Username, cfg.OBS.Password)
  h := hub.New()
  poller := obs.NewPoller(obsClient, db, cfg.Poller.Interval, h)
  consumer := mq.NewConsumer(cfg.MQ.URL, db, h, obsClient)
  ```

  Replace with:
  ```go
  obsClient := obs.NewClient(cfg.OBS.BaseURL, cfg.OBS.Username, cfg.OBS.Password)
  h := hub.New()

  // Seed working set from DB
  activePkgs, err := store.GetActivePackages(db)
  if err != nil {
      return fmt.Errorf("seed working set: %w", err)
  }
  ws := workingset.New(cfg.WorkerPool.QueueSize)
  ws.Seed(activePkgs)

  // Register tasks and start worker pool
  tasks := []worker.Task{
      obs.BuildStateTask{},
      obs.BlockedReasonTask{},
      obs.BuildReasonTask{},
  }
  pool := worker.NewPool(cfg.WorkerPool.Size, tasks, obsClient, db, h, ws)
  pool.Start(ctx)
  ws.StartScheduler(ctx, cfg.WorkerPool.PollInterval)

  poller := obs.NewPoller(obsClient, db, cfg.Poller.Interval, h, ws)
  consumer := mq.NewConsumer(cfg.MQ.URL, db, h, obsClient, ws)
  ```

- [ ] **Step 4: Build check**

  ```bash
  cd backend && go build ./...
  ```
  Expected: exits 0, no output.

  If there are import errors, check:
  - `obs.EnrichBlockedTargets` is no longer called from `consumer.go`, so the `obs` import in `consumer.go` might become unused — verify whether `obs.Client` is still referenced there (it is, as `c.obsClient *obs.Client`), so the import stays.
  - The `worker` package imports `obs` for `*obs.Client`; `obs/tasks.go` does NOT import `worker` — no cycle.

- [ ] **Step 5: Full test suite**

  ```bash
  cd backend && go test ./...
  ```
  Expected: all PASS

- [ ] **Step 6: Commit**

  ```bash
  cd backend && git add internal/obs/poller.go internal/mq/consumer.go cmd/obsboard/main.go
  git commit -s -m "feat: wire working set, worker pool, and scheduler into obsboard"
  ```
