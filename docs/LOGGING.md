# Logging — ant

## What's logged

Structured JSON logging via `log/slog` (`cmd/api/main.go`). Every log line is written
to **both**:
1. `os.Stdout` (captured by Docker)
2. `${LOG.DIR}/api.log` (bind-mounted to `./log/api.log` in `docker-compose.yml`)

via `io.MultiWriter`. Format is `slog.NewJSONHandler` — one JSON object per line.

The log file is opened once at startup in append mode (`os.OpenFile` with
`O_APPEND|O_CREATE|O_WRONLY`) and kept open for the process lifetime. **The process
does not reopen the file on `SIGHUP`** — this matters for rotation (see below).

## Configuration

| Key | Env override | Default | Notes |
|---|---|---|---|
| `LOG.DIR` | `ANT_LOG_DIR` | `log` | directory for `api.log`; created at startup if missing |
| `LOG.LEVEL` | `ANT_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error`; unknown values fall back to `info` |

Levels used in handlers/services: `INFO` normal operations, `WARN` client
errors/auth failures, `ERROR` system failures (DB, config, startup).

## Rotation

Two independent log streams, two rotation mechanisms:

### 1. Docker-captured stdout
Set in `docker-compose.yml` on the `api` service:
```yaml
logging:
  driver: json-file
  options:
    max-size: "10m"
    max-file: "3"
```
Caps Docker's own copy of stdout at ~30MB (3 files × 10MB), rotated automatically by
the Docker daemon. No app involvement.

### 2. The bind-mounted `./log/api.log` file
Not covered by the Docker log driver — it's a real file the app writes to directly.
Rotated on the **host**, via the `logrotate.conf` shipped in this repo:
```
daily
rotate 7
compress
delaycompress
missingok
notifempty
copytruncate
```
Install once on the production host (adjust the path glob inside to the real
deployment directory first):
```
cp logrotate.conf /etc/logrotate.d/ant-api
```

**Why `copytruncate` and not `create`+`postrotate` signal:** the app never reopens
its log file handle after opening it once at startup, so a rename-based rotation
(`create`) would leave the process writing into the renamed/rotated-away file
forever. `copytruncate` copies the current content out and truncates the original
file in place, which the already-open file descriptor tolerates correctly.

## Verifying

```
docker compose logs -f api        # stdout stream, Docker-side
tail -f log/api.log               # file stream, host-side
logrotate -d /etc/logrotate.d/ant-api   # dry-run the rotation rule
```
