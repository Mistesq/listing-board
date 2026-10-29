# Current Feature: Sheets Phase 2 - Start moves a draft to running

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] `schema.sql`: `CREATE UNIQUE INDEX IF NOT EXISTS jobs_one_running_per_user ON jobs (user_id) WHERE status = 'running'`
- [x] New sentinel `board.ErrAlreadyRunning` (not `ErrDuplicate`); the store maps a `23505` with `PgError.ConstraintName == "jobs_one_running_per_user"` to it
- [x] New store method `StartJob(ctx, id, userID, runID string) (board.Job, error)`: conditional `UPDATE jobs SET status = 'running', run_id = $runId, updated_at = $now WHERE id = $id AND user_id = $user AND status = 'draft'`; no row changed -> the store re-reads and returns the current sheet, so `runId` is chosen exactly once
- [x] `mem.StartJob` mirrors Postgres: only `draft` changes; another `running` sheet of the same user gives the same sentinel
- [x] Handler: 400 `Attach a brochure first` when `filename` is empty; otherwise conditional start with `board.NewID()`; `running`/`success`/`failed` come back unchanged with 200; 409 `Another sheet is already running`; 500 `Could not start the sheet`. Never reads the disk, never sets `success`/`failed`, steps stay `pending`
- [x] Rewrite `TestStartMarksTheJobSuccessful` (keep the name): status `running`, non-empty `runId`, all three steps `pending`, a second start returns the same `runId`
- [x] New tests: 409 with exact text for a second sheet of the same account; 400 without a brochure; `failed`/`success` (set through `mem`) unchanged; 10 concurrent starts on one draft all return the same `runId`
- [x] Postgres integration tests in `internal/store/store_test.go`:
  - `t.Skip` when `DATABASE_URL` is unset, so `go test ./...` still passes without a database
  - open through `store.Open(DATABASE_URL)`, so the schema and the index come from the production code path
  - each test creates its own users with a unique email (e.g. from `board.NewID()`); no shared state, no table cleanup
  - cover: `JobForUser` returns `ErrNotFound` for another account's sheet; `StartJob` moves a draft to `running` with a `runId`, a repeated call returns the same `runId`; 10 concurrent `StartJob` on one draft return one `runId`; a second draft of the same user while one is `running` gives `ErrAlreadyRunning`; `failed` and `success` sheets stay unchanged

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

### Route contract

`POST /api/jobs/{id}/start` -> 200 with the sheet, or 401 / 404 / 400 `Attach a brochure first` / 409 `Another sheet is already running` / 500 `Could not start the sheet`.

Check order: 401, 404, 400 without a brochure, then status logic (non-draft gives 200 unchanged), then 409. The conditional `UPDATE` gives this order by itself: a non-draft sheet changes no row and never touches the index.

### Constraints

- `TestStartRequiresASession` and `TestListHidesOtherUsersJobs`: bodies unchanged.
- Handler depends on `board.Store`, not `*store.Store`; check sentinels with `errors.Is`.
- Only `draft -> running` here. `running -> success | failed` belongs to the worker (worker phases).
- `store.Open` runs `schema.sql` without a lock until worker phase 2, and concurrent `CREATE ... IF NOT EXISTS` can race in Postgres. The store tests open the store once per package (shared helper) and only the test bodies run concurrently.

### Known gaps (closed in brochure phase 2)

- A. Upload can revert a concurrent start. `upload` reads the sheet (`server.go:220`), parses the body and writes the file, then writes the whole row through `UpdateJob` (`server.go:260`), which sets `status`, `run_id` and `checklist` from the stale copy `WHERE id` only. Interleaving: a sheet with a brochure gets a second upload (still allowed) and a start at the same time; `StartJob` sets `running` with `runId=R`, then the upload's `UpdateJob` writes it back to `draft` with an empty `runId`, frees the index slot, and a later start picks a new `runId`. Breaks "runId is chosen once". Decision: known gap, not fixed here; brochure phase 2 replaces this write with the conditional claim and removes `UpdateJob`.
- C. `start` checks `filename` on a stale read. Safe today because nothing clears `filename`; brochure phase 2 adds a conditional reset to `''` after a failed rename, so the start `UPDATE` gets `AND filename <> ''` there.
- Not changing: `StartJob` runs the `UPDATE` and then a separate `SELECT`. If the `SELECT` fails after the commit, the client gets 500 while the sheet is `running`; a retry returns the same `runId`.

### Out of scope

- Advisory lock around `schema.sql` (worker phase 2).
- Worker, Docker/compose changes, page polling.

### Testing

1. `go test ./internal/httpapi -run Start -v`: rewritten test passes; new tests for 409 (exact text), 400 without brochure, `failed`/`success` unchanged, 10 concurrent starts with one `runId`.
2. Store integration tests on real Postgres (PowerShell):
   ```powershell
   docker compose up -d postgres
   $env:DATABASE_URL="postgres://board:board@localhost:54329/board?sslmode=disable"; go test ./internal/store -v
   ```
   Without `DATABASE_URL`, `go test ./internal/store -v` reports the tests as skipped.
3. After worker phase 2, with Docker: start two sheets of one account; the second `curl -b jar -X POST localhost:8080/api/jobs/$ID2/start` gives 409.

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
