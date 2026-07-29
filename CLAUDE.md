# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`files.md` is a local-first markdown note app (notes, journal, tasks, checklists, habits) with a PWA frontend, a single-binary Go server, and a Telegram bot. Everything is stored as plain `.md` files. The whole project is designed for low cognitive load — junior devs or an LLM should be able to fit the whole codebase in their head.

## Build / test / lint

All common workflows are in the `Makefile` (DOCKER defaults to `docker`, override with `make docker_build DOCKER=podman`):

| Goal | Command |
|---|---|
| Run dev server locally | `make server` (`go run ./cmd/server`) |
| Run all Go tests | `make test` (`go test ./...`) |
| Run a single Go test | `go test ./server/fs/ -run TestWrite -v` |
| CI gate (fmt + vet + test) | `make check` |
| Lint | `make lint` (`golangci-lint run`) |
| Format | `make format` (`gofumpt -w .`) |
| E2E tests (headless) | `make e2e test="name pattern"` |
| E2E tests (headed) | `make e2eh test="name pattern"` |
| Single E2E test | `make e2es test="name"` / `make e2esh test="name"` |
| Sync-flow E2E | `make sync` / `make synch` |
| Perf benchmark E2E | `make perf` |
| Build + start Docker | `make compose_up` |
| Stop Docker | `make compose_down` |
| Deploy (systemd) | `make deploy_systemd host=user@host` |
| Deploy (raw binary) | `make deploy_binary host=user@host` |
| Run lite launcher (current OS) | `make lite` |
| Build lite binary (current OS) | `make lite_build` |
| Build lite Windows .exe (silent GUI) | `make lite_win` |
| Build lite Windows .exe (with console) | `make lite_win_console` |
| Force browser mode at runtime | `lite.exe --browser` |

Server config comes from `.env` (loaded via `godotenv`); see `docs/your-own-server.md` for the env vars (`BOT_API_TOKEN`, `STORAGE_DIR`, `TOKENS_DIR`, `CERT_DIR`, `API_URL`, `APP_URL`).

The Go module is `github.com/zakirullin/files.md`, Go 1.24.

## Repository layout

```
web/                     PWA frontend (no build system — plain HTML/JS)
  index.html             single entrypoint
  lib/                   vendored frontend libs (mark `PATCHED` if modified)
  lib/latex/             LaTeX font files
cmd/
  server/                server entrypoint (HTTP + Telegram bot + sync API)
  backlink/              inserts backlinks between notes
  shifttime/             shifts journal timestamps (e.g. after TZ change)
  tomdlinks/             [[wikilinks]] → standard markdown links
  whoop/                 appends Whoop metrics to journal
lite_main.go            root-level `package main`: embeds web/ + serves on localhost + opens native WebView2 window (falls back to default browser if WebView2 runtime is missing). `--browser` flag forces browser mode.
server/                  server-side Go packages
  bot.go                 main Telegram bot logic (message routing, buttons)
  chat.go                chat-related handlers
  fs/                    filesystem abstraction (per-user isolated, afero-backed)
  sync/                  sync HTTP API + append-only rename/delete log
  db/                    user-scoped state (no global state, abstracted from Redis)
  userconfig/            per-user config (read from disk on every access — see ADRs)
  journal/               journal file parsing/rendering
  habits/                habits tracking (cron-driven)
  i18n/                  bot localization
  plugins/               bot plugins (e.g. world clock)
  stats/                 lightweight counters
  config/                env-driven config loading
  pkg/{slice,tg,txt}/    small shared helpers (keep these tiny)
tests/                   Playwright E2E suite (npm-based, run via make)
docs/                    bot.md, sync-flow.md, e2e-tests.md, your-own-server.md
vendor/                  vendored Go deps (committed; no go module proxy needed)
```

The server binary serves both the PWA at `/` and the sync API at `/api/...` (when `API_URL` is set).

## Backend conventions (from README + ADRs)

