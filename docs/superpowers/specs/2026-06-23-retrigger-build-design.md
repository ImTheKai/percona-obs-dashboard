# Retrigger Build Design

## Goal

Allow users to retrigger the build of a specific package target directly from the dashboard UI, without having to visit OBS or use the command line.

## Architecture

```
PackageCard.vue → useRebuild.ts → POST /api/rebuild → OBS client → OBS API
```

Four backend files change (obs client, handler, router) plus one new frontend composable and one modified Vue component. No database changes, no schema changes.

## Trigger States

The restart icon is shown on a target row when the target's state is one of:
- `failed`
- `broken`
- `unresolvable`
- `blocked`

It is **not** shown for `building`, `scheduled`, `succeeded`, or `published` states.

## Backend Changes

### `backend/internal/obs/client.go`

Add a private `post(ctx context.Context, path string) error` helper, mirroring the existing `get()`:
- Sets `Authorization: Basic` header using existing credentials
- Fires `HTTP POST` to `c.base + path` with no request body
- On non-2xx response: reads up to 512 bytes of the body and returns it as an error string
- On success: returns nil

Add a public `Rebuild(ctx context.Context, project, repo, arch, pkg string) error` method:
- Builds path: `/build/<project>?cmd=rebuild&repository=<repo>&arch=<arch>&package=<pkg>`
- `project` is path-escaped (`url.PathEscape`); `repo`, `arch`, and `pkg` are query-escaped (`url.QueryEscape`)
- Delegates to `post()`

### `backend/internal/api/handlers.go`

Add `rebuildHandler(obsClient *obs.Client) http.HandlerFunc`:
- Decodes JSON body: `{"project": "...", "repo": "...", "arch": "...", "package": "..."}`
- Returns 400 if any field is empty
- Calls `obsClient.Rebuild(r.Context(), project, repo, arch, pkg)`
- Returns `{"status":"ok"}` (200) on success
- Returns 502 with the OBS error message on failure

### `backend/internal/api/server.go`

Register: `r.Post("/api/rebuild", rebuildHandler(obsClient))`

## Frontend Changes

### `frontend/src/composables/useRebuild.ts` (new file)

Exposes three functions:

```typescript
function trigger(project: string, pkg: string, repo: string, arch: string): Promise<void>
function isLoading(repo: string, arch: string): boolean
function errorFor(repo: string, arch: string): string | null
```

Internal state uses two `Map<string, Ref<...>>` keyed by `"repo/arch"`.

Behaviour:
- `trigger()` sets `isLoading` to true, calls `POST /api/rebuild`, then sets `isLoading` to false
- On success: does nothing — the target transitions naturally via SSE/polling
- On failure: stores the error message in `errorFor`; auto-clears after 4 seconds

### `frontend/src/components/PackageCard.vue`

In the target row for each qualifying state (`failed`, `broken`, `unresolvable`, `blocked`):

- A small rotate-cw restart icon appears at the right end of the row
- While `isLoading(repo, arch)` is true: the icon is replaced by a same-size spinner
- If `errorFor(repo, arch)` is non-null: a one-line error message is shown in red below the target row; it disappears automatically after 4 seconds (driven by the composable's auto-clear)
- Clicking the icon calls `trigger(pkg.project, pkg.name, t.repo, t.arch)`

## UX Summary

| Event | UI response |
|-------|-------------|
| User clicks restart icon | Icon → spinner |
| OBS accepts rebuild | Spinner → icon (target will transition to `scheduled`/`building` on next poll) |
| OBS returns error | Spinner → icon + inline red error text below the row (auto-dismisses after 4s) |

## Out of Scope

- Confirmation dialog before triggering (not requested)
- Per-user authorisation (same OBS credentials used by the backend for polling)
- Bulk retrigger (all failed targets at once)
