# Listing board: Project Overview

## Purpose

A small board for real-estate listing sheets. An account signs in, creates a sheet for one address, attaches a brochure (PDF or text), and starts it. A worker process walks the sheet through a three-step checklist while the page shows progress.

This is a take-home task. **`README.md` is the spec** (routes, status codes, sheet shape, worker and brochure rules). Where code disagrees with it, change the code. Task constraints from the README:

- Rewrite `TestStartMarksTheJobSuccessful` and `TestUploadKeepsTheClientFilename` to describe the spec, then make them pass. Do not skip, delete or weaken them.
- Keep `TestListHidesOtherUsersJobs` and `TestStartRequiresASession` as they are.
- Add `NOTES.md` at the root: one rejected AI suggestion and how the result was checked.
- Keep the API routes, `{"error":"..."}` error shape and `lb_session` cookie.

## Modules

| Path | Role |
| --- | --- |
| `cmd/api/main.go` | Entry point: reads env (`DATABASE_URL`, `DATA_DIR`, `PORT`, `WEB_DIR`), retries the DB for 30s, serves HTTP, graceful shutdown |
| `internal/board/` | Domain types (`User`, `Job`, `Step`), `NewChecklist`, `NewID`/`NewToken`, sentinel errors, the `Store` interface |
| `internal/store/` | Postgres implementation of `board.Store` via `database/sql` + pgx. `schema.sql` is embedded and executed on every start |
| `internal/httpapi/` | `Server`: routes, handlers, session cookie auth, JSON helpers, CORS for `localhost:5173`, static file serving of `WEB_DIR` |
| `web/` | Vite + React UI, all components in `src/App.tsx`, plain CSS in `src/styles.css` |
| `Dockerfile`, `docker-compose.yml` | Multi-stage image (web build, Go build, alpine runtime); compose runs `postgres` and `api` with a `boarddata` volume at `/data` |

## Data Model

`internal/store/schema.sql` (`CREATE TABLE IF NOT EXISTS`, no migration tool):

- `users`: `id`, `email` (unique), `password_hash` (bcrypt), `created_at`
- `sessions`: `token` (PK), `user_id`, `expires_at` (24h)
- `jobs` (a "sheet"): `id`, `user_id`, `title`, `address`, `status` (`draft`/`running`/`success`/`failed`), `error`, `run_id`, `filename`, `checklist` (JSON text of `[{label, state}]`), `created_at`, `updated_at`

IDs are random hex (16 bytes); session tokens are 32 bytes. Brochure bytes live on disk under `DATA_DIR`.

## Integrations

Postgres only. No external services.

## Run and Test

See Commands in `CLAUDE.md`. `go test ./...` uses an in-memory fake store and `t.TempDir()`; no Postgres or network.

## Known Gaps (code vs README spec)

- `POST /start` sets `success` and ticks every step inside the handler. Spec: move to `running`, set `runId`, leave steps `pending`; 409 if another sheet of the account is `running`; idempotent while `running`.
- No `cmd/worker`; compose has no `worker` service; the Dockerfile builds only `api`.
- `Server.load` does not check ownership, so `GET /jobs/{id}`, upload, start and download work on other accounts' sheets (spec: 404).
- Upload: stores at `{DATA_DIR}/{jobID}/{client filename}` (path traversal possible), limit 8 MiB (spec: 1 MiB), type taken from header/extension (spec: from bytes), second upload allowed (spec: 400). Spec path: `{DATA_DIR}/{userID}/{jobID}/body`.
- UI does not refresh the open sheet while running; the `Job` type in `App.tsx` has no `runId`.
- `NOTES.md` is missing.

## Out of scope

Known, intentionally not fixed in this task:

- No index on `jobs.user_id`; expired sessions are never deleted; `GET /api/jobs` is unpaginated.
- No lint setup and no frontend tests.
