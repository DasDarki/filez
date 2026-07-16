# Filez

A small, self-hostable file-sharing platform for any kind of file — media or not.
Upload from a minimalist web UI or straight from your terminal, and share a link.

*Made with ♥ by DasDarki ([github.com/DasDarki](https://github.com/DasDarki)).*

## Features

- **Four kinds of files:** permanent, temporary (auto-expiring), download-limited, and password-protected.
- **Web UI:** drag & drop upload, light/dark mode, self-hosted font (no external CDN — GDPR friendly).
- **Direct links** (`/d/…`) and **rich previews** (`/p/…`): text viewer, image/audio/video players, PDF embed, ZIP listing.
- **Public or private:** open to everyone by default, or gated behind access keys managed in a Basic-Auth admin area.
- **CLI (`filez`)** for scripted uploads and host management, plus **`filezui`**, an animated interactive console UI.
- **Auto-upload folders:** watch a directory (e.g. your screenshots) — new files upload automatically, the link is copied to your clipboard and a notification pops up.

## Architecture

One Go module, three binaries:

| Path            | Binary     | What it is                                              |
|-----------------|------------|--------------------------------------------------------|
| `cmd/server`    | server     | Fiber v3 HTTP server + embedded web UI                  |
| `cmd/filez`     | `filez`    | Cobra CLI: direct upload + host config                  |
| `cmd/filezui`   | `filezui`  | Bubble Tea interactive upload UI                        |

The server stores **metadata** in SQLite (pure-Go `modernc.org/sqlite`, WAL mode) and **file bytes** on
disk, with a fast in-memory cache (`ristretto`) for hot files. Nothing but metadata lives in the database.

## Running the server

```sh
cp .env.example .env      # optional; sane defaults otherwise
go run ./cmd/server
```

Then open <http://localhost:8080>. See [.env.example](.env.example) for all settings. The most important ones:

- `PUBLIC` — `true` (anyone can use it) or `false` (access key required).
- `ADMIN_PASSWORD` — set it to enable `/admin` (username is always `admin`) where you create access keys.
- `DATA_DIR`, `MAX_UPLOAD_SIZE`, `CACHE_SIZE`, `DEFAULT_UPLOAD`, `BASE_URL`.

### Routes

| Route                    | Description                                                        |
|--------------------------|-------------------------------------------------------------------|
| `GET /`                  | Web UI (drag & drop upload; shows an access-key gate when private) |
| `POST /api/upload`       | Upload (`mode`, `ttl`, `downloads`, `password`)                    |
| `GET /d/<id>/<name>`     | Direct download, keeps the original filename (byte-range support)  |
| `GET /p/<id>/<name>`     | Preview page (viewer chosen by file type)                         |
| `GET /admin`             | Access-key management (Basic Auth, only if `ADMIN_PASSWORD` set)  |
| `GET /api/info`          | Instance discovery for the CLI (`filez`/`public`/limits)          |
| `GET /api/auth/check`    | Validate an access key                                             |

Links keep the original filename (`/d/<id>/Report.pdf`) while the short random `<id>` remains the
collision-free, unguessable lookup key — the name segment is cosmetic, so two files named the same never
clash. The legacy `/d/<id>.<ext>` form still resolves.

### Access model

- **Instance access** (only when `PUBLIC=false`): every upload/link needs a valid access key, passed as
  `Authorization: Access-Key <key>`, HTTP Basic Auth (any user, password = key), or — in the browser —
  stored via the gate's *"stay signed in"* option.
- **Per-file passwords:** independent of the above. On `/p` you get a password form; on `/d` supply the
  password via `?pw=`, the `X-File-Password` header, or Basic Auth.

### Download limits vs. previews

For download-limited files, the counter is spent on the **actual file delivery** via `/d`. To keep the count
predictable, the preview page for a limited file shows a **download button** instead of streaming inline, and
byte-range continuations are not double-counted. When the last download is spent, the file is removed and the
link returns `410 Gone`. A background job also cleans up expired temporary files.

## Command-line client (`filez`)

Install straight from the repo (produces `filez` / `filezui` in `$(go env GOPATH)/bin`):

```sh
go install github.com/DasDarki/filez/cmd/filez@latest
go install github.com/DasDarki/filez/cmd/filezui@latest
```

Or build locally:

```sh
go build -o filez ./cmd/filez

# Configure a host (asks for the domain, verifies it's a Filez instance,
# then the access key if the instance is private):
filez config hosts add
filez config hosts                 # list configured hosts
filez config hosts primary <name>  # choose the default host
filez config hosts delete <name>
filez config askhost true          # ask which host on every upload
filez config default temp:1d       # default upload mode

# Upload:
filez report.pdf                   # uses the configured default mode
filez photo.png --temp 2d20m       # temporary (units: s m h d w M[onth])
filez build.zip  --downloads 3     # delete after 3 downloads
filez notes.txt  --password s3cret # password protected
filez data.bin   --host other      # pick a specific host
```

Short aliases mirror the flags: `-p`/`--permanent`, `-t`/`--temp`, `--pw`/`--password`,
`--dl`/`--downloads`, `-H`/`--host`. The default upload mode comes from `$DEFAULT_UPLOAD`, then the
CLI config, then `permanent`.

Config is stored at `$XDG_CONFIG_HOME/filez/config.json` (mode `0600`, since it holds access keys).

## Auto-upload folders (`filez hook`)

Watch a directory and automatically upload anything that lands in it — the link is copied to your
clipboard and a desktop notification pops up. Perfect for screenshots (e.g. KDE Spectacle saving to
`~/Pictures`).

```sh
filez hook add ~/Pictures            # watch a folder (uses your default upload mode)
filez hook add ~/Pictures --temp 30d # or make its uploads temporary
filez hook list
filez hook remove ~/Pictures

filez hook watch                     # run the watcher in the foreground
filez hook install                   # …or install it as a systemd user service (autostart at login)
filez hook uninstall
```

`hook install` creates `~/.config/systemd/user/filez-hook.service` and starts it, so uploads happen in
the background from every login (`journalctl --user -u filez-hook -f` for logs). The watcher waits until
a file finishes writing, skips hidden/temp files, and copies the link via `wl-copy` (Wayland),
`xclip`/`xsel` (X11) or `pbcopy` (macOS); notifications use `notify-send`.

## Interactive console UI (`filezui`)

```sh
go build -o filezui ./cmd/filezui
filezui
```

Walks through host → file → mode → options with a live progress bar. Uses the same hosts as `filez`.

## Deployment (Docker / Coolify)

The included [Dockerfile](Dockerfile) builds a fully static image (pure-Go SQLite → no CGO) that runs
the server as an unprivileged user and stores everything under `/data`.

```sh
docker build -t filez .
docker run -d -p 8080:8080 \
  -e ADMIN_PASSWORD=change-me -e PUBLIC=false \
  -v filez-data:/data filez
```

### On Coolify

1. New Resource → **Dockerfile** (point it at this repo).
2. **Port:** `8080`.
3. **Persistent storage:** mount a volume at `/data` (holds the SQLite DB and all files).
4. **Environment variables:** set at least `ADMIN_PASSWORD` (to enable `/admin`) and `PUBLIC`
   (`false` for a private instance). See [.env.example](.env.example) for the rest.

Links are generated with the correct public `https://…` URL automatically: `TRUST_PROXY` (on by default)
makes the server honor `X-Forwarded-Proto`/`X-Forwarded-Host` from Coolify's reverse proxy. Set `BASE_URL`
only if you want to force a fixed URL.

## Development

```sh
go build ./...   # build everything
go test ./...    # run tests
```
