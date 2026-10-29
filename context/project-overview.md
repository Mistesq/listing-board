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
| `cmd/worker/main.go` | (to add) Entry point: reads env (`DATABASE_URL`, `DATA_DIR`, `STEP_PAUSE`), retries the DB for 60s, runs the worker loop, stops on SIGTERM/SIGINT |
| `internal/board/` | Domain types (`User`, `Job`, `Step`), `NewChecklist`, `NewID`/`NewToken`, sentinel errors, the `Store` interface. Gets the shared brochure path helper and the byte-based type detection |
| `internal/store/` | Postgres implementation of `board.Store` via `database/sql` + pgx. `schema.sql` is embedded and executed on every start, under an advisory lock |
| `internal/worker/` | (to add) Poll loop, per-sheet goroutines, step execution. Depends on a store interface so it is testable with a fake |
| `internal/httpapi/` | `Server`: routes, handlers, session cookie auth, JSON helpers, CORS for `localhost:5173`, static file serving of `WEB_DIR` |
| `web/` | Vite + React UI, all components in `src/App.tsx`, plain CSS in `src/styles.css` |
| `Dockerfile`, `docker-compose.yml` | Multi-stage image (web build, Go build of `api` and `worker`, alpine runtime); compose runs `postgres`, `api` and `worker`, the last two share the `boarddata` volume at `/data` |

## Data Model

`internal/store/schema.sql` (`CREATE TABLE IF NOT EXISTS`, no migration tool):

- `users`: `id`, `email` (unique), `password_hash` (bcrypt), `created_at`
- `sessions`: `token` (PK), `user_id`, `expires_at` (24h)
- `jobs` (a "sheet"): `id`, `user_id`, `title`, `address`, `status` (`draft`/`running`/`success`/`failed`), `error`, `run_id`, `filename`, `checklist` (JSON text of `[{label, state}]`), `created_at`, `updated_at`
- New: partial unique index on `jobs(user_id) WHERE status = 'running'` (`CREATE UNIQUE INDEX IF NOT EXISTS`), one running sheet per account.

IDs are random hex (16 bytes); session tokens are 32 bytes. Brochure bytes live on disk at `{DATA_DIR}/{userID}/{jobID}/body`.

Status transitions: `draft -> running` (API, `POST /start`), `running -> success | failed` (worker only). `success` and `failed` are final.

## Key requirements

### Ownership

- The store looks a sheet up by id **and** user id. A sheet owned by another account is indistinguishable from an unknown one: 404 on every id route (`GET /jobs/{id}`, upload, start, download).
- On upload, ownership is checked before the request body is read.

### Start

- A conditional `UPDATE ... SET status = 'running', run_id = $new WHERE id = $id AND user_id = $user AND status = 'draft'` performs the transition. If no row changed, the handler re-reads and returns the current sheet, so `runId` is chosen exactly once even under concurrent requests.
- The partial unique index guarantees one `running` sheet per account. Its violation maps to its own sentinel in `internal/board` (told apart by `PgError.ConstraintName`) and returns 409.
- The handler never reads the disk and never sets `success` or `failed`. Steps stay `pending`.

### Upload

- The multipart body is read as a stream (`r.MultipartReader()`), not `ParseMultipartForm`.
- The file limit is exact: 1048576 bytes pass, 1048577 bytes are 400. The request body limit (`MaxBytesReader`) leaves headroom for the multipart envelope. An oversized file, including one that trips `MaxBytesReader`, is 400 `Brochure is too large`, not 413.
- Cheap early check: after ownership and before the body is read, a sheet that already has a `filename` gets 400 `A brochure is already attached`. This is only a shortcut; the conditional `UPDATE` below is the guarantee.
- Type and size are validated before anything is written under `DATA_DIR`.
- Write order, so concurrent uploads never overwrite or delete another upload's `body`:
  1. Write the bytes to a temp file in the target directory.
  2. Claim the slot with a conditional `UPDATE ... SET filename = $name WHERE id = $id AND user_id = $user AND filename = ''` (own sentinel), not check-then-write.
  3. No row changed: remove the temp file, 400 `A brochure is already attached`.
  4. Row changed: rename the temp file to `body`.
  5. Rename failed: conditionally reset `filename = ''` (`WHERE id = $id AND filename = $name`), remove the temp file, 500 `Could not store the brochure`.
