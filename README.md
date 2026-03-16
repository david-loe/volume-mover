# volume-mover

Move Docker volumes without building ad hoc tar pipelines by hand.

Use it to:

- inspect volumes and attached containers
- clone within one host
- copy volumes across hosts
- move volume data with destination verification
- run transfers from a web UI or the CLI

## Quick Start

The fastest self-hosted setup is the included [`compose.yml`](/home/user/projects/volume-mover/compose.yml):

```bash
docker compose up -d
```

Open:

```text
http://127.0.0.1:8080/app/dashboard
```

Default login from `compose.yml`:

- username: `admin`
- password: `change-me`

Change that password before exposing the UI anywhere except a trusted local machine.

## CLI First Run

If you want to use the binary directly instead of Docker Compose:

```bash
volume-mover host list
volume-mover host import-ssh
volume-mover host add --name remote-a --host 10.0.0.20 --user root --port 22 --identity-file ~/.ssh/id_ed25519
volume-mover host test remote-a

volume-mover volume list --host local
volume-mover volume show --host local my-volume

volume-mover volume clone --host local --source app-data --dest app-data-copy
volume-mover volume copy --source-host local --source-volume app-data --dest-host remote-a --dest-volume app-data-backup
volume-mover volume move --source-host local --source-volume app-data --dest-host remote-a --dest-volume app-data-moved --allow-live

volume-mover web --listen 127.0.0.1:8080
```

Open the web UI at:

```text
http://127.0.0.1:8080/app/dashboard
```

## Common Flows

Clone a volume on the same host:

```bash
volume-mover volume clone --host local --source app-data --dest app-data-copy
```

Copy a volume to another host:

```bash
volume-mover volume copy \
  --source-host local \
  --source-volume app-data \
  --dest-host remote-a \
  --dest-volume app-data-backup
```

Move a volume and allow live-copy behavior:

```bash
volume-mover volume move \
  --source-host local \
  --source-volume app-data \
  --dest-host remote-a \
  --dest-volume app-data-moved \
  --allow-live
```

## What The Compose Setup Does

- runs the published container image
- persists app state in a named Docker volume
- mounts `/var/run/docker.sock` so the app can access the local Docker daemon
- exposes the web UI on port `8080`
- enables HTTP Basic Auth through environment variables

Optional for SSH host import and remote SSH access from inside the container:

```yaml
volumes:
  - ${HOME}/.ssh:/root/.ssh:ro
```

Important:

- Mounting the Docker socket effectively gives the container admin-level control over the local Docker engine.
- The container needs Docker access on the host where it runs.

## Transfer Modes

| Operation | What it does                                                               |
| --------- | -------------------------------------------------------------------------- |
| `clone`   | Copies a volume on the same host to a different destination name           |
| `copy`    | Copies a volume to another host or another volume while keeping the source |
| `move`    | Copies first, verifies the destination, then removes the source volume     |

Rules enforced by the app:

- destination volume must not already exist
- same-host source and destination names must differ
- live volumes are blocked unless `allow live` / `--allow-live` is enabled
- `quiesce source` is only used for `clone` and `copy`
- workloads are not relocated, only volume data

## Web UI

Main routes:

- `/app/dashboard`
- `/app/hosts`
- `/app/volumes`
- `/app/volumes/:host/:name`
- `/app/transfers/new`
- `/app/transfers`
- `/app/transfers/:jobId`

## Persistence And Paths

| Item         | Path                                |
| ------------ | ----------------------------------- |
| Host config  | `~/.config/volume-mover/hosts.yaml` |
| Job database | `~/.config/volume-mover/jobs.db`    |

If you pass a custom config path, the jobs database is created beside that config file.

## Environment Variables

| Variable                    | Purpose                                                    |
| --------------------------- | ---------------------------------------------------------- |
| `VOLUME_MOVER_WEB_USERNAME` | Enables Basic Auth when paired with password               |
| `VOLUME_MOVER_WEB_PASSWORD` | Enables Basic Auth when paired with username               |
| `VOLUME_MOVER_HELPER_IMAGE` | Overrides the helper image used for `du` and tar streaming |
| `VOLUME_MOVER_LOG_LEVEL`    | Reserved for future log-level control                      |

## Development

Frontend install:

```bash
npm --prefix webapp ci
```

Frontend dev server:

```bash
npm --prefix webapp run dev
```

Frontend build:

```bash
npm --prefix webapp run build
```

Backend tests:

```bash
go test ./...
```

Run the app locally:

```bash
go run ./cmd/volume-mover web --listen 127.0.0.1:8080
```

Optional Docker integration tests:

```bash
VOLUME_MOVER_RUN_DOCKER_TESTS=1 go test ./internal/service -run Integration
```

## Runtime Notes

- Transfers shell out to `docker` locally or over SSH on remote hosts.
- Volume data is streamed through temporary helper containers.
- The `/api/v1` API is internal to the bundled SPA and not a stable public contract.

## Required Build Order

When you change anything under `webapp/src`:

1. Build the SPA with `npm --prefix webapp run build`
2. Then run `go test ./...` or `go build ./...`

That order matters because Go embeds files from `internal/web/spa/dist`, and Vite rewrites hashed asset names during build.

## Release Output

Releases currently publish:

- Linux `amd64` binaries
- multi-arch container images through Docker buildx
- images to GHCR and Docker Hub

The container image names are based on release-time environment variables. In practice, GHCR publishes:

```text
ghcr.io/david-loe/volume-mover:latest
```

## Safety Notes

- `allow live` can copy a volume while applications are still writing to it.
- `quiesce source` only stops containers that directly mount the source volume.
- Compose-project lifecycle is not orchestrated automatically.
- Interrupted jobs are currently marked failed on process restart; they are not resumed.
