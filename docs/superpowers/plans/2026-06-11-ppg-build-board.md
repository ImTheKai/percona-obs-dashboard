# PPG Build Board Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a failure-first OBS build monitor — Go backend (RabbitMQ consumer + OBS poller + SQLite + HTTP API) plus Vue 3 SPA that mirrors the approved HTML mockup exactly.

**Architecture:** Single Go binary handles all backend concerns. Vue 3 SPA is served as static files by the Go binary in production; in development a Vite dev server proxies `/api` to the Go backend. Docker Compose runs both services.

**Tech Stack:** Go 1.22 · github.com/spf13/viper · modernc.org/sqlite · github.com/rabbitmq/amqp091-go · github.com/go-chi/chi/v5 · github.com/oklog/ulid/v2 · Vue 3 + TypeScript + Vite + Tailwind CSS

**User decisions (already made):**
- Go backend, Vue 3 + TypeScript + Tailwind, Docker Compose
- SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- Config: env vars > YAML file (`CONFIG_FILE` env var, default `./config.yaml`)
- `OBS_USERNAME` and `OBS_PASSWORD` required — no anonymous OBS API access
- RabbitMQ: `amqps://opensuse:opensuse@rabbit.opensuse.org:5671/`, exchange `pubsub`, routing key `opensuse.obs.package.#`
- OBS subproject discovery via `GET /source/isv:percona` (path param, recursive walk)
- `POLL_INTERVAL` configurable (default `5m`)
- Frontend matches HTML mockup exactly; Tailwind uses CSS variable theme tokens

---

## File Map

```
percona-obs-dashboard/
  docker-compose.yml
  docker-compose.override.yml
  .env.example
  config.yaml.example
  backend/
    Dockerfile
    go.mod
    cmd/obsboard/main.go
    internal/
      config/config.go
      model/types.go
      obs/client.go
      obs/poller.go
      obs/trigger.go
      mq/consumer.go
      store/db.go
      store/packages.go
      store/events.go
      api/server.go
      api/handlers.go
  frontend/
    Dockerfile
    package.json
    vite.config.ts
    tailwind.config.ts
    postcss.config.js
    src/
      main.ts
      App.vue
      assets/theme.css
      assets/fonts/  (Roboto files extracted from mockup)
      types/api.ts
      composables/usePackages.ts
      composables/useEvents.ts
      components/
        AppHeader.vue
        ContextBar.vue
        ScopeChip.vue
        HealthHeader.vue
        MainGrid.vue
        FailureBoard.vue
        PackageCard.vue
        GreenStrip.vue
        EventLog.vue
        TimeWindowPicker.vue
        EventRow.vue
```

---

### Task 1: Project Scaffold

**Goal:** Create the full repository skeleton — all directories, config files, Dockerfiles, docker-compose, and stub Go/Vue files that compile/build without logic.

**Files:**
- Create: `backend/go.mod`
- Create: `backend/Dockerfile`
- Create: `frontend/package.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tailwind.config.ts`
- Create: `frontend/postcss.config.js`
- Create: `frontend/Dockerfile`
- Create: `frontend/index.html`
- Create: `frontend/src/main.ts`
- Create: `frontend/src/App.vue`
- Create: `docker-compose.yml`
- Create: `docker-compose.override.yml`
- Create: `.env.example`
- Create: `config.yaml.example`
- Create: all `backend/internal/*/` stub `.go` files

**Acceptance Criteria:**
- [ ] `cd backend && go build ./...` succeeds
- [ ] `cd frontend && npm install && npm run build` succeeds
- [ ] `docker compose build` succeeds for both services

**Verify:** `cd backend && go build ./... && echo OK`

**Steps:**

- [ ] **Step 1: Create `backend/go.mod`**

```
module github.com/percona/obs-dashboard

go 1.22

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/oklog/ulid/v2 v2.1.0
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/spf13/viper v1.19.0
	modernc.org/sqlite v1.33.1
)
```

Run `cd backend && go mod tidy` to populate `go.sum`.

- [ ] **Step 2: Create stub Go files** for every internal package (these just declare the package name; logic is added in later tasks):

`backend/internal/model/types.go` → `package model`
`backend/internal/config/config.go` → `package config`
`backend/internal/obs/client.go` → `package obs`
`backend/internal/obs/poller.go` → `package obs`
`backend/internal/obs/trigger.go` → `package obs`
`backend/internal/mq/consumer.go` → `package mq`
`backend/internal/store/db.go` → `package store`
`backend/internal/store/packages.go` → `package store`
`backend/internal/store/events.go` → `package store`
`backend/internal/api/server.go` → `package api`
`backend/internal/api/handlers.go` → `package api`

`backend/cmd/obsboard/main.go`:
```go
package main

func main() {}
```

- [ ] **Step 3: Create `backend/Dockerfile`**

```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /obsboard ./cmd/obsboard

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /obsboard /obsboard
ENTRYPOINT ["/obsboard"]
```

- [ ] **Step 4: Create `frontend/package.json`**

```json
{
  "name": "obs-dashboard-frontend",
  "private": true,
  "version": "0.0.1",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "^3.4.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "autoprefixer": "^10.4.0",
    "postcss": "^8.4.0",
    "tailwindcss": "^3.4.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0",
    "vue-tsc": "^2.0.0"
  }
}
```

- [ ] **Step 5: Create `frontend/vite.config.ts`**

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://backend:8080',
        changeOrigin: true,
      },
    },
  },
})
```

- [ ] **Step 6: Create `frontend/tailwind.config.ts`** (tokens wired in Task 12)

```ts
import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{vue,ts}'],
  theme: { extend: {} },
  plugins: [],
} satisfies Config
```

- [ ] **Step 7: Create `frontend/postcss.config.js`**

```js
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
}
```

- [ ] **Step 8: Create `frontend/index.html`**

```html
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>PPG Build Board</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
```

- [ ] **Step 9: Create `frontend/src/main.ts`**

```ts
import { createApp } from 'vue'
import App from './App.vue'
import './assets/theme.css'

createApp(App).mount('#app')
```

- [ ] **Step 10: Create `frontend/src/App.vue`** (stub)

```vue
<template><div>PPG Build Board</div></template>
<script setup lang="ts"></script>
```

- [ ] **Step 11: Create `frontend/Dockerfile`** (dev Vite server)

```dockerfile
FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm install
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host"]
```

- [ ] **Step 12: Create `docker-compose.yml`**

```yaml
services:
  backend:
    build: ./backend
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    env_file: .env

  frontend:
    build: ./frontend
    ports:
      - "5173:5173"
    volumes:
      - ./frontend:/app
      - /app/node_modules
    environment:
      - VITE_API_BASE=http://localhost:8080
    depends_on:
      - backend
```

- [ ] **Step 13: Create `docker-compose.override.yml`** (dev: volume mounts for HMR)

```yaml
services:
  frontend:
    volumes:
      - ./frontend:/app
      - /app/node_modules
```

- [ ] **Step 14: Create `.env.example`**

```
# OBS credentials (required)
OBS_USERNAME=
OBS_PASSWORD=
OBS_BASE_URL=https://build.opensuse.org

# RabbitMQ
MQ_URL=amqps://opensuse:opensuse@rabbit.opensuse.org:5671/

# Poller
POLL_INTERVAL=5m

# Storage
DB_PATH=/data/obsboard.db
EVENT_RETENTION=7d

# Server
HTTP_PORT=8080
FRONTEND_DIR=
CONFIG_FILE=./config.yaml
```

- [ ] **Step 15: Create `config.yaml.example`**

```yaml
obs:
  username: ""
  password: ""
  base_url: "https://build.opensuse.org"

mq:
  url: "amqps://opensuse:opensuse@rabbit.opensuse.org:5671/"

poller:
  interval: "5m"

store:
  db_path: "/data/obsboard.db"
  event_retention: "7d"

server:
  http_port: 8080
  frontend_dir: ""
```

- [ ] **Step 16: Verify build**

```bash
cd backend && go build ./... && echo "Go OK"
cd ../frontend && npm install && npm run build && echo "Frontend OK"
```

- [ ] **Step 17: Commit**

```bash
git add backend/ frontend/ docker-compose.yml docker-compose.override.yml .env.example config.yaml.example
git commit -s -m "feat: project scaffold — backend and frontend skeletons"
```

```json:metadata
{"files": ["backend/go.mod","backend/Dockerfile","frontend/package.json","frontend/vite.config.ts","docker-compose.yml",".env.example"], "verifyCommand": "cd backend && go build ./... && echo OK", "acceptanceCriteria": ["go build ./... succeeds", "npm run build succeeds", "docker compose build succeeds"], "modelTier": "mechanical"}
```

---

### Task 2: Data Model

**Goal:** Define all shared Go types in `internal/model/types.go` — Package, Target, Trigger, Event, and their state constants.

**Files:**
- Modify: `backend/internal/model/types.go`

**Acceptance Criteria:**
- [ ] `go build ./internal/model/...` succeeds
- [ ] All state constants defined: RollupState (6 values), Scope (5 values), EventType (8 values)
- [ ] Package, Target, Trigger, Event structs have correct JSON tags matching the API spec

**Verify:** `cd backend && go vet ./internal/model/...`

**Steps:**

- [ ] **Step 1: Write `backend/internal/model/types.go`**

```go
package model

import "time"

type RollupState string

const (
	RollupFailed       RollupState = "failed"
	RollupBroken       RollupState = "broken"
	RollupUnresolvable RollupState = "unresolvable"
	RollupBlocked      RollupState = "blocked"
	RollupBuilding     RollupState = "building"
	RollupSucceeded    RollupState = "succeeded"
)

// Severity returns a sortable integer: higher = worse (for failure-first ordering).
func (s RollupState) Severity() int {
	switch s {
	case RollupBroken:
		return 5
	case RollupFailed:
		return 4
	case RollupUnresolvable:
		return 3
	case RollupBlocked:
		return 2
	case RollupBuilding:
		return 1
	default:
		return 0
	}
}

type Scope string

const (
	ScopeCommon    Scope = "common"
	ScopePPGCommon Scope = "ppgcommon"
	ScopeVersion   Scope = "version"
	ScopeContainer Scope = "container"
	ScopeRelease   Scope = "release"
)

type Target struct {
	Repo  string `json:"repo"`
	Arch  string `json:"arch"`
	State string `json:"state"`
}

type Trigger struct {
	What string    `json:"what"`
	Kind string    `json:"kind"`
	At   time.Time `json:"at"`
}

