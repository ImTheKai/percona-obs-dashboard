# Event Log Redesign

## Goal

Replace the noisy, hard-to-read build event log with a focused stream of eight meaningful event types, with build reasons shown for build-started and build-failed events.

## Architecture

Event emission is split between two sources:
- **Worker pool** — owns all per-target build events (started, succeeded, failed, published). Fires after the full task chain completes so reasons are always populated when events fire.
- **MQ consumer** — owns project/package lifecycle events only (created, deleted). All other MQ event types are dropped.

A new `PublishStateTask` is added to the worker pipeline to detect per-target publish transitions via the OBS API.

## Tech Stack

Go backend (worker, MQ consumer, model, store), Vue 3 / TypeScript frontend (EventRow.vue).

## User decisions (already made)

- Approach A: worker diff (snapshot before tasks, emit after all tasks complete).
- Build started is per-target (repo/arch), fired when `BuildReason` transitions from empty to non-empty.
- `unresolvable` and `broken` states count as build-failed events.
- `blocked` state is ignored (no event emitted).
- Published is per-target, detected by a new `PublishStateTask` via OBS API.
- Project context shown in every event row (project path in the tags row).
- Build started reason shown plain (no label prefix).
- Build failed reason label is `unresolvable:`, `broken:`, or empty for plain `failed` (build log text future work).

---

## Backend

### Worker diff (`worker/worker.go`)

`Pool.process()` snapshots `pkg.Targets` (slice of `model.Target`) **before** running the task chain. After all tasks complete it compares old vs new targets and emits events:

| Condition (old → new per target) | Event type | `why` content |
|---|---|---|
| `BuildReason == "" → non-empty` | `build_started` | reason text + triggering packages joined with `, ` |
| `state ∉ {failed,unresolvable,broken} → any of those` | `failed` | prefixed with `"unresolvable: "` or `"broken: "` when applicable, empty for plain `failed` |
| `state != "succeeded" → "succeeded"` | `succeeded` | empty |
| `Published == false → true` | `published` | empty |
| `state → "blocked"` | *(no event)* | — |

Events are appended via `store.AppendEvent` and broadcast via `hub.Notify`. The `what` field is `"<package> <event-label> on <repo>/<arch>"`.

`Pool` already holds `hub *hubpkg.Hub` and `db *sql.DB` — no interface changes needed.

### PublishStateTask (`obs/tasks.go`)

New task added **after** `BuildStateTask` in the pipeline (before `BlockedReasonTask` and `BuildReasonTask`). For each target whose state is `"succeeded"`, calls the OBS API to check whether that `(repo, arch)` has been published. Sets `Target.Published = true` when confirmed.

`model.Target` gains a `Published bool` field (JSON: `"published,omitempty"`), persisted in the DB via the existing `UpsertPackageState` upsert.

Working set removal condition changes from `RollupState == succeeded` to `RollupState == succeeded && allTargetsPublished(pkg)`, keeping packages in the pool until every succeeded target has been published.

### MQ consumer (`mq/consumer.go`)

Kept event types:

| Routing key | Event type | Scope |
|---|---|---|
| `opensuse.obs.project.create` | `created` | project-level scope |
| `opensuse.obs.project.delete` | `deleted` | project-level scope |
| `opensuse.obs.package.create` | `created` | package scope |
| `opensuse.obs.package.delete` *(new handler)* | `deleted` | package scope |

Dropped entirely (no event appended, cases removed):
- `opensuse.obs.repo.published` — replaced by per-target `PublishStateTask`
- `opensuse.obs.package.build_success`, `build_fail`, `build_unchanged` — replaced by worker diff
- `opensuse.obs.repo.build_started`, `repo.build_finished`
- `opensuse.obs.project.update`, `project.update_project_conf`
- `opensuse.obs.package.commit`, `package.version_change`

Note: `build_success` and `build_fail` MQ messages still trigger `upsertPackage` and `ws.Signal` for real-time state updates — only the `appendEvent` call is removed.

### model.Target (new field)

```go
type Target struct {
    Repo                string   `json:"repo"`
    Arch                string   `json:"arch"`
    State               string   `json:"state"`
    Details             string   `json:"details,omitempty"`
    BlockedBy           string   `json:"blocked_by,omitempty"`
    BuildReason         string   `json:"build_reason,omitempty"`
    BuildReasonPackages []string `json:"build_reason_packages,omitempty"`
    Published           bool     `json:"published,omitempty"`  // new
}
```

Store migration: `ALTER TABLE packages` is not needed — targets are stored as JSON blobs; the new field serialises transparently.

---

## Frontend

### EventRow.vue

Supported event types and their visual treatment:

| Event type | Glyph | Colour | Reason line |
|---|---|---|---|
| `build_started` | `▶` | `--info` (blue) | plain reason text in mono box |
| `succeeded` | `✓` | `--ok` (green) | — |
| `failed` | `✗` | `--fail` (red) | `unresolvable: …`, `broken: …`, or empty |
| `published` | `↑` | `--brand-purple` | — |
| `created` | `+` | `--ok` | — |
| `deleted` | `−` | `--fail` | — |

Every row shows in the tags section:
- Scope chip (PPG, PPG Common, Common, Container, Project)
- Project path in `font-mono` muted text (e.g. `isv:percona:ppg:17`)
- `repo/arch` when present (build events)

Reason line (build_started and failed): rendered as a monospace pill below the title row, truncated with ellipsis.

### api.ts

`EventType` union retains all existing values for backwards compatibility with events already in the DB. The unused legacy types (`version_change`, `updated`, `triggered`, `started`, `build_finished`) are kept in the type but receive neutral styling in `EventRow`.

---

## Event `what` / `why` field conventions

| Event | `what` | `why` |
|---|---|---|
| build_started | `"<pkg> build started on <repo>/<arch>"` | reason + packages |
| succeeded | `"<pkg> succeeded on <repo>/<arch>"` | `""` |
| failed | `"<pkg> failed on <repo>/<arch>"` | `"unresolvable: …"` / `"broken: …"` / `""` |
| published | `"<pkg> published on <repo>/<arch>"` | `""` |
| package created | `"package <pkg> created"` | sender |
| package deleted | `"package <pkg> deleted"` | sender |
| project created | `"project <project> created"` | sender |
| project deleted | `"project <project> deleted"` | sender |
