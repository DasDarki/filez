# Filez

A small, self-hostable file-sharing platform for any kind of file — media or not.
Upload from a minimalist web UI or straight from your terminal, and share a link.

*Made with ♥ by DasDarki ([github.com/DasDarki](https://github.com/DasDarki)).*

## Features

- **Four kinds of files:** permanent, temporary (auto-expiring), download-limited, and password-protected.
- **Idle cleanup:** optionally delete permanent files that haven't been accessed in a while, with an authorized "keep forever" override.
- **Web UI:** drag & drop upload, light/dark mode, self-hosted font (no external CDN — GDPR friendly).
- **Direct links** (`/d/…`) and **rich previews** (`/p/…`): text viewer, image/audio/video players, PDF embed, ZIP listing.
- **Public or private:** open to everyone by default, or gated behind access keys managed in a Basic-Auth admin area.
- **CLI (`filez`)** for scripted uploads and host management, plus **`filezui`**, an animated interactive console UI.
- **Auto-upload folders:** watch a directory (e.g. your screenshots) — new files upload automatically, the link is copied to your clipboard and a notification pops up.
- **KDE integration:** a "Share with Filez" right-click menu in Dolphin (Plasma / Wayland).
- **Live sessions:** stream your uploads to a live, auto-refreshing view (`/l/<id>`) instead of creating links — great for demoing screenshots.
- **Sync buckets:** a LocalSend-style temporary shared drop with a 4-digit code — anyone with the link uploads/downloads, in memory only.

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

- **Instance access** (only when `PUBLIC=false`): uploading needs a valid access key, passed as
  `Authorization: Access-Key <key>`, HTTP Basic Auth (any user, password = key), or — in the browser —
  stored via the gate's *"stay signed in"* option. Download/preview links stay reachable without a key
  by default so you can share them (set `PUBLIC_LINKS=false` to require the key for links too).
- **Per-file passwords:** independent of the above. On `/p` you get a password form; on `/d` supply the
  password via `?pw=`, the `X-File-Password` header, or Basic Auth.

### Idle cleanup & permanent files

`CLEANUP` (default `1w`) deletes **permanent** files that haven't been accessed (via `/d` or `/p`)
within the period; temporary and download-limited files keep their own explicit lifecycles. Set
`CLEANUP=off` to disable it.

A file can opt out of cleanup with **keep** (truly permanent). Because a public instance would otherwise
fill up forever, keep is gated:

- **Public instance + cleanup on:** keep is never allowed — no file survives indefinitely.
- **Private instance:** keep requires an access key the admin granted the *"may upload permanent files"*
  permission (a checkbox when creating the key in `/admin`).
- **Cleanup off:** permanent means permanent; keep is unrestricted.

From the CLI: `filez file.zip --keep`. In the web UI a *"keep permanent"* checkbox appears when your key
is allowed; otherwise a hint shows the cleanup period.

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
filez keepme.pdf --keep            # exempt from idle cleanup (needs an authorized key)
filez data.bin   --host other      # pick a specific host
```

Short aliases mirror the flags: `-p`/`--permanent`, `-t`/`--temp`, `--pw`/`--password`,
`--dl`/`--downloads`, `-k`/`--keep`, `-H`/`--host`. The default upload mode comes from `$DEFAULT_UPLOAD`, then the
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

## KDE right-click menu (`filez menu`)

Add a **"Share with Filez"** entry to Dolphin's right-click menu — select one or more files, share them,
and the link lands in your clipboard with a notification.

```sh
filez menu install     # adds the Dolphin service menu + extracts the icon
filez menu uninstall
```

`menu install` extracts the embedded Filez icon to `~/.local/share/icons/hicolor/128x128/apps/filez.png`
and writes an executable service menu to `~/.local/share/kio/servicemenus/filez-share.desktop` (the
Plasma 6 location). The menu calls `filez share <files>`, which you can also use directly from scripts.

## Live sessions (`filez live`)

Turn your uploads into a live, auto-updating view — open one URL on a screen and every screenshot you
take appears there instantly, no links to click.

```sh
filez live          # start a session; prints & copies the viewer URL, e.g. <host>/l/<id>
# ...take screenshots / run `filez file.png` — each one replaces the live frame...
filez live          # run again to stop
```

While a session is active, **every** upload path — `filez <file>`, `filez share`, and the screenshot
hook — streams the file into the session instead of creating a link or storing it. The server keeps only
the latest frame in memory (nothing is persisted); the viewer at `/l/<id>` polls and swaps it in live.
The active session is recorded in `~/.config/filez/.filez_live`, so all `filez` processes (including a
running `filez hook watch`) pick it up. On a private instance, starting/pushing needs your access key;
the viewer link is public.

## Sync buckets (`filez sync`)

A temporary, in-memory shared drop — like LocalSend. An authorized user creates a bucket with a short
**4-digit code**; anyone with the link (`/s/<code>`) can upload, view and download files. Only the
creator can close it, and everything lives in memory (nothing persisted, auto-expired when idle).

```sh
filez sync                 # create a bucket; prints & copies the URL, e.g. <host>/s/4821
filez sync add report.pdf  # optionally push files from the CLI
filez sync close           # only the creator can close it
```

You can also create one from the web UI ("🔄 Sync-Bucket" on the start page) and share the code. The
bucket page has a simple drag & drop uploader and a live-updating file list with download buttons.
Creating a bucket needs the access key on a private instance; using an existing bucket only needs its code.

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