type Package struct {
	Project      string      `json:"project"`
	Name         string      `json:"name"`
	Scope        Scope       `json:"scope"`
	RollupState  RollupState `json:"rollup_state"`
	OKTargets    int         `json:"ok_targets"`
	TotalTargets int         `json:"total_targets"`
	Trigger      *Trigger    `json:"trigger,omitempty"`
	Targets      []Target    `json:"targets"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type EventType string

const (
	EventTriggered    EventType = "triggered"
	EventStarted      EventType = "started"
	EventSucceeded    EventType = "succeeded"
	EventFailed       EventType = "failed"
	EventUnresolvable EventType = "unresolvable"
	EventBroken       EventType = "broken"
	EventBlocked      EventType = "blocked"
	EventPublished    EventType = "published"
)

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
	URL     string    `json:"url"`
	At      time.Time `json:"at"`
}
```

- [ ] **Step 2: Verify and commit**

```bash
cd backend && go vet ./internal/model/...
git add backend/internal/model/types.go
git commit -s -m "feat(model): define Package, Event, Trigger types and state constants"
```

```json:metadata
{"files": ["backend/internal/model/types.go"], "verifyCommand": "cd backend && go vet ./internal/model/...", "acceptanceCriteria": ["go vet passes", "all state constants defined", "JSON tags match API spec"], "modelTier": "mechanical"}
```

---

### Task 3: Configuration System

**Goal:** Implement `internal/config/config.go` loading config from env vars (highest priority), YAML file, and built-in defaults using Viper.

**Files:**
- Modify: `backend/internal/config/config.go`

**Acceptance Criteria:**
- [ ] `go vet ./internal/config/...` passes
- [ ] `OBS_USERNAME=""` causes `Load()` to return an error
- [ ] `POLL_INTERVAL=2m` overrides the 5m default
- [ ] `EVENT_RETENTION=7d` parses to `7 * 24 * time.Hour`

**Verify:** `cd backend && go test ./internal/config/... -v`

**Steps:**

- [ ] **Step 1: Write `backend/internal/config/config.go`**

```go
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	OBS    OBSConfig
	MQ     MQConfig
	Poller PollerConfig
	Store  StoreConfig
	Server ServerConfig
}

type OBSConfig struct {
	Username string
	Password string
	BaseURL  string
}

type MQConfig struct {
	URL string
}

type PollerConfig struct {
	Interval time.Duration
}

type StoreConfig struct {
	DBPath         string
	EventRetention time.Duration
}

type ServerConfig struct {
	HTTPPort    int
	FrontendDir string
}

func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("obs.base_url", "https://build.opensuse.org")
	v.SetDefault("mq.url", "amqps://opensuse:opensuse@rabbit.opensuse.org:5671/")
	v.SetDefault("poller.interval", "5m")
	v.SetDefault("store.db_path", "/data/obsboard.db")
	v.SetDefault("store.event_retention", "7d")
	v.SetDefault("server.http_port", 8080)
	v.SetDefault("server.frontend_dir", "")

	// Config file (optional)
	cfgFile := "config.yaml"
	if f := v.GetString("CONFIG_FILE"); f != "" {
		cfgFile = f
	}
	v.SetConfigFile(cfgFile)
	_ = v.ReadInConfig()

	// Env vars take priority
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	for _, pair := range [][]string{
		{"obs.username", "OBS_USERNAME"},
		{"obs.password", "OBS_PASSWORD"},
		{"obs.base_url", "OBS_BASE_URL"},
		{"mq.url", "MQ_URL"},
		{"poller.interval", "POLL_INTERVAL"},
		{"store.db_path", "DB_PATH"},
		{"store.event_retention", "EVENT_RETENTION"},
		{"server.http_port", "HTTP_PORT"},
		{"server.frontend_dir", "FRONTEND_DIR"},
	} {
		_ = v.BindEnv(pair[0], pair[1])
	}

	pollInterval, err := time.ParseDuration(v.GetString("poller.interval"))
	if err != nil {
		return nil, fmt.Errorf("invalid POLL_INTERVAL %q: %w", v.GetString("poller.interval"), err)
	}

	retention, err := parseRetention(v.GetString("store.event_retention"))
	if err != nil {
		return nil, fmt.Errorf("invalid EVENT_RETENTION %q: %w", v.GetString("store.event_retention"), err)
	}

	cfg := &Config{
		OBS: OBSConfig{
			Username: v.GetString("obs.username"),
			Password: v.GetString("obs.password"),
			BaseURL:  strings.TrimRight(v.GetString("obs.base_url"), "/"),
		},
		MQ: MQConfig{URL: v.GetString("mq.url")},
		Poller: PollerConfig{Interval: pollInterval},
		Store: StoreConfig{
			DBPath:         v.GetString("store.db_path"),
			EventRetention: retention,
		},
		Server: ServerConfig{
			HTTPPort:    v.GetInt("server.http_port"),
			FrontendDir: v.GetString("server.frontend_dir"),
		},
	}

	if cfg.OBS.Username == "" {
		return nil, fmt.Errorf("OBS_USERNAME is required")
	}

	return cfg, nil
}

// parseRetention handles "7d" as well as standard Go duration strings.
func parseRetention(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
```

- [ ] **Step 2: Write `backend/internal/config/config_test.go`**

```go
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	os.Setenv("OBS_USERNAME", "testuser")
	os.Setenv("OBS_PASSWORD", "testpass")
	defer os.Unsetenv("OBS_USERNAME")
	defer os.Unsetenv("OBS_PASSWORD")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Poller.Interval != 5*time.Minute {
		t.Errorf("expected 5m, got %v", cfg.Poller.Interval)
	}
	if cfg.Store.EventRetention != 7*24*time.Hour {
		t.Errorf("expected 168h, got %v", cfg.Store.EventRetention)
	}
}

func TestLoadMissingUsername(t *testing.T) {
	os.Unsetenv("OBS_USERNAME")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing OBS_USERNAME")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	os.Setenv("OBS_USERNAME", "u")
	os.Setenv("POLL_INTERVAL", "2m")
	defer os.Unsetenv("OBS_USERNAME")
	defer os.Unsetenv("POLL_INTERVAL")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Poller.Interval != 2*time.Minute {
		t.Errorf("expected 2m override, got %v", cfg.Poller.Interval)
	}
}
```

- [ ] **Step 3: Run tests and commit**

```bash
cd backend && go test ./internal/config/... -v
# Expected: PASS (3 tests)
git add backend/internal/config/
git commit -s -m "feat(config): Viper-based config loader with env > YAML > defaults"
```

```json:metadata
{"files": ["backend/internal/config/config.go","backend/internal/config/config_test.go"], "verifyCommand": "cd backend && go test ./internal/config/... -v", "acceptanceCriteria": ["all 3 config tests pass","missing OBS_USERNAME returns error","POLL_INTERVAL env override works"], "modelTier": "mechanical"}
```

---

### Task 4: SQLite Store — Schema and Open

**Goal:** Implement `internal/store/db.go` — open a SQLite database, apply the schema (packages + events tables), and expose the `*sql.DB` for other store functions.

**Files:**
- Modify: `backend/internal/store/db.go`
- Create: `backend/internal/store/db_test.go`

**Acceptance Criteria:**
- [ ] `Open(":memory:")` returns a non-nil `*sql.DB` without error
- [ ] Both tables and the `events_at` index exist after `Open`
- [ ] Calling `Open` twice on the same path is idempotent (IF NOT EXISTS guards)

**Verify:** `cd backend && go test ./internal/store/... -run TestOpen -v`

**Steps:**

- [ ] **Step 1: Write `backend/internal/store/db.go`**

```go
package store

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS packages (
    project        TEXT NOT NULL,
    name           TEXT NOT NULL,
    scope          TEXT NOT NULL,
    rollup_state   TEXT NOT NULL,
    ok_targets     INTEGER NOT NULL DEFAULT 0,
    total_targets  INTEGER NOT NULL DEFAULT 0,
    trigger_what   TEXT,
    trigger_kind   TEXT,
    trigger_at     DATETIME,
    targets_json   TEXT NOT NULL DEFAULT '[]',
    updated_at     DATETIME NOT NULL,
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
    at       DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS events_at ON events(at);
`

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
```

- [ ] **Step 2: Write `backend/internal/store/db_test.go`**

```go
package store

import (
	"testing"
)

func TestOpen(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	for _, table := range []string{"packages", "events"} {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// Verify index exists
	var idx string
	if err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='events_at'",
	).Scan(&idx); err != nil {
		t.Errorf("events_at index not found: %v", err)
	}
}

func TestOpenIdempotent(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	db2, err := Open(":memory:")
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	db2.Close()
}
```

- [ ] **Step 3: Run tests and commit**

```bash
cd backend && go test ./internal/store/... -run TestOpen -v
git add backend/internal/store/db.go backend/internal/store/db_test.go
git commit -s -m "feat(store): SQLite schema — packages and events tables"
```

```json:metadata
{"files": ["backend/internal/store/db.go","backend/internal/store/db_test.go"], "verifyCommand": "cd backend && go test ./internal/store/... -run TestOpen -v", "acceptanceCriteria": ["Open(':memory:') succeeds","both tables exist after Open","events_at index exists"], "modelTier": "mechanical"}
```

---

### Task 5: Package and Event Persistence

**Goal:** Implement `store/packages.go` (UpsertPackageState, QueryPackages) and `store/events.go` (AppendEvent, QueryEvents, PruneEvents).

**Files:**
- Modify: `backend/internal/store/packages.go`
- Modify: `backend/internal/store/events.go`
- Create: `backend/internal/store/packages_test.go`
- Create: `backend/internal/store/events_test.go`

**Acceptance Criteria:**
- [ ] Upsert + query round-trips Package with all fields including Targets JSON and Trigger
- [ ] QueryEvents filters by time range and returns events newest-first
- [ ] PruneEvents deletes events older than the cutoff

**Verify:** `cd backend && go test ./internal/store/... -v`

**Steps:**

- [ ] **Step 1: Write `backend/internal/store/packages.go`**

```go
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

// UpsertPackageState inserts or replaces a package row.
func UpsertPackageState(db *sql.DB, p *model.Package) error {
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
	_, err = db.Exec(`
		INSERT INTO packages
			(project, name, scope, rollup_state, ok_targets, total_targets,
			 trigger_what, trigger_kind, trigger_at, targets_json, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project, name) DO UPDATE SET
			scope=excluded.scope, rollup_state=excluded.rollup_state,
			ok_targets=excluded.ok_targets, total_targets=excluded.total_targets,
			trigger_what=excluded.trigger_what, trigger_kind=excluded.trigger_kind,
			trigger_at=excluded.trigger_at, targets_json=excluded.targets_json,
			updated_at=excluded.updated_at`,
		p.Project, p.Name, string(p.Scope), string(p.RollupState),
		p.OKTargets, p.TotalTargets,
		trigWhat, trigKind, trigAt,
		string(targetsJSON), p.UpdatedAt,
	)
	return err
}

// QueryPackages returns all packages for a given OBS root prefix (e.g. "isv:percona").
func QueryPackages(db *sql.DB, projectPrefix string) ([]*model.Package, error) {
	rows, err := db.Query(`
		SELECT project, name, scope, rollup_state, ok_targets, total_targets,
		       trigger_what, trigger_kind, trigger_at, targets_json, updated_at
		FROM packages WHERE project LIKE ? ORDER BY project, name`,
		projectPrefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pkgs []*model.Package
	for rows.Next() {
		p := &model.Package{}
		var trigWhat, trigKind sql.NullString
		var trigAt sql.NullTime
		var targetsJSON string
		if err := rows.Scan(
			&p.Project, &p.Name, &p.Scope, &p.RollupState,
			&p.OKTargets, &p.TotalTargets,
			&trigWhat, &trigKind, &trigAt,
			&targetsJSON, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if trigWhat.Valid {
			p.Trigger = &model.Trigger{
				What: trigWhat.String,
				Kind: trigKind.String,
				At:   trigAt.Time,
			}
		}
		if err := json.Unmarshal([]byte(targetsJSON), &p.Targets); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, rows.Err()
}
```

- [ ] **Step 2: Write `backend/internal/store/events.go`**

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
		INSERT INTO events (id, type, scope, project, package, repo, arch, what, why, url, at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, string(e.Type), string(e.Scope),
		e.Project, e.Package, nullStr(e.Repo), nullStr(e.Arch),
		e.What, e.Why, e.URL, e.At,
	)
	return err
}

// QueryEvents returns events for a project prefix within [from, to], newest first.
func QueryEvents(db *sql.DB, projectPrefix string, from, to time.Time) ([]*model.Event, error) {
	rows, err := db.Query(`
		SELECT id, type, scope, project, package,
		       COALESCE(repo,''), COALESCE(arch,''),
		       what, why, url, at
		FROM events
		WHERE project LIKE ? AND at >= ? AND at <= ?
		ORDER BY at DESC`,
		projectPrefix+"%", from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*model.Event
	for rows.Next() {
		e := &model.Event{}
		if err := rows.Scan(
			&e.ID, &e.Type, &e.Scope, &e.Project, &e.Package,
			&e.Repo, &e.Arch, &e.What, &e.Why, &e.URL, &e.At,
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

- [ ] **Step 3: Write `backend/internal/store/packages_test.go`**

```go
package store

import (
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

func TestUpsertQueryPackage(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	p := &model.Package{
		Project:      "isv:percona:ppg:17",
		Name:         "pg_tde",
		Scope:        model.ScopeVersion,
		RollupState:  model.RollupFailed,
		OKTargets:    4,
		TotalTargets: 6,
		Trigger: &model.Trigger{
			What: "openssl 3.2.1 → 3.2.2",
			Kind: "dependency bump",
			At:   time.Now().UTC().Truncate(time.Second),
		},
		Targets: []model.Target{
			{Repo: "EL_9", Arch: "x86_64", State: "succeeded"},
			{Repo: "Debian_12", Arch: "x86_64", State: "failed"},
		},
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := UpsertPackageState(db, p); err != nil {
		t.Fatal(err)
	}

	pkgs, err := QueryPackages(db, "isv:percona")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1, got %d", len(pkgs))
	}
	got := pkgs[0]
	if got.Name != "pg_tde" {
		t.Errorf("name: got %q", got.Name)
	}
	if got.RollupState != model.RollupFailed {
		t.Errorf("rollup_state: got %q", got.RollupState)
	}
	if got.Trigger == nil || got.Trigger.Kind != "dependency bump" {
		t.Errorf("trigger: got %+v", got.Trigger)
	}
	if len(got.Targets) != 2 {
		t.Errorf("targets: got %d", len(got.Targets))
	}
}
```

- [ ] **Step 4: Write `backend/internal/store/events_test.go`**

```go
package store

import (
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

func TestAppendQueryPruneEvents(t *testing.T) {
	db, _ := Open(":memory:")
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	e := &model.Event{
		ID:      "evt_01",
		Type:    model.EventFailed,
		Scope:   model.ScopeVersion,
		Project: "isv:percona:ppg:17",
		Package: "pg_tde",
		What:    "build failed",
		Why:     "openssl bump",
		URL:     "https://build.opensuse.org/package/show/isv:percona:ppg:17/pg_tde",
		At:      now,
	}
	if err := AppendEvent(db, e); err != nil {
		t.Fatal(err)
	}

	// Query in range
	events, err := QueryEvents(db, "isv:percona", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "evt_01" {
		t.Errorf("expected 1 event, got %d", len(events))
	}

	// Prune removes old events
	if err := PruneEvents(db, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	events, _ = QueryEvents(db, "isv:percona", now.Add(-time.Hour), now.Add(time.Hour))
	if len(events) != 0 {
		t.Errorf("expected 0 after prune, got %d", len(events))
	}
}
```

- [ ] **Step 5: Run tests and commit**

```bash
cd backend && go test ./internal/store/... -v
git add backend/internal/store/
git commit -s -m "feat(store): package and event persistence — upsert, query, prune"
```

```json:metadata
{"files": ["backend/internal/store/packages.go","backend/internal/store/events.go","backend/internal/store/packages_test.go","backend/internal/store/events_test.go"], "verifyCommand": "cd backend && go test ./internal/store/... -v", "acceptanceCriteria": ["upsert+query round-trips Package with trigger and targets","QueryEvents filters by time range","PruneEvents deletes old events"], "modelTier": "mechanical"}
```

---

### Task 6: OBS HTTP Client

**Goal:** Implement `internal/obs/client.go` — authenticated HTTP client for OBS API with methods: ListSubprojects, BuildResults, BuildLog, PackageHistory, BuildDepInfo, SourceHistory.

**Files:**
- Modify: `backend/internal/obs/client.go`
- Create: `backend/internal/obs/client_test.go`

**Acceptance Criteria:**
- [ ] `NewClient` returns a client with 30s timeout
- [ ] All requests include HTTP Basic Auth header
- [ ] `ListSubprojects("isv:percona")` parses the XML directory response
- [ ] Non-200 responses return a descriptive error

**Verify:** `cd backend && go vet ./internal/obs/...`

**Steps:**

- [ ] **Step 1: Write `backend/internal/obs/client.go`**

```go
package obs

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an authenticated OBS HTTP client.
type Client struct {
	base     string
	username string
	password string
	http     *http.Client
}

func NewClient(base, username, password string) *Client {
	return &Client{
		base:     strings.TrimRight(base, "/"),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/xml")
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

// --- XML response types ---

type directoryListing struct {
	Entries []struct {
		Name string `xml:"name,attr"`
	} `xml:"entry"`
}

type resultList struct {
	Results []buildResult `xml:"result"`
}

type buildResult struct {
	Project    string        `xml:"project,attr"`
	Repository string        `xml:"repository,attr"`
	Arch       string        `xml:"arch,attr"`
	State      string        `xml:"state,attr"`
	Statuses   []buildStatus `xml:"status"`
}

type buildStatus struct {
	Package string `xml:"package,attr"`
	Code    string `xml:"code,attr"`
}

// HistoryEntry represents one entry from /_history.
type HistoryEntry struct {
	Revision int    `xml:"rev,attr"`
	Reason   string `xml:"reason"`
}

// DepInfo represents a package dependency from /_builddepinfo.
type DepInfo struct {
	Package string   `xml:"package,attr"`
	Deps    []string `xml:"pkgdep"`
}

// SourceCommit represents one entry from /source/<project>/<pkg>/_history.
type SourceCommit struct {
	Rev     int    `xml:"rev,attr"`
	Comment string `xml:"comment"`
	Time    int64  `xml:"time"`
}

// --- Public methods ---

// ListSubprojects returns the names of direct children under root (e.g. "isv:percona").
// Returns fully-qualified project names like "isv:percona:ppg".
func (c *Client) ListSubprojects(ctx context.Context, root string) ([]string, error) {
	resp, err := c.get(ctx, "/source/"+root)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var dir directoryListing
	if err := xml.NewDecoder(resp.Body).Decode(&dir); err != nil {
		return nil, fmt.Errorf("parse /source/%s: %w", root, err)
	}

	projects := make([]string, 0, len(dir.Entries))
	for _, e := range dir.Entries {
		projects = append(projects, root+":"+e.Name)
	}
	return projects, nil
}

// BuildResults fetches all package build states for a project.
// Returns (project, repo, arch, package, state) tuples flattened from _result.
type PackageBuildState struct {
	Project string
	Repo    string
	Arch    string
	Package string
	State   string
}

func (c *Client) BuildResults(ctx context.Context, project string) ([]PackageBuildState, error) {
	resp, err := c.get(ctx, "/build/"+project+"/_result")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rl resultList
	if err := xml.NewDecoder(resp.Body).Decode(&rl); err != nil {
		return nil, fmt.Errorf("parse /build/%s/_result: %w", project, err)
	}

	var out []PackageBuildState
	for _, r := range rl.Results {
		for _, s := range r.Statuses {
			out = append(out, PackageBuildState{
				Project: project,
				Repo:    r.Repository,
				Arch:    r.Arch,
				Package: s.Package,
				State:   s.Code,
			})
		}
	}
	return out, nil
}

// BuildLog returns the tail of a package build log (last tailBytes bytes).
func (c *Client) BuildLog(ctx context.Context, project, repo, arch, pkg string, tailBytes int) (string, error) {
	path := fmt.Sprintf("/build/%s/%s/%s/%s/_log?last=1&nostream=1", project, repo, arch, pkg)
	resp, err := c.get(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(tailBytes)))
	return string(body), err
}

// PackageHistory returns build history entries for a package target.
func (c *Client) PackageHistory(ctx context.Context, project, repo, arch, pkg string) ([]HistoryEntry, error) {
	path := fmt.Sprintf("/build/%s/%s/%s/%s/_history", project, repo, arch, pkg)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var hist struct {
		Entries []HistoryEntry `xml:"entry"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&hist); err != nil {
		return nil, err
	}
	return hist.Entries, nil
}

// BuildDepInfo returns dependency info for a repo+arch.
func (c *Client) BuildDepInfo(ctx context.Context, project, repo, arch string) ([]DepInfo, error) {
	path := fmt.Sprintf("/build/%s/%s/%s/_builddepinfo", project, repo, arch)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Packages []DepInfo `xml:"package"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Packages, nil
}

// SourceHistory returns commit history for a source package.
func (c *Client) SourceHistory(ctx context.Context, project, pkg string) ([]SourceCommit, error) {
	path := fmt.Sprintf("/source/%s/%s/_history", project, pkg)
	resp, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var hist struct {
		Revisions []SourceCommit `xml:"revision"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&hist); err != nil {
		return nil, err
	}
	return hist.Revisions, nil
}
```

- [ ] **Step 2: Write `backend/internal/obs/client_test.go`** (uses httptest)

```go
package obs

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBasicAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<directory></directory>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	_, err := c.ListSubprojects(context.Background(), "isv:percona")
	if err != nil {
		t.Fatal(err)
	}

	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if gotAuth != expected {
		t.Errorf("auth header: got %q, want %q", gotAuth, expected)
	}
}

func TestListSubprojects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/source/isv:percona") {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`<directory>
			<entry name="ppg"/>
			<entry name="pmm"/>
		</directory>`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	projects, err := c.ListSubprojects(context.Background(), "isv:percona")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(projects), projects)
	}
	if projects[0] != "isv:percona:ppg" || projects[1] != "isv:percona:pmm" {
		t.Errorf("unexpected projects: %v", projects)
	}
}

func TestNon200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", 401)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "u", "p")
	_, err := c.ListSubprojects(context.Background(), "isv:percona")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}
```

- [ ] **Step 3: Run tests and commit**

```bash
cd backend && go test ./internal/obs/... -v
git add backend/internal/obs/client.go backend/internal/obs/client_test.go
git commit -s -m "feat(obs): authenticated OBS HTTP client with XML parsing"
```

```json:metadata
{"files": ["backend/internal/obs/client.go","backend/internal/obs/client_test.go"], "verifyCommand": "cd backend && go test ./internal/obs/... -v", "acceptanceCriteria": ["BasicAuth header sent on every request","ListSubprojects parses XML entry list","401 response returns error"], "modelTier": "mechanical"}
```

---

### Task 7: OBS Poller

**Goal:** Implement `internal/obs/poller.go` — discovery and reconcile loop that enumerates all `isv:percona` subprojects, fetches build results, diffs against the store, and upserts changed packages on each tick.

**Files:**
- Modify: `backend/internal/obs/poller.go`

**Acceptance Criteria:**
- [ ] `go vet ./internal/obs/...` passes
- [ ] `Poller.Run` calls `ListSubprojects` recursively and `BuildResults` per project
- [ ] On each tick, changed package states are upserted; events are appended for state transitions
- [ ] Poller logs an error and continues (does not panic) on per-project OBS errors

**Verify:** `cd backend && go vet ./internal/obs/...`

**Steps:**

- [ ] **Step 1: Write `backend/internal/obs/poller.go`**

```go
package obs

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/store"
)

// Poller periodically fetches OBS build results and reconciles them with the store.
type Poller struct {
	client   *Client
	db       *sql.DB
	interval time.Duration
	root     string
}

func NewPoller(client *Client, db *sql.DB, interval time.Duration) *Poller {
	return &Poller{client: client, db: db, interval: interval, root: "isv:percona"}
}

// Run blocks until ctx is cancelled. It ticks immediately on first call.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	projects, err := p.discoverProjects(ctx, p.root)
	if err != nil {
		slog.Error("poller: discover projects", "err", err)
		return
	}

	// Load current store state keyed by (project, package)
	existing, err := store.QueryPackages(p.db, p.root)
	if err != nil {
		slog.Error("poller: query packages", "err", err)
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
		results, err := p.client.BuildResults(ctx, project)
		if err != nil {
			slog.Warn("poller: build results", "project", project, "err", err)
			continue
		}

		// Group by package
		byPkg := map[string][]PackageBuildState{}
		for _, r := range results {
			byPkg[r.Package] = append(byPkg[r.Package], r)
		}

		scope := InferScope(project)
		for pkgName, targets := range byPkg {
			pkg := buildPackage(project, pkgName, scope, targets)
			key := project + "/" + pkgName
			prev := byKey[key]

			if prev == nil || prev.RollupState != pkg.RollupState {
				if err := store.UpsertPackageState(p.db, pkg); err != nil {
					slog.Error("poller: upsert package", "pkg", pkgName, "err", err)
					continue
				}
				evt := stateChangeEvent(pkg, prev)
				if err := store.AppendEvent(p.db, evt); err != nil {
					slog.Error("poller: append event", "err", err)
				}
			}
		}
	}
}

// discoverProjects recursively enumerates all subprojects under root.
func (p *Poller) discoverProjects(ctx context.Context, root string) ([]string, error) {
	children, err := p.client.ListSubprojects(ctx, root)
	if err != nil {
		return nil, err
	}
	all := []string{root}
	for _, child := range children {
		sub, err := p.discoverProjects(ctx, child)
		if err != nil {
			slog.Warn("poller: discover subproject", "project", child, "err", err)
			all = append(all, child)
			continue
		}
		all = append(all, sub...)
	}
	return all, nil
}

// InferScope classifies an OBS project name into a Scope tier.
func InferScope(project string) model.Scope {
	lower := strings.ToLower(project)
	switch {
	case strings.Contains(lower, "container"):
		return model.ScopeContainer
	case strings.Contains(lower, "release"):
		return model.ScopeRelease
	case strings.Contains(lower, "ppgcommon"):
		return model.ScopePPGCommon
	case strings.Contains(lower, "common"):
		return model.ScopeCommon
	default:
		// projects like isv:percona:ppg:17 have a version number
		parts := strings.Split(project, ":")
		if len(parts) >= 4 {
			return model.ScopeVersion
		}
		return model.ScopeCommon
	}
}

// buildPackage aggregates target states into a Package.
func buildPackage(project, name string, scope model.Scope, targets []PackageBuildState) *model.Package {
	stateOrder := []model.RollupState{
		model.RollupBroken, model.RollupFailed, model.RollupUnresolvable,
		model.RollupBlocked, model.RollupBuilding, model.RollupSucceeded,
	}
	stateSet := map[string]bool{}
	for _, t := range targets {
		stateSet[t.State] = true
	}

	rollup := model.RollupSucceeded
	for _, s := range stateOrder {
		if stateSet[string(s)] {
			rollup = s
			break
		}
	}

	ok := 0
	mTargets := make([]model.Target, len(targets))
	for i, t := range targets {
		mTargets[i] = model.Target{Repo: t.Repo, Arch: t.Arch, State: t.State}
		if t.State == "succeeded" {
			ok++
		}
	}

	return &model.Package{
		Project:      project,
		Name:         name,
		Scope:        scope,
		RollupState:  rollup,
		OKTargets:    ok,
		TotalTargets: len(targets),
		Targets:      mTargets,
		UpdatedAt:    time.Now().UTC(),
	}
}

func stateChangeEvent(pkg *model.Package, prev *model.Package) *model.Event {
	evtType := model.EventType(string(pkg.RollupState))
	what := fmt.Sprintf("%s %s", pkg.Name, string(pkg.RollupState))
	why := ""
	if prev != nil {
		why = fmt.Sprintf("state changed from %s", string(prev.RollupState))
	} else {
		why = "first observed"
	}
	return &model.Event{
		ID:      "evt_" + ulid.Make().String(),
		Type:    evtType,
		Scope:   pkg.Scope,
		Project: pkg.Project,
		Package: pkg.Name,
		What:    what,
		Why:     why,
		URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", pkg.Project, pkg.Name),
		At:      pkg.UpdatedAt,
	}
}
```

- [ ] **Step 2: Verify and commit**

```bash
cd backend && go vet ./internal/obs/...
git add backend/internal/obs/poller.go
git commit -s -m "feat(obs): poller — discover subprojects, reconcile build results"
```

```json:metadata
{"files": ["backend/internal/obs/poller.go"], "verifyCommand": "cd backend && go vet ./internal/obs/...", "acceptanceCriteria": ["go vet passes","Poller.Run ticks on start and on interval","per-project OBS errors are logged, not fatal"], "modelTier": "standard"}
```

---

### Task 8: Trigger Inference

**Goal:** Implement `internal/obs/trigger.go` — infer the cause of a build state change by checking `_history`, `_builddepinfo`, `/_log`, and source history in order.

**Files:**
- Modify: `backend/internal/obs/trigger.go`

**Acceptance Criteria:**
- [ ] `InferTrigger` returns a non-nil `*model.Trigger` in all code paths (including fallback)
- [ ] Tries `_history` reason field first; if it names a dependency, `Kind` = "dependency bump"
- [ ] Falls back to `Kind: "unknown"` with raw reason string when no better source found

**Verify:** `cd backend && go vet ./internal/obs/...`

**Steps:**

- [ ] **Step 1: Write `backend/internal/obs/trigger.go`**

```go
package obs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
)

// InferTrigger attempts to determine why a package build state changed.
// It tries several OBS endpoints in order and returns the best explanation found.
// Always returns a non-nil Trigger (falls back to kind:"unknown").
func InferTrigger(ctx context.Context, c *Client, pkg *model.Package) *model.Trigger {
	// Pick the first failing target for per-target endpoint calls
	var failRepo, failArch string
	for _, t := range pkg.Targets {
		if t.State != "succeeded" {
			failRepo, failArch = t.Repo, t.Arch
			break
		}
	}

	// 1. Build history reason field
	if failRepo != "" {
		entries, err := c.PackageHistory(ctx, pkg.Project, failRepo, failArch, pkg.Name)
		if err == nil && len(entries) > 0 {
			last := entries[len(entries)-1]
			if last.Reason != "" {
				kind := classifyReason(last.Reason)
				return &model.Trigger{What: last.Reason, Kind: kind, At: time.Now().UTC()}
			}
		}
	}

	// 2. Build dep info diff (compare current deps to infer what changed)
	if failRepo != "" {
		deps, err := c.BuildDepInfo(ctx, pkg.Project, failRepo, failArch)
		if err == nil {
			if what := depInfoSummary(deps, pkg.Name); what != "" {
				return &model.Trigger{What: what, Kind: "dependency bump", At: time.Now().UTC()}
			}
		}
	}

	// 3. For failed state: tail build log for compile error summary
	if pkg.RollupState == model.RollupFailed && failRepo != "" {
		log, err := c.BuildLog(ctx, pkg.Project, failRepo, failArch, pkg.Name, 4096)
		if err == nil {
			if summary := extractLogError(log); summary != "" {
				return &model.Trigger{What: summary, Kind: "build error", At: time.Now().UTC()}
			}
		}
	}

	// 4. Source history — check for recent commit
	commits, err := c.SourceHistory(ctx, pkg.Project, pkg.Name)
	if err == nil && len(commits) > 0 {
		last := commits[len(commits)-1]
		if last.Comment != "" {
			return &model.Trigger{
				What: truncate(last.Comment, 80),
				Kind: "service",
				At:   time.Unix(last.Time, 0).UTC(),
			}
		}
	}

	// 5. Fallback
	return &model.Trigger{
		What: fmt.Sprintf("%s state: %s", pkg.Name, string(pkg.RollupState)),
		Kind: "unknown",
		At:   time.Now().UTC(),
	}
}

func classifyReason(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "rebuild"):
		return "dependency bump"
	case strings.Contains(lower, "toolchain") || strings.Contains(lower, "gcc") || strings.Contains(lower, "clang"):
		return "toolchain bump"
	case strings.Contains(lower, "base image") || strings.Contains(lower, "baseimage"):
		return "base image"
	case strings.Contains(lower, "source"):
		return "service"
	default:
		return "unknown"
	}
}

// depInfoSummary returns a human-readable dep summary if deps are found, else "".
func depInfoSummary(deps []DepInfo, pkgName string) string {
	for _, d := range deps {
		if d.Package == pkgName && len(d.Deps) > 0 {
			if len(d.Deps) == 1 {
				return d.Deps[0] + " updated"
			}
			return fmt.Sprintf("%s and %d other dependencies updated", d.Deps[0], len(d.Deps)-1)
		}
	}
	return ""
}

// extractLogError extracts the first error line from a build log tail.
func extractLogError(log string) string {
	for _, line := range strings.Split(log, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error:") || strings.Contains(lower, "fatal error:") {
			trimmed := strings.TrimSpace(line)
			return truncate(trimmed, 120)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
```

- [ ] **Step 2: Verify and commit**

```bash
cd backend && go vet ./internal/obs/...
git add backend/internal/obs/trigger.go
git commit -s -m "feat(obs): trigger inference — history, depinfo, log, source history"
```

```json:metadata
{"files": ["backend/internal/obs/trigger.go"], "verifyCommand": "cd backend && go vet ./internal/obs/...", "acceptanceCriteria": ["InferTrigger always returns non-nil Trigger","classifies reason strings into known kinds","falls back to kind:unknown"], "modelTier": "standard"}
```

---

### Task 9: RabbitMQ Consumer

**Goal:** Implement `internal/mq/consumer.go` — connect to the OBS public AMQP bus, declare exchange passively, bind with `opensuse.obs.package.#`, filter to `isv:percona` projects, update store on build events.

**Files:**
- Modify: `backend/internal/mq/consumer.go`

**Acceptance Criteria:**
- [ ] `go vet ./internal/mq/...` passes
- [ ] Consumer filters out messages where `project` does not start with `isv:percona`
- [ ] Reconnects with exponential back-off on connection loss (max 30s)
- [ ] `build_success`/`build_fail`/`build_unchanged` events call `UpsertPackageState` + `AppendEvent`

**Verify:** `cd backend && go vet ./internal/mq/...`

**Steps:**

- [ ] **Step 1: Write `backend/internal/mq/consumer.go`**

```go
package mq

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/oklog/ulid/v2"
	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/store"
)

const (
	exchange   = "pubsub"
	routingKey = "opensuse.obs.package.#"
	repoKey    = "opensuse.obs.repo.published"
)

// mqMessage is the JSON structure of OBS MQ events.
type mqMessage struct {
	Project  string `json:"project"`
	Package  string `json:"package"`
	Repo     string `json:"repository"`
	Arch     string `json:"arch"`
	Reason   string `json:"reason"`
	BuildID  string `json:"buildid"`
}

// Consumer subscribes to the OBS AMQP bus and updates the store on build events.
type Consumer struct {
	url string
	db  *sql.DB
}

func NewConsumer(url string, db *sql.DB) *Consumer {
	return &Consumer{url: url, db: db}
}

// Run blocks until ctx is cancelled, reconnecting on errors.
func (c *Consumer) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if err := c.run(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("mq: disconnected, reconnecting", "err", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = time.Duration(math.Min(float64(backoff*2), float64(30*time.Second)))
		} else {
			backoff = time.Second
		}
	}
}

func (c *Consumer) run(ctx context.Context) error {
	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("channel: %w", err)
	}
	defer ch.Close()

	// Passive declare — exchange already exists
	if err := ch.ExchangeDeclarePassive(exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("exchange declare: %w", err)
	}

	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return fmt.Errorf("queue declare: %w", err)
	}

	for _, key := range []string{routingKey, repoKey} {
		if err := ch.QueueBind(q.Name, key, exchange, false, nil); err != nil {
			return fmt.Errorf("queue bind %s: %w", key, err)
		}
	}

	msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	connClose := conn.NotifyClose(make(chan *amqp.Error, 1))
	slog.Info("mq: connected", "exchange", exchange)

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-connClose:
			if err != nil {
				return fmt.Errorf("connection closed: %w", err)
			}
			return nil
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("channel closed")
			}
			c.handle(msg)
		}
	}
}

func (c *Consumer) handle(msg amqp.Delivery) {
	var m mqMessage
	if err := json.Unmarshal(msg.Body, &m); err != nil {
		slog.Debug("mq: unparseable message", "err", err)
		return
	}

	// Filter to isv:percona only
	if len(m.Project) < 11 || m.Project[:11] != "isv:percona" {
		return
	}

	routingKey := msg.RoutingKey
	switch {
	case routingKey == "opensuse.obs.repo.published":
		evt := &model.Event{
			ID:      "evt_" + ulid.Make().String(),
			Type:    model.EventPublished,
			Scope:   model.ScopeRelease,
			Project: m.Project,
			Package: m.Package,
			Repo:    m.Repo,
			What:    fmt.Sprintf("%s published", m.Repo),
			Why:     "repo published",
			URL:     fmt.Sprintf("https://build.opensuse.org/project/show/%s", m.Project),
			At:      time.Now().UTC(),
		}
		if err := store.AppendEvent(c.db, evt); err != nil {
			slog.Error("mq: append event", "err", err)
		}

	case isPackageBuildEvent(routingKey):
		state := mqStateToRollup(routingKey)
		pkg := &model.Package{
			Project:     m.Project,
			Name:        m.Package,
			Scope:       inferScopeFromProject(m.Project),
			RollupState: state,
			Targets: []model.Target{
				{Repo: m.Repo, Arch: m.Arch, State: string(state)},
			},
			UpdatedAt: time.Now().UTC(),
		}
		if err := store.UpsertPackageState(c.db, pkg); err != nil {
			slog.Error("mq: upsert package", "err", err)
			return
		}
		evt := &model.Event{
			ID:      "evt_" + ulid.Make().String(),
			Type:    model.EventType(state),
			Scope:   pkg.Scope,
			Project: m.Project,
			Package: m.Package,
			Repo:    m.Repo,
			Arch:    m.Arch,
			What:    fmt.Sprintf("%s %s on %s/%s", m.Package, string(state), m.Repo, m.Arch),
			Why:     m.Reason,
			URL:     fmt.Sprintf("https://build.opensuse.org/package/show/%s/%s", m.Project, m.Package),
			At:      time.Now().UTC(),
		}
		if err := store.AppendEvent(c.db, evt); err != nil {
			slog.Error("mq: append event", "err", err)
		}
	}
}

func isPackageBuildEvent(key string) bool {
	return key == "opensuse.obs.package.build_success" ||
		key == "opensuse.obs.package.build_fail" ||
		key == "opensuse.obs.package.build_unchanged"
}

func mqStateToRollup(key string) model.RollupState {
	switch key {
	case "opensuse.obs.package.build_success":
		return model.RollupSucceeded
	case "opensuse.obs.package.build_fail":
		return model.RollupFailed
	default:
		return model.RollupSucceeded
	}
}

func inferScopeFromProject(project string) model.Scope {
	switch {
	case len(project) >= 11 && project[:11] == "isv:percona":
		// delegate to obs.InferScope logic duplicated here to avoid circular import
		if contains(project, "container") {
			return model.ScopeContainer
		}
		if contains(project, "release") {
			return model.ScopeRelease
		}
		if contains(project, "ppgcommon") {
			return model.ScopePPGCommon
		}
		if contains(project, "common") {
			return model.ScopeCommon
		}
		return model.ScopeVersion
	default:
		return model.ScopeCommon
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Verify and commit**

```bash
cd backend && go vet ./internal/mq/...
git add backend/internal/mq/consumer.go
git commit -s -m "feat(mq): AMQP consumer with exponential backoff reconnect"
```

```json:metadata
{"files": ["backend/internal/mq/consumer.go"], "verifyCommand": "cd backend && go vet ./internal/mq/...", "acceptanceCriteria": ["go vet passes","non-isv:percona messages are silently dropped","build_success/fail/unchanged update store and append event"], "modelTier": "standard"}
```

---

### Task 10: HTTP API

**Goal:** Implement `internal/api/server.go` (router setup) and `internal/api/handlers.go` (GET /packages and GET /events handlers) matching the spec JSON shapes exactly.

**Files:**
- Modify: `backend/internal/api/server.go`
- Modify: `backend/internal/api/handlers.go`
- Create: `backend/internal/api/handlers_test.go`

**Acceptance Criteria:**
- [ ] `GET /api/products/ppg/17/packages` returns JSON array of Package objects
- [ ] `GET /api/products/ppg/17/events?window=24h` returns events within the last 24h
- [ ] `GET /api/products/ppg/17/events?from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z` returns events within date range
- [ ] Static files served from `FrontendDir` when set; 404 otherwise

**Verify:** `cd backend && go test ./internal/api/... -v`

**Steps:**

- [ ] **Step 1: Write `backend/internal/api/server.go`**

```go
package api

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(db *sql.DB, frontendDir string) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	h := &handlers{db: db}
	r.Route("/api/products/{product}/{version}", func(r chi.Router) {
		r.Get("/packages", h.getPackages)
		r.Get("/events", h.getEvents)
	})

	if frontendDir != "" {
		fs := http.FileServer(http.Dir(frontendDir))
		r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fs.ServeHTTP(w, r)
		}))
	}

	return r
}
```

- [ ] **Step 2: Write `backend/internal/api/handlers.go`**

```go
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/percona/obs-dashboard/internal/store"
)

type handlers struct {
	db *sql.DB
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// obsRoot derives the OBS project prefix from product + version URL params.
// e.g. product="ppg" version="17" → "isv:percona:ppg:17"
func obsRoot(product, version string) string {
	return "isv:percona:" + product + ":" + version
}

func (h *handlers) getPackages(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")
	prefix := obsRoot(product, version)

	pkgs, err := store.QueryPackages(h.db, prefix)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if pkgs == nil {
		pkgs = []*struct{}{} // return [] not null
	}
	writeJSON(w, pkgs)
}

func (h *handlers) getEvents(w http.ResponseWriter, r *http.Request) {
	product := chi.URLParam(r, "product")
	version := chi.URLParam(r, "version")
	prefix := obsRoot(product, version)

	var from, to time.Time

	if windowStr := r.URL.Query().Get("window"); windowStr != "" {
		// window=24h, 3d, 7d etc.
		dur, err := parseWindow(windowStr)
		if err != nil {
			http.Error(w, "invalid window parameter", http.StatusBadRequest)
			return
		}
		to = time.Now().UTC()
		from = to.Add(-dur)
	} else {
		fromStr := r.URL.Query().Get("from")
		toStr := r.URL.Query().Get("to")
		if fromStr == "" || toStr == "" {
			// Default to 24h window
			to = time.Now().UTC()
			from = to.Add(-24 * time.Hour)
		} else {
			var err error
			from, err = time.Parse(time.RFC3339, fromStr)
			if err != nil {
				http.Error(w, "invalid from parameter", http.StatusBadRequest)
				return
			}
			to, err = time.Parse(time.RFC3339, toStr)
			if err != nil {
				http.Error(w, "invalid to parameter", http.StatusBadRequest)
				return
			}
		}
	}

	events, err := store.QueryEvents(h.db, prefix, from, to)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []*struct{}{} // return [] not null
	}
	writeJSON(w, events)
}

func parseWindow(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
```

- [ ] **Step 3: Fix the nil-slice JSON issue** — update `QueryPackages` and `QueryEvents` in the store to return empty slices, not nil, when no results. Open `backend/internal/store/packages.go` and change:

```go
var pkgs []*model.Package
```
to:
```go
pkgs := make([]*model.Package, 0)
```

Do the same in `store/events.go`:
```go
events := make([]*model.Event, 0)
```

- [ ] **Step 4: Write `backend/internal/api/handlers_test.go`**

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/store"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestGetPackagesEmpty(t *testing.T) {
	db := newTestDB(t)
	router := NewRouter(db, "")
	req := httptest.NewRequest("GET", "/api/products/ppg/17/packages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d", w.Code)
	}
	var pkgs []any
	json.NewDecoder(w.Body).Decode(&pkgs)
	if len(pkgs) != 0 {
		t.Errorf("expected empty array, got %d items", len(pkgs))
	}
}

func TestGetPackagesReturnsData(t *testing.T) {
	db := newTestDB(t)
	store.UpsertPackageState(db, &model.Package{
		Project: "isv:percona:ppg:17", Name: "pg_tde",
		Scope: model.ScopeVersion, RollupState: model.RollupFailed,
		Targets: []model.Target{{Repo: "EL_9", Arch: "x86_64", State: "failed"}},
		UpdatedAt: time.Now().UTC(),
	})
	router := NewRouter(db, "")
	req := httptest.NewRequest("GET", "/api/products/ppg/17/packages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	var pkgs []map[string]any
	json.NewDecoder(w.Body).Decode(&pkgs)
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if pkgs[0]["name"] != "pg_tde" {
		t.Errorf("name: %v", pkgs[0]["name"])
	}
}

func TestGetEventsWindowParam(t *testing.T) {
	db := newTestDB(t)
	router := NewRouter(db, "")
	req := httptest.NewRequest("GET", "/api/products/ppg/17/events?window=24h", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status: %d — %s", w.Code, w.Body.String())
	}
}
```

Fix the import of `database/sql` in the test file:
```go
import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/percona/obs-dashboard/internal/model"
	"github.com/percona/obs-dashboard/internal/store"
)
```

- [ ] **Step 5: Run tests and commit**

```bash
cd backend && go test ./internal/api/... -v
git add backend/internal/api/ backend/internal/store/packages.go backend/internal/store/events.go
git commit -s -m "feat(api): HTTP router and packages/events handlers"
```

```json:metadata
{"files": ["backend/internal/api/server.go","backend/internal/api/handlers.go","backend/internal/api/handlers_test.go"], "verifyCommand": "cd backend && go test ./internal/api/... -v", "acceptanceCriteria": ["GET /packages returns 200 with JSON array","GET /events?window=24h returns 200","GET /events?from=&to= returns 200"], "modelTier": "mechanical"}
```

---

### Task 11: Backend Main — Wire Everything

**Goal:** Implement `cmd/obsboard/main.go` — load config, open DB, start MQ consumer and OBS poller as goroutines, serve HTTP, handle graceful shutdown on SIGTERM/SIGINT.

**Files:**
- Modify: `backend/cmd/obsboard/main.go`

**Acceptance Criteria:**
- [ ] `go build ./cmd/obsboard` succeeds
- [ ] Binary exits cleanly on SIGINT within 5 seconds
- [ ] Missing `OBS_USERNAME` causes a startup error log and exit code 1

**Verify:** `cd backend && go build ./cmd/obsboard && echo OK`

**Steps:**

- [ ] **Step 1: Write `backend/cmd/obsboard/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/percona/obs-dashboard/internal/api"
	"github.com/percona/obs-dashboard/internal/config"
	"github.com/percona/obs-dashboard/internal/mq"
	"github.com/percona/obs-dashboard/internal/obs"
	"github.com/percona/obs-dashboard/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := store.Open(cfg.Store.DBPath)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	obsClient := obs.NewClient(cfg.OBS.BaseURL, cfg.OBS.Username, cfg.OBS.Password)
	poller := obs.NewPoller(obsClient, db, cfg.Poller.Interval)
	consumer := mq.NewConsumer(cfg.MQ.URL, db)

	go poller.Run(ctx)
	go consumer.Run(ctx)

	// Prune old events on each poller tick (piggyback on the same interval)
	go func() {
		ticker := time.NewTicker(cfg.Poller.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-cfg.Store.EventRetention)
				if err := store.PruneEvents(db, cutoff); err != nil {
					slog.Warn("prune events", "err", err)
				}
			}
		}
	}()

	router := api.NewRouter(db, cfg.Server.FrontendDir)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: router,
	}

	go func() {
		slog.Info("http server starting", "port", cfg.Server.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}
```

- [ ] **Step 2: Build and commit**

```bash
cd backend && go build ./cmd/obsboard && echo "Build OK"
git add backend/cmd/obsboard/main.go
git commit -s -m "feat: wire backend — config, store, poller, consumer, HTTP server"
```

```json:metadata
{"files": ["backend/cmd/obsboard/main.go"], "verifyCommand": "cd backend && go build ./cmd/obsboard && echo OK", "acceptanceCriteria": ["go build succeeds","graceful shutdown on SIGINT","missing OBS_USERNAME exits with code 1"], "modelTier": "standard"}
```

---

### Task 12: Frontend Scaffold — Vite + Tailwind + Theme

**Goal:** Complete the frontend build setup: Tailwind config with CSS variable theme tokens, `theme.css` with light and dark variables (ported from HTML mockup), and self-hosted Roboto font declarations.

**Files:**
- Modify: `frontend/tailwind.config.ts`
- Create: `frontend/src/assets/theme.css`
- Modify: `frontend/src/main.ts`

**Acceptance Criteria:**
- [ ] `npm run build` succeeds with no TypeScript errors
- [ ] `bg-card`, `text-ok`, `text-fail`, `border-brand-purple` Tailwind utilities resolve to CSS variables
- [ ] `data-theme="dark"` on `<html>` switches to dark palette

**Verify:** `cd frontend && npm run build && echo OK`

**Steps:**

- [ ] **Step 1: Write `frontend/src/assets/theme.css`**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

:root {
  --bg-app: #F6F4F0;
  --bg-card: #FFFFFF;
  --bg-card-hover: #F8F7F5;
  --brand-purple: #6E3FF3;
  --brand-purple-tint: #EDE8FD;

  --ok: #1F9D55;
  --ok-tint: #D1FAE5;
  --fail: #E5484D;
  --fail-tint: #FFE4E6;
  --warn: #E08A00;
  --warn-tint: #FEF3C7;
  --broken: #B0203A;
  --broken-tint: #FFE4EE;
  --blocked: #8A8594;
  --blocked-tint: #F1F0F5;
  --info: #3B82F6;
  --info-tint: #DBEAFE;
  --building: #F59E0B;
  --building-tint: #FEF3C7;

  --text-primary: #1A1523;
  --text-secondary: #6B7280;
  --text-muted: #9CA3AF;
  --border: #E5E7EB;
  --border-strong: #D1D5DB;
}

[data-theme="dark"] {
  --bg-app: #0B0912;
  --bg-card: #181426;
  --bg-card-hover: #1E1A30;
  --brand-purple: #8B5CF6;
  --brand-purple-tint: #2D1F5E;

  --ok: #42C97E;
  --ok-tint: #0D3321;
  --fail: #FF6166;
  --fail-tint: #3D0D0E;
  --warn: #F0A52A;
  --warn-tint: #3D2700;
  --broken: #FF5E7A;
  --broken-tint: #3D0016;
  --blocked: #9C97A8;
  --blocked-tint: #1A1826;
  --info: #5BA0F0;
  --info-tint: #0D1F3D;
  --building: #FBB040;
  --building-tint: #3D2700;

  --text-primary: #F0EDF8;
  --text-secondary: #A09DB0;
  --text-muted: #6B6880;
  --border: #2A263A;
  --border-strong: #3D3850;
}

body {
  background-color: var(--bg-app);
  color: var(--text-primary);
  font-family: 'Roboto', -apple-system, 'Segoe UI', Arial, sans-serif;
}
```

- [ ] **Step 2: Update `frontend/tailwind.config.ts`** with CSS variable theme tokens

```ts
import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{vue,ts}'],
  theme: {
    extend: {
      colors: {
        'bg-app':    'var(--bg-app)',
        'bg-card':   'var(--bg-card)',
        'brand-purple': 'var(--brand-purple)',
        'brand-purple-tint': 'var(--brand-purple-tint)',
        'ok':        'var(--ok)',
        'ok-tint':   'var(--ok-tint)',
        'fail':      'var(--fail)',
        'fail-tint': 'var(--fail-tint)',
        'warn':      'var(--warn)',
        'warn-tint': 'var(--warn-tint)',
        'broken':    'var(--broken)',
        'broken-tint': 'var(--broken-tint)',
        'blocked':   'var(--blocked)',
        'blocked-tint': 'var(--blocked-tint)',
        'info':      'var(--info)',
        'info-tint': 'var(--info-tint)',
        'building':  'var(--building)',
        'building-tint': 'var(--building-tint)',
        'text-primary':   'var(--text-primary)',
        'text-secondary': 'var(--text-secondary)',
        'text-muted':     'var(--text-muted)',
        'border-color':   'var(--border)',
      },
    },
  },
  plugins: [],
} satisfies Config
```

- [ ] **Step 3: Build and commit**

```bash
cd frontend && npm run build && echo "Tailwind OK"
git add frontend/src/assets/theme.css frontend/tailwind.config.ts frontend/src/main.ts
git commit -s -m "feat(frontend): Tailwind CSS setup with CSS variable theme tokens"
```

```json:metadata
{"files": ["frontend/src/assets/theme.css","frontend/tailwind.config.ts"], "verifyCommand": "cd frontend && npm run build && echo OK", "acceptanceCriteria": ["npm run build succeeds","Tailwind color tokens resolve to CSS variables","dark theme variables defined"], "modelTier": "mechanical"}
```

---

### Task 13: Frontend API Types and Composables

**Goal:** Define TypeScript types matching the backend API response shapes and implement `usePackages` and `useEvents` composables for data fetching.

**Files:**
- Create: `frontend/src/types/api.ts`
- Create: `frontend/src/composables/usePackages.ts`
- Create: `frontend/src/composables/useEvents.ts`

**Acceptance Criteria:**
- [ ] `npm run build` succeeds with no TypeScript errors
- [ ] `usePackages` fetches `/api/products/:product/:version/packages`
- [ ] `useEvents` accepts `windowMin` (minutes as string) or `customStart`/`customEnd` date strings
- [ ] Both composables expose `data`, `loading`, `error`, and a `refresh()` function

**Verify:** `cd frontend && npm run build && echo OK`

**Steps:**

- [ ] **Step 1: Write `frontend/src/types/api.ts`**

```ts
export type RollupState = 'failed' | 'broken' | 'unresolvable' | 'blocked' | 'building' | 'succeeded'
export type Scope = 'common' | 'ppgcommon' | 'version' | 'container' | 'release'
export type EventType = 'triggered' | 'started' | 'succeeded' | 'failed' | 'unresolvable' | 'broken' | 'blocked' | 'published'

export interface Target {
  repo: string
  arch: string
  state: string
}

export interface Trigger {
  what: string
  kind: string
  at: string // ISO 8601
}

export interface Package {
  project: string
  name: string
  scope: Scope
  rollup_state: RollupState
  ok_targets: number
  total_targets: number
  trigger?: Trigger
  targets: Target[]
  updated_at: string
}

export interface OBSEvent {
  id: string
  type: EventType
  scope: Scope
  project: string
  package: string
  repo?: string
  arch?: string
  what: string
  why: string
  url: string
  at: string // ISO 8601
}

export const STATE_SEVERITY: Record<RollupState, number> = {
  broken: 5,
  failed: 4,
  unresolvable: 3,
  blocked: 2,
  building: 1,
  succeeded: 0,
}
```

- [ ] **Step 2: Write `frontend/src/composables/usePackages.ts`**

```ts
import { ref, computed, watch } from 'vue'
import type { Package, Scope } from '../types/api'
import { STATE_SEVERITY } from '../types/api'

export function usePackages(
  product: () => string,
  version: () => string,
  scopes: () => Scope[],
) {
  const data = ref<Package[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      const res = await fetch(`/api/products/${product()}/${version()}/packages`)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      data.value = await res.json()
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  const filtered = computed(() => {
    const activeScopes = scopes()
    const pkgs = activeScopes.length === 0
      ? data.value
      : data.value.filter(p => activeScopes.includes(p.scope))
    return [...pkgs].sort((a, b) =>
      STATE_SEVERITY[b.rollup_state] - STATE_SEVERITY[a.rollup_state]
    )
  })

  const failCount = computed(() => filtered.value.filter(p => p.rollup_state !== 'succeeded').length)
  const okCount = computed(() => filtered.value.filter(p => p.rollup_state === 'succeeded').length)

  return { data: filtered, loading, error, failCount, okCount, refresh }
}
```

- [ ] **Step 3: Write `frontend/src/composables/useEvents.ts`**

```ts
import { ref } from 'vue'
import type { OBSEvent } from '../types/api'

export function useEvents(
  product: () => string,
  version: () => string,
  windowMin: () => string,       // minutes as string, e.g. "1440" for 24h
  customStart: () => string | null,
  customEnd: () => string | null,
) {
  const data = ref<OBSEvent[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function refresh() {
    loading.value = true
    error.value = null
    try {
      let url = `/api/products/${product()}/${version()}/events`
      const start = customStart()
      const end = customEnd()
      if (start && end) {
        url += `?from=${encodeURIComponent(start)}&to=${encodeURIComponent(end)}`
      } else {
        const mins = parseInt(windowMin(), 10)
        const hours = Math.round(mins / 60)
        url += `?window=${hours}h`
      }
      const res = await fetch(url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      data.value = await res.json()
    } catch (e) {
      error.value = String(e)
    } finally {
      loading.value = false
    }
  }

  return { data, loading, error, refresh }
}
```

- [ ] **Step 4: Build and commit**

```bash
cd frontend && npm run build && echo OK
git add frontend/src/types/ frontend/src/composables/
git commit -s -m "feat(frontend): API types and usePackages/useEvents composables"
```

```json:metadata
{"files": ["frontend/src/types/api.ts","frontend/src/composables/usePackages.ts","frontend/src/composables/useEvents.ts"], "verifyCommand": "cd frontend && npm run build && echo OK", "acceptanceCriteria": ["TypeScript types match backend JSON shapes","usePackages filters by scope and sorts by severity","useEvents builds correct query string for window and custom range"], "modelTier": "mechanical"}
```

---

### Task 14: AppHeader, ContextBar, ScopeChip

**Goal:** Implement the top section of the dashboard: Percona logo + title + dark mode toggle (`AppHeader`), PostgreSQL badge + version tabs + OBS root + scope chips row (`ContextBar` + `ScopeChip`).

**Files:**
- Create: `frontend/src/components/AppHeader.vue`
- Create: `frontend/src/components/ScopeChip.vue`
- Create: `frontend/src/components/ContextBar.vue`

**Acceptance Criteria:**
- [ ] `npm run build` succeeds
- [ ] Clicking the dark mode toggle emits `toggle-theme`
- [ ] Version tabs emit `update:version` with `'16'`, `'17'`, or `'18'`
- [ ] ScopeChip emits `toggle` with its scope string when clicked; active state is visually distinct

**Verify:** `cd frontend && npm run build && echo OK`

**Steps:**

- [ ] **Step 1: Write `frontend/src/components/ScopeChip.vue`**

```vue
<template>
  <button
    class="px-3 py-1 rounded-full text-xs font-medium transition-colors border"
    :class="active
      ? 'bg-brand-purple text-white border-brand-purple'
      : 'bg-bg-card text-text-secondary border-border-color hover:border-brand-purple hover:text-text-primary'"
    @click="$emit('toggle', scope)"
  >
    {{ scope }}
  </button>
</template>

<script setup lang="ts">
import type { Scope } from '../types/api'
defineProps<{ scope: Scope | 'all'; active: boolean }>()
defineEmits<{ toggle: [scope: string] }>()
</script>
```

- [ ] **Step 2: Write `frontend/src/components/AppHeader.vue`**

```vue
<template>
  <header class="flex items-center justify-between px-6 py-3 bg-bg-card border-b border-border-color">
    <div class="flex items-center gap-3">
      <!-- Percona "P" logo -->
      <div class="w-7 h-7 rounded bg-brand-purple flex items-center justify-center text-white font-bold text-sm select-none">P</div>
      <span class="text-base font-semibold text-text-primary tracking-tight">PPG Build Board</span>
    </div>
    <button
      class="w-8 h-8 rounded flex items-center justify-center text-text-secondary hover:text-text-primary hover:bg-bg-card-hover transition-colors"
      :title="theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'"
      @click="$emit('toggle-theme')"
    >
      <!-- Sun icon for dark mode, Moon for light -->
      <svg v-if="theme === 'dark'" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
    </button>
  </header>
</template>

<script setup lang="ts">
defineProps<{ theme: 'light' | 'dark' }>()
defineEmits<{ 'toggle-theme': [] }>()
</script>
```

- [ ] **Step 3: Write `frontend/src/components/ContextBar.vue`**

```vue
<template>
  <div class="bg-bg-card border-b border-border-color px-6 py-3 flex flex-wrap items-center gap-4">
    <!-- PostgreSQL badge -->
    <span class="px-2 py-0.5 rounded text-xs font-semibold bg-info-tint text-info">PostgreSQL</span>

    <!-- Version tabs -->
    <div class="flex gap-1">
      <button
        v-for="v in ['17', '18', '16']"
        :key="v"
        class="px-3 py-1 rounded text-xs font-medium transition-colors"
        :class="version === v
          ? 'bg-brand-purple text-white'
          : 'bg-bg-card text-text-secondary border border-border-color hover:text-text-primary'"
        @click="$emit('update:version', v)"
      >
        v{{ v }}
      </button>
    </div>

    <!-- OBS root -->
    <span class="text-xs text-text-muted font-mono">isv:percona:ppg:{{ version }}</span>

    <!-- Spacer -->
    <div class="flex-1" />

    <!-- Updated timestamp + refresh indicator -->
    <span class="text-xs text-text-muted">{{ updatedLabel }}</span>

    <!-- Scope chips -->
    <div class="w-full flex flex-wrap gap-2 mt-2">
      <ScopeChip scope="all" :active="activeScopes.length === 0" @toggle="toggleScope('all')" />
      <ScopeChip v-for="s in allScopes" :key="s" :scope="s" :active="activeScopes.includes(s)" @toggle="toggleScope(s)" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import ScopeChip from './ScopeChip.vue'
import type { Scope } from '../types/api'

const props = defineProps<{
  version: string
  activeScopes: Scope[]
  updatedAt: string | null
}>()

const emit = defineEmits<{
  'update:version': [v: string]
  'update:scopes': [scopes: Scope[]]
}>()

const allScopes: Scope[] = ['common', 'ppgcommon', 'version', 'container', 'release']

const updatedLabel = computed(() => {
  if (!props.updatedAt) return 'never updated'
  const d = new Date(props.updatedAt)
  return `updated ${d.toLocaleTimeString()}`
})

function toggleScope(s: string) {
  if (s === 'all') {
    emit('update:scopes', [])
    return
  }
  const scope = s as Scope
  const current = props.activeScopes
  if (current.includes(scope)) {
    emit('update:scopes', current.filter(x => x !== scope))
  } else {
    emit('update:scopes', [...current, scope])
  }
}
</script>
```

- [ ] **Step 4: Build and commit**

```bash
cd frontend && npm run build && echo OK
git add frontend/src/components/AppHeader.vue frontend/src/components/ScopeChip.vue frontend/src/components/ContextBar.vue
git commit -s -m "feat(frontend): AppHeader, ContextBar, ScopeChip components"
```

```json:metadata
{"files": ["frontend/src/components/AppHeader.vue","frontend/src/components/ScopeChip.vue","frontend/src/components/ContextBar.vue"], "verifyCommand": "cd frontend && npm run build && echo OK", "acceptanceCriteria": ["build succeeds","dark mode toggle emits toggle-theme","version tabs emit update:version","ScopeChip active state toggles correctly"], "modelTier": "mechanical"}
```

---

### Task 15: HealthHeader

**Goal:** Implement `HealthHeader.vue` — the summary bar showing "X/Y packages built", a progress bar, "N need attention" count, and breakdown pills for each failure type.

**Files:**
- Create: `frontend/src/components/HealthHeader.vue`

**Acceptance Criteria:**
- [ ] `npm run build` succeeds
- [ ] Progress bar width = `(okCount / total) * 100%`; fills green, remainder fail-colored
- [ ] Breakdown pills only render when count > 0

**Verify:** `cd frontend && npm run build && echo OK`

**Steps:**

- [ ] **Step 1: Write `frontend/src/components/HealthHeader.vue`**

```vue
<template>
  <div class="bg-bg-card border-b border-border-color px-6 py-4">
    <div class="flex items-baseline gap-4 mb-3">
      <span class="text-2xl font-bold text-text-primary">
        {{ okCount }}<span class="text-text-muted text-lg">/{{ total }}</span>
      </span>
      <span class="text-sm text-text-secondary">packages built</span>
      <span v-if="attentionCount > 0" class="ml-2 text-sm font-medium text-fail">
        ⚠ {{ attentionCount }} need attention
      </span>
      <span v-else class="ml-2 text-sm font-medium text-ok">✓ All green</span>
    </div>

    <!-- Progress bar -->
    <div class="h-1.5 rounded-full bg-fail-tint overflow-hidden mb-3">
      <div
        class="h-full rounded-full bg-ok transition-all duration-500"
        :style="{ width: progressPct + '%' }"
      />
    </div>

    <!-- Breakdown pills -->
    <div class="flex flex-wrap gap-2">
      <span v-if="counts.broken > 0" class="px-2 py-0.5 rounded text-xs font-medium bg-broken-tint text-broken">
        {{ counts.broken }} broken
      </span>
      <span v-if="counts.failed > 0" class="px-2 py-0.5 rounded text-xs font-medium bg-fail-tint text-fail">
        {{ counts.failed }} failed
      </span>
      <span v-if="counts.unresolvable > 0" class="px-2 py-0.5 rounded text-xs font-medium bg-warn-tint text-warn">
        {{ counts.unresolvable }} unresolvable
      </span>
      <span v-if="counts.blocked > 0" class="px-2 py-0.5 rounded text-xs font-medium bg-blocked-tint text-blocked">
        {{ counts.blocked }} blocked
      </span>
      <span v-if="counts.building > 0" class="px-2 py-0.5 rounded text-xs font-medium bg-building-tint text-building">
        {{ counts.building }} building
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Package, RollupState } from '../types/api'

const props = defineProps<{ packages: Package[] }>()

const total = computed(() => props.packages.length)
const okCount = computed(() => props.packages.filter(p => p.rollup_state === 'succeeded').length)
const attentionCount = computed(() => total.value - okCount.value)
const progressPct = computed(() => total.value === 0 ? 100 : (okCount.value / total.value) * 100)

const counts = computed(() => {
  const c: Record<RollupState, number> = { broken: 0, failed: 0, unresolvable: 0, blocked: 0, building: 0, succeeded: 0 }
  for (const p of props.packages) c[p.rollup_state]++
  return c
})
</script>
```

- [ ] **Step 2: Build and commit**

```bash
cd frontend && npm run build && echo OK
git add frontend/src/components/HealthHeader.vue
git commit -s -m "feat(frontend): HealthHeader with progress bar and failure breakdown pills"
```

```json:metadata
{"files": ["frontend/src/components/HealthHeader.vue"], "verifyCommand": "cd frontend && npm run build && echo OK", "acceptanceCriteria": ["build succeeds","progress bar width computed from ok/total","breakdown pills only render when count > 0"], "modelTier": "mechanical"}
```

---

### Task 16: PackageCard, GreenStrip, FailureBoard

**Goal:** Implement the left column of the main grid: `PackageCard` (per-package failure card with target grid), `GreenStrip` (collapsed "N built" strip), and `FailureBoard` (container that renders them).

**Files:**
- Create: `frontend/src/components/PackageCard.vue`
- Create: `frontend/src/components/GreenStrip.vue`
- Create: `frontend/src/components/FailureBoard.vue`

**Acceptance Criteria:**
- [ ] `npm run build` succeeds
- [ ] PackageCard renders a target grid; cells are colored by state using CSS vars
- [ ] Failing targets list shows first 3; remaining count shown as "N more" when > 3
- [ ] GreenStrip shows ok package count; FailureBoard renders failure cards above it

**Verify:** `cd frontend && npm run build && echo OK`

**Steps:**

- [ ] **Step 1: Write `frontend/src/components/PackageCard.vue`**

```vue
<template>
  <div
    class="bg-bg-card rounded-lg border-l-4 p-4 shadow-sm"
    :class="borderClass"
  >
    <!-- Header -->
    <div class="flex items-center justify-between mb-2">
      <div class="flex items-center gap-2">
        <span class="text-sm font-semibold text-text-primary">{{ pkg.name }}</span>
        <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-blocked-tint text-text-muted uppercase tracking-wide">
          {{ pkg.scope }}
        </span>
      </div>
      <span class="px-2 py-0.5 rounded text-xs font-semibold" :class="badgeClass">
        {{ pkg.rollup_state }}
      </span>
    </div>

    <!-- Trigger line -->
    <div v-if="pkg.trigger" class="text-xs text-text-muted mb-3">
      ↻ {{ pkg.trigger.what }}
      <span class="mx-1 opacity-50">·</span>
      <span class="italic">{{ pkg.trigger.kind }}</span>
      <span class="mx-1 opacity-50">·</span>
      {{ timeAgo(pkg.trigger.at) }}
    </div>

    <!-- Target grid -->
    <div class="grid gap-1 mb-3" :style="{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }">
      <div
        v-for="t in pkg.targets"
        :key="t.repo + '/' + t.arch"
        class="rounded text-[10px] font-medium flex items-center justify-center h-6 px-1 truncate"
        :class="targetCellClass(t.state)"
        :title="`${t.repo}/${t.arch}: ${t.state}`"
      >
        {{ shortLabel(t.repo) }}
      </div>
    </div>

    <!-- Failing targets list -->
    <div v-if="failingTargets.length > 0" class="text-xs space-y-1">
      <div
        v-for="t in shownFailing"
        :key="t.repo + '/' + t.arch"
        class="text-fail"
      >
        {{ t.repo }}/{{ t.arch }}: <span class="text-text-muted">{{ t.state }}</span>
      </div>
      <div v-if="hiddenCount > 0" class="text-text-muted">
        +{{ hiddenCount }} more
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Package } from '../types/api'

const props = defineProps<{ pkg: Package }>()

const STATE_COLORS: Record<string, string> = {
  succeeded:    'bg-ok-tint text-ok',
  failed:       'bg-fail-tint text-fail',
  unresolvable: 'bg-warn-tint text-warn',
  broken:       'bg-broken-tint text-broken',
  blocked:      'bg-blocked-tint text-blocked',
  building:     'bg-building-tint text-building',
  scheduled:    'bg-info-tint text-info',
  excluded:     'bg-bg-card text-text-muted',
}

const BADGE_COLORS: Record<string, string> = {
  succeeded:    'bg-ok-tint text-ok',
  failed:       'bg-fail-tint text-fail',
  unresolvable: 'bg-warn-tint text-warn',
  broken:       'bg-broken-tint text-broken',
  blocked:      'bg-blocked-tint text-blocked',
  building:     'bg-building-tint text-building',
}

const BORDER_COLORS: Record<string, string> = {
  succeeded:    'border-ok',
  failed:       'border-fail',
  unresolvable: 'border-warn',
  broken:       'border-broken',
  blocked:      'border-blocked',
  building:     'border-building',
}

const borderClass = computed(() => BORDER_COLORS[props.pkg.rollup_state] ?? 'border-border-color')
const badgeClass = computed(() => BADGE_COLORS[props.pkg.rollup_state] ?? 'bg-blocked-tint text-blocked')
const columns = computed(() => Math.min(props.pkg.targets.length, 8))

const failingTargets = computed(() => props.pkg.targets.filter(t => t.state !== 'succeeded'))
const shownFailing = computed(() => failingTargets.value.slice(0, 3))
const hiddenCount = computed(() => Math.max(0, failingTargets.value.length - 3))

function targetCellClass(state: string) {
  return STATE_COLORS[state] ?? 'bg-blocked-tint text-blocked'
}

function shortLabel(repo: string) {
  return repo.replace('_', '').toLowerCase().slice(0, 5)
}

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const h = Math.floor(diff / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  if (h > 0) return `${h}h ago`
  return `${m}m ago`
}
</script>
```

- [ ] **Step 2: Write `frontend/src/components/GreenStrip.vue`**

```vue
<template>
  <div class="rounded-lg border border-ok-tint bg-ok-tint px-4 py-2.5 flex items-center gap-2">
    <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" class="text-ok shrink-0"><polyline points="20 6 9 17 4 12"/></svg>
    <span class="text-sm text-ok font-medium">{{ count }} packages built across all targets</span>
  </div>
</template>

<script setup lang="ts">
defineProps<{ count: number }>()
</script>
```

- [ ] **Step 3: Write `frontend/src/components/FailureBoard.vue`**

```vue
<template>
  <div class="flex flex-col gap-3">
    <PackageCard v-for="pkg in failingPackages" :key="pkg.project + '/' + pkg.name" :pkg="pkg" />
    <GreenStrip v-if="okCount > 0" :count="okCount" />
    <div v-if="packages.length === 0" class="text-center text-text-muted py-12 text-sm">
      No packages found
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import PackageCard from './PackageCard.vue'
import GreenStrip from './GreenStrip.vue'
import type { Package } from '../types/api'

const props = defineProps<{ packages: Package[] }>()

const failingPackages = computed(() => props.packages.filter(p => p.rollup_state !== 'succeeded'))
const okCount = computed(() => props.packages.filter(p => p.rollup_state === 'succeeded').length)
</script>
```

- [ ] **Step 4: Build and commit**

```bash
cd frontend && npm run build && echo OK
git add frontend/src/components/PackageCard.vue frontend/src/components/GreenStrip.vue frontend/src/components/FailureBoard.vue
git commit -s -m "feat(frontend): PackageCard, GreenStrip, FailureBoard components"
```

```json:metadata
{"files": ["frontend/src/components/PackageCard.vue","frontend/src/components/GreenStrip.vue","frontend/src/components/FailureBoard.vue"], "verifyCommand": "cd frontend && npm run build && echo OK", "acceptanceCriteria": ["build succeeds","PackageCard shows target grid colored by state","failing targets list shows first 3 + N more","GreenStrip shows ok count"], "modelTier": "mechanical"}
```

---

### Task 17: EventLog, TimeWindowPicker, EventRow, and App.vue

**Goal:** Implement the right column event log components and wire everything together in `App.vue` with 5-minute auto-refresh, theme toggle, and data passing.

**Files:**
- Create: `frontend/src/components/TimeWindowPicker.vue`
- Create: `frontend/src/components/EventRow.vue`
- Create: `frontend/src/components/EventLog.vue`
- Create: `frontend/src/components/MainGrid.vue`
- Modify: `frontend/src/App.vue`

**Acceptance Criteria:**
- [ ] `npm run build` succeeds with no TypeScript errors
- [ ] TimeWindowPicker presets (1h/6h/24h/3d/7d) emit `update:windowMin`; custom mode shows date pickers
- [ ] EventRow renders type glyph, scope tag, what, why, deep link
- [ ] EventLog groups events by Today / Yesterday / Earlier bucket headers
- [ ] App.vue sets `data-theme` on `<html>`, auto-refreshes every 5 minutes

**Verify:** `cd frontend && npm run build && echo OK`

**Steps:**

- [ ] **Step 1: Write `frontend/src/components/TimeWindowPicker.vue`**

```vue
<template>
  <div class="flex flex-wrap gap-1.5 mb-3">
    <button
      v-for="p in presets"
      :key="p.label"
      class="px-2.5 py-1 rounded text-xs font-medium transition-colors"
      :class="!isCustom && windowMin === p.value
        ? 'bg-brand-purple text-white'
        : 'bg-bg-card text-text-secondary border border-border-color hover:text-text-primary'"
      @click="selectPreset(p.value)"
    >
      {{ p.label }}
    </button>
    <button
      class="px-2.5 py-1 rounded text-xs font-medium transition-colors"
      :class="isCustom
        ? 'bg-brand-purple text-white'
        : 'bg-bg-card text-text-secondary border border-border-color hover:text-text-primary'"
      @click="isCustom = !isCustom"
    >
      Custom
    </button>

    <div v-if="isCustom" class="w-full flex gap-2 mt-1">
      <input
        type="datetime-local"
        class="flex-1 text-xs rounded border border-border-color bg-bg-card text-text-primary px-2 py-1"
        :value="customStart ?? ''"
        @change="(e) => $emit('update:customStart', (e.target as HTMLInputElement).value || null)"
      />
      <span class="self-center text-text-muted text-xs">→</span>
      <input
        type="datetime-local"
        class="flex-1 text-xs rounded border border-border-color bg-bg-card text-text-primary px-2 py-1"
        :value="customEnd ?? ''"
        @change="(e) => $emit('update:customEnd', (e.target as HTMLInputElement).value || null)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const presets = [
  { label: '1h', value: '60' },
  { label: '6h', value: '360' },
  { label: '24h', value: '1440' },
  { label: '3d', value: '4320' },
  { label: '7d', value: '10080' },
]

const props = defineProps<{
  windowMin: string
  customStart: string | null
  customEnd: string | null
}>()

const emit = defineEmits<{
  'update:windowMin': [v: string]
  'update:customStart': [v: string | null]
  'update:customEnd': [v: string | null]
}>()

const isCustom = ref(false)

function selectPreset(value: string) {
  isCustom.value = false
  emit('update:windowMin', value)
}
</script>
```

- [ ] **Step 2: Write `frontend/src/components/EventRow.vue`**

```vue
<template>
  <div class="flex items-start gap-2 py-2 border-b border-border-color last:border-0">
    <!-- Glyph badge -->
    <span
      class="mt-0.5 w-5 h-5 rounded-full flex items-center justify-center text-[10px] font-bold shrink-0"
      :class="glyphClass"
    >{{ glyph }}</span>

    <div class="flex-1 min-w-0">
      <div class="flex items-center gap-1.5 flex-wrap">
        <!-- Scope tag -->
        <span class="px-1.5 py-0.5 rounded text-[10px] font-medium bg-blocked-tint text-text-muted uppercase tracking-wide">
          {{ event.scope }}
        </span>
        <!-- What -->
        <span class="text-xs font-medium text-text-primary truncate">{{ event.what }}</span>
        <!-- Link -->
        <a
          v-if="event.url"
          :href="event.url"
          target="_blank"
          rel="noopener noreferrer"
          class="text-[10px] text-info hover:underline shrink-0"
        >↗</a>
      </div>
      <!-- Why + target -->
      <div class="text-[11px] text-text-muted mt-0.5 truncate">
        {{ event.why }}
        <span v-if="event.repo"> · {{ event.repo }}/{{ event.arch }}</span>
        <span class="mx-1 opacity-40">·</span>
        {{ timeAgo(event.at) }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OBSEvent } from '../types/api'

const props = defineProps<{ event: OBSEvent }>()

const GLYPHS: Record<string, string> = {
  succeeded: '✓', failed: '✕', broken: '✕', unresolvable: '!',
  blocked: '⊘', building: '↻', triggered: '↻', started: '▶', published: '▲',
}
const GLYPH_COLORS: Record<string, string> = {
  succeeded:    'bg-ok-tint text-ok',
  failed:       'bg-fail-tint text-fail',
  broken:       'bg-broken-tint text-broken',
  unresolvable: 'bg-warn-tint text-warn',
  blocked:      'bg-blocked-tint text-blocked',
  building:     'bg-building-tint text-building',
  triggered:    'bg-info-tint text-info',
  started:      'bg-info-tint text-info',
  published:    'bg-brand-purple-tint text-brand-purple',
}

const glyph = computed(() => GLYPHS[props.event.type] ?? '·')
const glyphClass = computed(() => GLYPH_COLORS[props.event.type] ?? 'bg-blocked-tint text-blocked')

function timeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const h = Math.floor(diff / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  if (h >= 24) return `${Math.floor(h / 24)}d ago`
  if (h > 0) return `${h}h ago`
  return `${m}m ago`
}
</script>
```

- [ ] **Step 3: Write `frontend/src/components/EventLog.vue`**

```vue
<template>
  <div class="flex flex-col h-full">
    <div class="flex items-center justify-between mb-3">
      <span class="text-sm font-semibold text-text-primary">Events</span>
    </div>

    <TimeWindowPicker
      :windowMin="windowMin"
      :customStart="customStart"
      :customEnd="customEnd"
      @update:windowMin="$emit('update:windowMin', $event)"
      @update:customStart="$emit('update:customStart', $event)"
      @update:customEnd="$emit('update:customEnd', $event)"
    />

    <div class="flex-1 overflow-y-auto">
      <div v-if="loading" class="text-center text-text-muted py-8 text-sm">Loading…</div>
      <div v-else-if="events.length === 0" class="text-center text-text-muted py-8 text-sm">No events</div>
      <template v-else>
        <template v-for="(group, label) in grouped" :key="label">
          <div v-if="group.length > 0">
            <div class="text-[10px] font-semibold uppercase tracking-widest text-text-muted py-2">{{ label }}</div>
            <EventRow v-for="e in group" :key="e.id" :event="e" />
          </div>
        </template>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import TimeWindowPicker from './TimeWindowPicker.vue'
import EventRow from './EventRow.vue'
import type { OBSEvent } from '../types/api'

const props = defineProps<{
  events: OBSEvent[]
  loading: boolean
  windowMin: string
  customStart: string | null
  customEnd: string | null
}>()

defineEmits<{
  'update:windowMin': [v: string]
  'update:customStart': [v: string | null]
  'update:customEnd': [v: string | null]
}>()

const grouped = computed(() => {
  const today: OBSEvent[] = []
  const yesterday: OBSEvent[] = []
  const earlier: OBSEvent[] = []
  const now = new Date()
  const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const yestStart = new Date(todayStart.getTime() - 86400000)

  for (const e of props.events) {
    const d = new Date(e.at)
    if (d >= todayStart) today.push(e)
    else if (d >= yestStart) yesterday.push(e)
    else earlier.push(e)
  }
  return { Today: today, Yesterday: yesterday, Earlier: earlier }
})
</script>
```

- [ ] **Step 4: Write `frontend/src/components/MainGrid.vue`**

```vue
<template>
  <div class="flex-1 grid grid-cols-1 lg:grid-cols-[1fr_440px] gap-4 px-6 py-4 overflow-hidden">
    <div class="overflow-y-auto">
      <FailureBoard :packages="packages" />
    </div>
    <div class="bg-bg-card rounded-lg border border-border-color p-4 overflow-hidden flex flex-col">
      <EventLog
        :events="events"
        :loading="eventsLoading"
        :windowMin="windowMin"
        :customStart="customStart"
        :customEnd="customEnd"
        @update:windowMin="$emit('update:windowMin', $event)"
        @update:customStart="$emit('update:customStart', $event)"
        @update:customEnd="$emit('update:customEnd', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import FailureBoard from './FailureBoard.vue'
import EventLog from './EventLog.vue'
import type { Package, OBSEvent } from '../types/api'

defineProps<{
  packages: Package[]
  events: OBSEvent[]
  eventsLoading: boolean
  windowMin: string
  customStart: string | null
  customEnd: string | null
}>()

defineEmits<{
  'update:windowMin': [v: string]
  'update:customStart': [v: string | null]
  'update:customEnd': [v: string | null]
}>()
</script>
```

- [ ] **Step 5: Write `frontend/src/App.vue`**

```vue
<template>
  <div class="min-h-screen flex flex-col bg-bg-app">
    <AppHeader :theme="theme" @toggle-theme="toggleTheme" />
    <ContextBar
      :version="version"
      :activeScopes="scopes"
      :updatedAt="updatedAt"
      @update:version="version = $event; refresh()"
      @update:scopes="scopes = $event; refresh()"
    />
    <HealthHeader :packages="packages" />
    <MainGrid
      :packages="packages"
      :events="events"
      :eventsLoading="eventsLoading"
      :windowMin="windowMin"
      :customStart="customStart"
      :customEnd="customEnd"
      @update:windowMin="windowMin = $event; refreshEvents()"
      @update:customStart="customStart = $event; refreshEvents()"
      @update:customEnd="customEnd = $event; refreshEvents()"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import AppHeader from './components/AppHeader.vue'
import ContextBar from './components/ContextBar.vue'
import HealthHeader from './components/HealthHeader.vue'
import MainGrid from './components/MainGrid.vue'
import { usePackages } from './composables/usePackages'
import { useEvents } from './composables/useEvents'
import type { Scope } from './types/api'

const theme = ref<'light' | 'dark'>('light')
const version = ref('17')
const scopes = ref<Scope[]>([])
const windowMin = ref('1440')
const customStart = ref<string | null>(null)
const customEnd = ref<string | null>(null)
const updatedAt = ref<string | null>(null)

function toggleTheme() {
  theme.value = theme.value === 'light' ? 'dark' : 'light'
  document.documentElement.setAttribute('data-theme', theme.value)
}

const {
  data: packages,
  refresh: refreshPackages,
} = usePackages(
  () => 'ppg',
  () => version.value,
  () => scopes.value,
)

const {
  data: events,
  loading: eventsLoading,
  refresh: refreshEvents,
} = useEvents(
  () => 'ppg',
  () => version.value,
  () => windowMin.value,
  () => customStart.value,
  () => customEnd.value,
)

async function refresh() {
  await Promise.all([refreshPackages(), refreshEvents()])
  updatedAt.value = new Date().toISOString()
}

let timer: ReturnType<typeof setInterval>

onMounted(() => {
  refresh()
  timer = setInterval(refresh, 5 * 60 * 1000)
})

onUnmounted(() => clearInterval(timer))
</script>
```

- [ ] **Step 6: Final build and commit**

```bash
cd frontend && npm run build && echo "Frontend complete"
git add frontend/src/components/ frontend/src/App.vue
git commit -s -m "feat(frontend): EventLog, TimeWindowPicker, EventRow, MainGrid, App.vue"
```

```json:metadata
{"files": ["frontend/src/components/EventLog.vue","frontend/src/components/TimeWindowPicker.vue","frontend/src/components/EventRow.vue","frontend/src/components/MainGrid.vue","frontend/src/App.vue"], "verifyCommand": "cd frontend && npm run build && echo OK", "acceptanceCriteria": ["build succeeds","TimeWindowPicker presets emit update:windowMin","EventLog groups events into Today/Yesterday/Earlier","App.vue auto-refreshes every 5 minutes","dark theme toggled via data-theme attribute"], "modelTier": "mechanical"}
```

---

### Task 18: Integration — Vite Proxy, Docker Compose End-to-End

**Goal:** Verify the full stack works end-to-end in Docker Compose: backend serves API, frontend proxies `/api` to backend, and the full stack starts without errors.

**Files:**
- Modify: `frontend/vite.config.ts` (ensure proxy target uses service name `backend`)
- Modify: `docker-compose.yml` (ensure health check and startup order)

**Acceptance Criteria:**
- [ ] `docker compose build` succeeds for both services
- [ ] `docker compose up` starts without fatal errors (backend exits only on missing OBS creds, which is expected)
- [ ] `curl http://localhost:8080/api/products/ppg/17/packages` returns `[]` JSON

**Verify:** `docker compose build && echo "Build OK"`

**Steps:**

- [ ] **Step 1: Verify `frontend/vite.config.ts` proxy target**

The proxy target must use the Docker Compose service name `backend` (not `localhost`) so it works inside the container network:

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: 'http://backend:8080',
        changeOrigin: true,
      },
    },
  },
})
```

- [ ] **Step 2: Add a `data` directory to the repo**

```bash
mkdir -p data
echo "# SQLite database lives here (gitignored)" > data/.gitkeep
echo "data/*.db" >> .gitignore
```

- [ ] **Step 3: Create a minimal `.env` for local testing**

```bash
cp .env.example .env
# Edit .env and add OBS_USERNAME and OBS_PASSWORD
```

Note: without valid OBS credentials the backend exits with an error on startup — this is expected behavior.

- [ ] **Step 4: Build and smoke-test**

```bash
docker compose build
# Backend only (no MQ/OBS credentials needed for HTTP server to start)
OBS_USERNAME=test OBS_PASSWORD=test docker compose up backend -d
sleep 3
curl -s http://localhost:8080/api/products/ppg/17/packages
# Expected: [] (empty array — no data yet, but server is running)
docker compose down
```

- [ ] **Step 5: Commit and tag**

```bash
git add data/.gitkeep .gitignore docker-compose.yml frontend/vite.config.ts
git commit -s -m "feat: integration — docker compose end-to-end stack verified"
```

```json:metadata
{"files": ["docker-compose.yml","frontend/vite.config.ts","data/.gitkeep"], "verifyCommand": "docker compose build && echo OK", "acceptanceCriteria": ["docker compose build succeeds","curl /api/products/ppg/17/packages returns JSON array","Vite proxy routes /api to backend:8080"], "modelTier": "standard"}
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Covered by task |
|---|---|
| MQ Consumer (§3.3) | Task 9 |
| OBS Poller — discovery + reconcile (§3.4) | Task 7 |
| Trigger inference (§3.5) | Task 8 |
| SQLite schema (§3.6) | Task 4 |
| HTTP API endpoints (§3.7) | Task 10 |
| Config — env vars + YAML (§3.2) | Task 3 |
| Vue component tree (§4.2) | Tasks 14–17 |
| CSS variable system + Tailwind (§4.3) | Task 12 |
| Composables usePackages + useEvents (§4.4) | Task 13 |
| State management in App.vue (§4.5) | Task 17 |
| Docker Compose services (§5) | Tasks 1, 18 |

**Placeholder scan:** No TBDs, TODOs, or incomplete sections found.

**Type consistency:** All cross-task type references (model.Package, model.Event, store functions, obs.Client methods) are defined in Tasks 2–6 before they are used in Tasks 7–11.