- **No panics** in business logic — errors are part of the contract.
- **Wrap errors** with method context (`fmt.Errorf("fs.Put: %w", err)`).
- If an error is intentionally ignored, leave a **WHY comment**.
- **No `get*` prefix** on methods — `UserName()` not `GetUserName()`.
- Prefer **real implementations or fakes** over mocks/stubs.
- **Granular locks**, not one global per-user lock — workers may hold third-party calls (e.g. ChatGPT) and we don't want to serialize unrelated resources (`db`, `journal`, `userconfig` each have their own).
- **Per-user updates are sequential** to avoid race conditions between bot + workers writing user files.
- **Read userconfig from disk on every access** — never cache it across a method that may yield to network. Otherwise a concurrent `worker.MoveDueTasks()` can clobber a bot write-back with stale data.
- **Sanitize early** at system boundaries (don't sanitize in `Path` accessor — it breaks paths).
- Rename imports only to avoid collisions.
- **Portability**: filenames restricted to chars valid on Windows/PWA (no `:?<>*`).
- Only **one level of nesting** under root. Any file is uniquely identified by `(dir, filename)`.
- Use `fs.Hash` (md5) for user-supplied strings going into Telegram `callbackData` (max 64 bytes).
- Bot inline-query entries are identified by a stable content hash, not a positional index — so buttons survive inserts/deletes.

### File time semantics

- `ctime` — file location/structure changes (rename, move, archive). Used to track where a file lives.
- `mtime` — content changes. Used for synchronization (mtime survives Dropbox metadata rewrites, unlike ctime; also restoreable from `.git`).
- Sync uses **microsecond** timestamps (not nanoseconds, because JS int64 precision issues).
- Filenames are hashed (`fs.Hash`) where they'd exceed Telegram callback limits.

## Frontend conventions

- **No build system** — `web/index.html` must work as-is in 10 years. Plain `<script>` tags, vendored libs in `web/lib/`.
- If you modify a vendored lib, mark it with the **`PATCHED`** keyword in a comment near the change so it can be upstreamed or re-vendored cleanly.
- Long-term goal: replace CodeMirror with a tiny in-house editor.
- Mind **TOCTOU** races between async lock checks and lock acquires.
- Most bugs are race conditions where an async flow is interrupted mid-way.
- Avoid **flaky E2E tests** — a single flake makes the whole suite get skipped.
- Persisted FS driver is **OPFS** by default; users can switch to Local File System API by opening a folder (handle stored in IndexedDB).

## Markdown storage layout

Files live in `STORAGE_DIR/<chatID>/`. Reserved names:

- `Chat.md`, `Later.md`, `Done.md`
- `journal/YYYY.MM Month.md`
- `habits/*.md`
- `media/*` (png/jpg/webp/gif)
- `archive/*.md`
- `config.json`

The scheme is published at `files.md/llms.txt` and is meant to be copy-pasted into consumer `CLAUDE.md` / `AGENTS.md`.

## Glossary (use these exact terms)

- `filename` — name with extension, e.g. `note.md` (used as the canonical ID)
- `header` — extension-stripped, capitalized filename, e.g. `Note`
- `body` — file content
- `dir` — a category folder, e.g. `happiness`
- `userID` — Telegram chatID (PM with the bot, used as the user identity)
- `ctime` / `mtime` — see File time semantics above

## Deployment notes

- Single static binary at `/app/server`, systemd unit `filesmd.service`.
- Storage and token volumes are **named Docker volumes** (`files-md-storage`, `files-md-tokens`) — survive `compose down`.
- Web assets are repackaged on each `deploy_*` with the git short SHA appended as `?v=` cache-buster on every `<script>`/`<link>` that uses one.
- HTTPS is enabled when `CERT_DIR` is set to a dir containing cert/key files (see `compose.yaml` for the port mapping).
- `make init_server host=... salt=...` creates remote dirs, `.env`, and the systemd unit in one shot.

## When adding code

- Prefer **removing or simplifying** code over adding it (project ethos).
- Avoid new dependencies if possible — all deps are vendored and our responsibility.
- Update the ADR list at the bottom of `README.md` for any meaningful architectural decision (date + one-paragraph rationale).
- After writing meaningful code: run `make check` and `make lint`. Tests are mandatory on the backend.