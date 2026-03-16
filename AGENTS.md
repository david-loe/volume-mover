# AGENTS.md

## Project Purpose

`volume-mover` is a Go application for cloning, copying, and moving Docker volumes across hosts.

It has two user-facing surfaces:

- a Cobra CLI
- a React + Vite + TypeScript SPA served by the Go binary

The web UI is the primary operations interface. It uses an internal JSON API and server-sent events. There is no stable public API contract.

## Current Architecture

### Backend

- Language: Go
- Router: `chi`
- CLI: `cobra`
- Persistence: SQLite via `github.com/mattn/go-sqlite3`
- Runtime transfer execution: shelling out to `docker` locally or over SSH

Important backend packages:

- `cmd/volume-mover`: binary entrypoint
- `internal/cli`: Cobra command tree
- `internal/service`: host, volume, and transfer logic
- `internal/web`: SPA/API server and routing
- `internal/jobs`: async transfer jobs, SQLite store, SSE fanout
- `internal/config`: host config persistence and SSH config import
- `internal/model`: shared domain/API types
- `internal/shell`: local/SSH command execution abstraction
- `internal/humanize`: byte formatting helpers

### Frontend

- Framework: React 18
- Build tool: Vite 5
- Language: TypeScript
- Router: `react-router-dom`

SPA source lives in `webapp/src`.

Embedded build output lives in `internal/web/spa/dist` and is served by `internal/web/spa.go`.

Do not hand-edit files under `internal/web/spa/dist`. They are generated.

## Core Product Behavior

### Hosts

- Supports `local` and `ssh` hosts.
- Manual host definitions are stored in YAML config.
- SSH hosts can be imported from `~/.ssh/config`.
- `local` is implicit and cannot be deleted.

### Volumes

- Volume lists are discovered through `docker volume ls`.
- Attached container information is derived from container inspection, not the Docker volume list output.
- Volume detail includes:
  - name
  - driver
  - labels
  - attached containers
  - running container count
  - size in bytes

### Size Calculation

- Size is computed on demand by running a helper container against the volume and calling `du`.
- Default helper image is `busybox:1.36`.
- Override with `VOLUME_MOVER_HELPER_IMAGE`.
- Size parsing is intentionally tolerant of Docker image pull noise in command output.

### Transfers

Supported operations:

- `clone`: same-host copy, destination name must differ
- `copy`: cross-host or same-host copy, source retained
- `move`: copy first, then delete source only after destination verification

Safety rules:

- destination volume must not already exist
- if source volume is attached to running containers, transfer is blocked unless `allowLive` / `--allow-live` is set
- `quiesceSource` / `--quiesce-source` is only meaningful for `clone` and `copy`
- move does not relocate workloads; it only moves volume data

Transfer execution is agentless. The app shells out to `docker` on the source and destination hosts and streams tar data through temporary helper containers.

## Async Job System

The SPA does not run transfers inline through HTTP request/response. It creates background jobs.

Implementation:

- `internal/jobs/store.go`: SQLite persistence
- `internal/jobs/manager.go`: queueing, execution, cancellation, event broadcast

Job statuses:

- `queued`
- `validating`
- `running`
- `cancelling`
- `completed`
- `failed`
- `cancelled`

Persistence:

- jobs are stored in SQLite
- job items store per-volume state
- job events store timeline/progress entries

Current restart behavior:

- interrupted jobs are marked failed on process start
- they are not resumed

Cancellation behavior:

- cancellation is best effort
- cancellation is checked between major steps, not as an interruptible byte-stream abort

## Web UI

Primary routes:

- `/app/dashboard`
- `/app/hosts`
- `/app/volumes`
- `/app/volumes/:host/:name`
- `/app/transfers/new`
- `/app/transfers`
- `/app/transfers/:jobId`

API routes are under `/api/v1`.

Current API groups:

- hosts
- volumes
- transfer jobs
- job event streaming via SSE

Legacy server-rendered templates were removed. This project is now SPA/API-only.

There are still legacy redirect routes in the Go server for compatibility:

- `/hosts`
- `/volumes`
- `/volumes/{host}/{name}`
- `/transfer`

Those redirect into `/app/...`. There are no old template assets left.

## Volume List UX Rules

- The volumes page defaults to hiding anonymous volumes.
- Anonymous volume detection is heuristic-based: a 64-character lowercase hex name is treated as anonymous.
- The SPA currently sends `hideAnonymous=1` by default.

This is UI behavior, not a Docker-native volume type distinction.

## Important Runtime Paths

Host config:

- `~/.config/volume-mover/hosts.yaml`

Job database:

- `~/.config/volume-mover/jobs.db`

If a custom config path is passed, the jobs database is created beside that config file.

## Important Environment Variables

- `VOLUME_MOVER_WEB_USERNAME`
- `VOLUME_MOVER_WEB_PASSWORD`
- `VOLUME_MOVER_HELPER_IMAGE`
- `VOLUME_MOVER_LOG_LEVEL`