- Type comes from the bytes only: PDF starts with `%PDF-`; text is valid UTF-8, has no NUL byte and does not start with `%PDF-`; an empty file or anything else is 400.
- `{DATA_DIR}/{userID}/{jobID}/body` is built by one shared helper in `internal/board`, used by both the API and the worker. Client input never takes part in a path.

### Download

- `Content-Type` is set explicitly from the stored bytes: `application/pdf` or `text/plain; charset=utf-8`.
- `X-Content-Type-Options: nosniff`.
- `Content-Disposition` is built with `mime.FormatMediaType("attachment", {"filename": ...})`.

### Worker

- Binary `cmd/worker`, same image. Retries the DB connection for 60 seconds.
- The schema migration is safe when API and worker start at the same time: `pg_advisory_lock` (or `pg_advisory_xact_lock` in one transaction) around executing `schema.sql`.
- Polls `running` sheets, one goroutine per sheet, and tracks sheets already in progress so a sheet is never processed twice by the same process.
- No pause before the first step. For each step: check the brochure is readable, mark the first `pending` step `done`, save, then wait `STEP_PAUSE`. A `done` step is never redone.
- Every write is conditional: `WHERE status = 'running' AND run_id = $runId`. A write that changes no row stops processing that sheet. A DB error while polling or saving is logged and retried on the next tick; the worker keeps running and the sheet stays `running`.
- On SIGTERM the worker stops without writing anything, so a killed worker leaves the sheet `running`. After a restart it continues from the first `pending` step with the same `runId`.
- Brochure cannot be read: `status = 'failed'`, `error` is exactly `Brochure is missing`.

### Docker

- The build stage builds `/app/api` and `/app/worker` exactly as in README.md; the final stage copies both binaries.
- The `worker` service in `docker-compose.yml` matches README.md exactly (`STEP_PAUSE: 2s`, `restart: "no"`, `boarddata:/data`).

### Tests

- `TestStartMarksTheJobSuccessful` asserts: status `running`, non-empty `runId`, all steps `pending`, a repeated start returns the same `runId`.
- `TestUploadKeepsTheClientFilename` asserts: the brochure is at `{userID}/{jobID}/body`, `filename` holds the client name, a traversal name creates nothing outside that directory.
- The `mem` store implements every new store method with the same semantics as Postgres (conditional updates, one running sheet per account, one brochure per sheet, same sentinels).
- Worker logic is covered by tests with a fake store and `t.TempDir()`, no Postgres.

### Page

- The open sheet and its status in the list update while it is `running`.
- `error` is shown when the sheet has `failed`.
- The 409 message from `POST /start` is shown.
- A failed upload after create still adds the sheet to the list; an open `draft` without a brochure shows a file input to attach one.
- The `Job` type in `App.tsx` gets `runId`.

### NOTES.md

- At the repo root: one rejected AI suggestion and how the result was checked, plus a few lines on decisions where the spec leaves room.

## Integrations

Postgres only. No external services.

## Run and Test

See Commands in `CLAUDE.md`. `go test ./...` uses an in-memory fake store and `t.TempDir()`; no Postgres or network.

## Known Gaps (code vs README spec)

- `POST /start` sets `success` and ticks every step inside the handler. Spec: move to `running`, set `runId`, leave steps `pending`; 409 if another sheet of the account is `running`; idempotent while `running`.
- No `cmd/worker`; compose has no `worker` service; the Dockerfile builds only `api`.
- `Server.load` does not check ownership, so `GET /jobs/{id}`, upload, start and download work on other accounts' sheets (spec: 404).
- Upload: stores at `{DATA_DIR}/{jobID}/{client filename}` (path traversal possible), limit 8 MiB (spec: 1 MiB), type taken from header/extension (spec: from bytes), second upload allowed (spec: 400). Spec path: `{DATA_DIR}/{userID}/{jobID}/body`.
- Schema runs without a lock; no partial unique index for running sheets.
- UI does not refresh the open sheet while running; the `Job` type in `App.tsx` has no `runId`.
- `NOTES.md` is missing.

## Assumptions

- Start on a final sheet: `POST /start` on `failed` or `success` returns 200 and the sheet unchanged. A finished sheet stays as it is. Alt: 409.
- Rewritten tests: both keep their names; extra coverage goes into new tests. Alt: subtests inside them.
- Filename: `filename` stores the raw client name from the multipart `Content-Disposition` (e.g. `../../escaped.txt`). Metadata only, never used in a path. Alt: store the base name.
- Brochure check: the worker checks the brochure is readable before every step, not only the first. A file lost mid-run fails the sheet. Alt: first step only.
- Success write: after the third step is saved, the worker sets `success` right away, without a pause, as a separate save. Alt: same write as the third step.
- Page refresh: the page polls `GET /api/jobs/{id}` about once a second while the open sheet is `running`, and updates the list entry from the response. No SSE. Alt: SSE.
- `STEP_PAUSE`: unset means `2s`, `0` is allowed, an unparsable or negative value makes the worker exit at startup with a clear error. Alt: fall back to `2s`.
- Check order on id routes: 401, then 404 (unknown or another account's sheet), then route checks. For start: 400 without a brochure, then status logic, then 409.
- Error texts not fixed by README: existing texts in the code stay as they are (missing `file` field `Choose a brochure file`, wrong or empty type `Brochure must be a PDF or a text file`, oversized file `Brochure is too large`, broken multipart `Brochure upload was incomplete`, disk/store failure `Could not store the brochure`). New texts only for new cases: 409 on start `Another sheet is already running`; second upload `A brochure is already attached`; invalid client filename `Brochure file name is not valid`.
- Filename check: a client filename that is not valid UTF-8 or is longer than 255 bytes (counted on the raw name, stored as sent) gives 400 `Brochure file name is not valid`, checked before anything is written under `DATA_DIR`, so Postgres never sees it and no orphan `body` is left. Alt: reuse `Choose a brochure file`.

## Known trade-offs

Accepted on purpose; each goes into `NOTES.md`.

- Claim/rename window: upload claims the slot in the database (`filename` set) before it renames the temp file to `body`. A start that lands in that short window lets the worker find no file and fail the sheet with `Brochure is missing`. A normal user cannot hit this, because the upload response arrives only after the rename. Not made more complex.
- Separate `SELECT` after the `UPDATE` in `StartJob`: the conditional `UPDATE` chooses `runId` once, then `JobForUser` re-reads the sheet in a second round trip instead of `UPDATE ... RETURNING`. The response can already show a later write (e.g. a worker step), and a re-read that fails after a committed start gives 500, while a retry returns the `running` sheet with 200. Left as it is.
- Lost database connection in the middle of the `UPDATE` in `AttachBrochure`: the outcome is unknown (the claim may be committed), the upload removes its temp file and answers 500, so the sheet can be left with `filename` and no `body`. `AttachBrochure` is a single `UPDATE ... RETURNING` called with `context.WithoutCancel`, so a client hanging up does not cause this; only a broken database connection does. Not reset blindly, because `DetachBrochure` matches on `filename`, which is not unique per request.
- Temp files before the claim: every concurrent upload to a sheet writes its validated bytes (up to 1 MiB) to its own `body-*.tmp` before the claim decides the winner; losers remove theirs.
- Stale `body-*.tmp` after a crash: a process killed between writing the temp file and the rename leaves it behind, and nothing sweeps them.

## Open Questions

None.

## Roadmap

1. `context/features/sheets-phase-1-spec.md`: ownership lookup on every id route, `mem` parity rule. Depends on: none.
2. `context/features/sheets-phase-2-spec.md`: start draft -> running, partial unique index, 409. Depends on: 1.
3. `context/features/brochure-phase-1-spec.md`: streamed upload, exact limit, byte type, shared path helper. Depends on: 1.
4. `context/features/brochure-phase-2-spec.md`: one brochure per sheet (temp, claim, rename), download headers. Depends on: 3.
5. `context/features/worker-phase-1-spec.md`: `internal/worker` checklist runner with conditional writes. Depends on: 2, 3.
6. `context/features/worker-phase-2-spec.md`: `cmd/worker`, advisory-locked migration, Dockerfile and compose. Depends on: 5.
7. `context/features/page-phase-1-spec.md`: page polling, failed error, 409 message, `NOTES.md`. Depends on: 2, 4, 6.

## Out of scope

Known, intentionally not fixed in this task:

- No index on `jobs.user_id` besides the partial running index; expired sessions are never deleted; `GET /api/jobs` is unpaginated.
- No lint setup and no frontend tests.