Current auth model:

- optional HTTP Basic Auth on the web server
- no user accounts or RBAC

## Build and Development Requirements

- Go 1.22 is what CI uses
- `go.mod` currently declares `go 1.18`
- Node.js 22 is what CI uses
- Docker CLI must be installed where the binary runs
- remote hosts must support SSH and Docker
- `gcc` is required for local builds because SQLite uses CGO

## Local Development Commands

Frontend install:

```bash
npm --prefix webapp ci
```

Frontend build:

```bash
npm --prefix webapp run build
```

Frontend dev server:

```bash
npm --prefix webapp run dev
```

Backend tests:

```bash
go test ./...
```

Run the app:

```bash
go run ./cmd/volume-mover web --listen 127.0.0.1:8080
```

Docker integration tests:

```bash
VOLUME_MOVER_RUN_DOCKER_TESTS=1 go test ./internal/service -run Integration
```

## Required Build Order

When frontend files change:

1. build the SPA with `npm --prefix webapp run build`
2. then run `go test ./...` or `go build ./...`

Do not run the SPA build and Go compile/test in parallel.

Reason:

- Go embeds files from `internal/web/spa/dist`
- Vite produces hashed asset filenames
- if Go compiles while Vite is replacing hashed files, embed can fail with missing asset errors

This race has already occurred in this project.

## Generated and Non-Generated Boundaries

Generated or build output:

- `internal/web/spa/dist/*`
- `webapp/node_modules/*`

Authoritative source files:

- `webapp/src/*`
- `internal/*`
- `cmd/*`
- workflow files
- `Dockerfile`
- `.goreleaser.yaml`

Never treat `internal/web/spa/dist` as the source of truth.

## Testing Notes

Current backend tests exist for:

- config and SSH import
- service transfer validation and parsing
- jobs manager/store behavior
- web server/API behavior

Frontend tests are not yet in place.

If you change SPA behavior, at minimum rebuild the frontend and run Go tests.

## Release and Distribution

CI workflow:

- installs Node dependencies
- builds the SPA
- runs `go test ./...`
- runs `goreleaser check`

Release workflow:

- runs on tags matching `v*`
- logs in to GHCR
- logs in to Docker Hub
- runs GoReleaser release

Current release outputs:

- Linux `amd64` binaries via GoReleaser
- multi-arch container images via Docker buildx

Important constraint:

- because SQLite uses CGO, the project does not currently publish the broader cross-platform static binary matrix it had before

## Docker Image

`Dockerfile` is multi-stage:

1. Node stage builds the SPA
2. Go stage builds the binary with CGO enabled
3. Alpine runtime includes:
   - `bash`
   - `ca-certificates`
   - `openssh-client`
   - `docker-cli`

Default container command:

```bash
volume-mover web --listen 0.0.0.0:8080
```

## Frontend/Backend Contract Notes

The SPA expects the backend to return empty arrays, not `null`, for list-like fields where possible.

This matters for:

- `hosts`
- `volumes`
- `jobs`
- `job.items`
- `job.events`
- `volumeDetail.containers`

The frontend API layer also normalizes nullable arrays defensively. Keep both sides tolerant.

## Important Source Files

Use these as entry points when changing behavior:

- CLI root: `internal/cli/root.go`
- web server and API routes: `internal/web/server.go`
- SPA embedding: `internal/web/spa.go`
- transfer execution: `internal/service/service.go`
- job persistence: `internal/jobs/store.go`
- job orchestration: `internal/jobs/manager.go`
- shared models: `internal/model/types.go`, `internal/model/jobs.go`
- SPA API client: `webapp/src/api.ts`
- SPA router: `webapp/src/App.tsx`

## Known Project Constraints and Gotchas

- SQLite driver is `github.com/mattn/go-sqlite3`, so CGO is required.
- The helper image is pinned to `busybox:1.36` by default.
- Integration tests also currently reference `busybox:1.36`.
- Legacy redirect routes still exist even though old HTML templates were removed.
- The internal API is for the bundled SPA; avoid treating it as a stable external contract.
- Anonymous-volume filtering is heuristic only.
- Restart recovery currently marks interrupted jobs failed instead of resuming them.

## Practical Maintenance Guidance

- If you touch `webapp/src`, rebuild the SPA before validating Go builds.
- If you change API payload shapes, update both `internal/model` and `webapp/src/types.ts` / `webapp/src/api.ts`.
- If you change transfer semantics, review both direct CLI transfer calls and async job execution.
- If you change job persistence, review restart recovery and SSE consumers.
- If you remove legacy redirects, update any docs or bookmarks that still mention `/hosts`, `/volumes`, or `/transfer`.

## Current Status Summary

This repository is no longer a template-driven Go web app.

It is currently:

- a Go CLI
- a Go API/SSE server
- an embedded React SPA
- a SQLite-backed async transfer system

Any future work should assume that architecture, not the earlier server-rendered UI design.
