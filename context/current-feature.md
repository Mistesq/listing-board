# Current Feature: Worker Phase 1 - Checklist runner

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] `internal/worker` declares a small store interface; `*store.Store` implements it with new methods (poll `running` sheets across all accounts, conditional step save, conditional final status write). Checklist JSON stays in the store
- [x] `Run(ctx)` polls `running` sheets on a ticker (about every second) and starts one goroutine per sheet; a mutex-guarded in-progress set skips sheets already being processed and drops a sheet when its goroutine ends
- [x] Per sheet: no pause before the first step. For each step: check the brochure at `board.BrochurePath` can be read, mark the first `pending` step `done`, save, then wait `STEP_PAUSE`. A `done` step is never redone
- [x] After the third step is saved, `success` is set right away (no pause) in a separate save
- [x] Every write is `WHERE id = $id AND status = 'running' AND run_id = $runId` and reports whether a row changed; no row changed stops processing that sheet
- [x] A DB error while polling or saving is logged, ends that sheet's goroutine (it leaves the in-progress set) and is retried on the next tick; `Run` keeps running, the sheet stays `running` and resumes from its first `pending` step
- [x] Context cancel (SIGTERM) makes the worker return without writing anything; the sheet stays `running`. The worker never changes `runId`
- [x] Brochure cannot be read: conditional write of `status = 'failed'`, `error` exactly `Brochure is missing`
- [x] Pause uses `select` on `ctx.Done()` and a timer, not `time.Sleep`
- [x] Tests in `internal/worker/worker_test.go` with a fake store and `t.TempDir()`, no Postgres
- [x] `Run` waits for all its sheet goroutines (`sync.WaitGroup`) before it returns, so SIGTERM ends the process cleanly with no write after cancel
- [x] Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`), as in earlier phases: the conditional step, `success` and `failed` writes change the row only when `status = 'running'` and `run_id` matches; with an old `run_id` or another status nothing changes; the `running` poll returns sheets of all accounts with `UserID` set

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

- Spec: `context/features/worker-phase-1-spec.md`. Files: `internal/worker/worker.go`, `internal/worker/worker_test.go`, `internal/store/store.go` (new methods).
- README rules: mark the first `pending` step `done`, save, wait `STEP_PAUSE`; a `done` step is not done again; after the third step is saved the sheet becomes `success`; unreadable brochure gives `failed` with `error` exactly `Brochure is missing`; killing the worker leaves the sheet `running`, a restart continues from the first pending step with the same `runId`.
- Project assumptions (`context/project-overview.md`): the brochure is checked before every step, not only the first; `success` is a separate save right after the third step, no pause.
- Writes also set `updated_at = time.Now().UTC()`. The worker reads `UserID` from `board.Job` (`json:"-"`) to build `BrochurePath`.
- The worker interface lives in `internal/worker`, not in `board.Store`, so the httpapi `mem` store does not need the new methods.
- Go 1.23 only: no `t.Context`, no `testing.B.Loop`. Logging with the stdlib `log` package, as in `cmd/api` and `httpapi`.
- Decisions made at load:
  - Resume with every step already `done` (crash between the third step save and the `success` write): write `success` right away, no brochure check, since no step is left to run.
  - "Brochure can be read" means open and read the file at `board.BrochurePath`, not `os.Stat`; a permission error or unreadable file also gives `Brochure is missing`.
  - API shape: `worker.New(store, dataDir, pause)`; the poll interval is a field tests can shorten. `STEP_PAUSE` env parsing stays in worker phase 2.
  - The worker tests use their own fake store; the httpapi `mem` store does not get the new methods.
- Decisions made at the code-scanner review:
  - L1 fixed: a poll snapshot can be stale when the sheet's previous goroutine saved more steps, failed on a DB error and released between the poll query and `claim`; a save from it would rewrite a `done` step (with `STEP_PAUSE=0` even roll one back to `pending`). `SaveChecklist(ctx, id, runID, prev, next)` now also has `AND checklist = $prev` (JSON text, both sides from `json.Marshal` of `[]board.Step`), so a stale save changes no row and the goroutine stops; the next poll starts from the stored checklist. The fake store compares `prev` the same way. Tests: `TestProcessStopsOnAStaleSnapshot` (worker) and the `stale checklist` case of `TestSaveChecklistNeedsTheCurrentRun` (Postgres).
  - M1 moved to worker phase 2 as a requirement: store integration tests leave `running` sheets without a brochure in the shared database, which a live worker would fail. Phase 2 adds `t.Cleanup` that deletes each test user's sessions, sheets and user row, and runs these tests with only Postgres up, without the `worker` service. Until then, run `go test ./internal/store` with `DATABASE_URL` only while no worker is running.
  - Also fixed: the `Run` doc comment now says no write starts after cancel (one in flight may commit); `TestPollNeverStartsASecondGoroutineForASheet` no longer hangs or leaks a goroutine on failure.
  - Left as is: `FinishJob` accepts any status (only the worker calls it); `assertNoChange` does not compare `updated_at` (real queries always change it).
- Out of scope (worker phase 2): `cmd/worker`, `STEP_PAUSE` env parsing, advisory-locked migration, Dockerfile and compose.

### Testing (from spec)

1. `go test ./internal/worker -v` (pause of a few ms):
   - happy path: 3 step saves then `success`
   - missing file: `failed` / `Brochure is missing`
   - step 1 already `done`: only 2 step saves, same `runId`
   - cancel during the pause: no write after it
   - a changed `runId` stops the worker
   - a save from a stale snapshot changes no row and stops the goroutine
   - two polls never start a second goroutine for one sheet
   - a fake store that fails the poll once, then one step save, keeps `Run` alive; the sheet still ends `success` with each step `done` once
2. `go test ./...` and `go vet ./...` pass.
3. With `DATABASE_URL` set and no worker running: `go test ./internal/store -v` runs the new integration tests (conditional writes incl. a stale `prev` checklist, all-account `running` poll with `UserID`).

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
- Sheets Phase 2 - Start moves a draft to running: `Store.StartJob(id, userID, runID)` does a conditional `UPDATE ... WHERE status = 'draft'` and re-reads, so `runId` is chosen once even under concurrent starts; partial unique index `jobs_one_running_per_user` maps by `ConstraintName` to `board.ErrAlreadyRunning` -> 409 `Another sheet is already running`; non-draft sheets come back unchanged with 200; `mem` mirrors Postgres; `TestStartMarksTheJobSuccessful` rewritten to the spec; Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`). Known gap left for brochure phase 2: upload's whole-row `UpdateJob` can revert a concurrent start (spec now removes `UpdateJob` and adds `AND filename <> ''` to the start `UPDATE`)
- Brochure Phase 1 - Streamed upload, byte-based type, fixed path: upload streams via `r.MultipartReader()` (first `file` part wins) under `MaxBytesReader` (1 MiB + 64 KiB, `*http.MaxBytesError` -> 400 `Brochure is too large`, never 413, checked over real TCP); exact 1048576/1048577 limit; `board.BrochureType` decides PDF/text from bytes only (`headerAllows` removed); `board.BrochurePath` builds `{DATA_DIR}/{userID}/{jobID}/body` for upload and download; raw client filename from `Content-Disposition` is metadata only; new 400 `Brochure file name is not valid` for non-UTF-8 or >255-byte names (found by code-scanner review); check order is name, size, type, then `MkdirAll`/write, so rejected uploads leave `DATA_DIR` empty; `TestUploadKeepsTheClientFilename` rewritten to the spec. Left for brochure phase 2: tests for upload's 500 branches (rewritten by temp/claim/rename) and download's 404 on an empty `filename` (not yet in the phase 2 Testing list)
- Brochure Phase 2 - One brochure per sheet, safe download: early 400 `A brochure is already attached` before the body is read; upload writes `body-*.tmp`, claims the slot with one `UPDATE ... AND filename = '' RETURNING` (`board.ErrBrochureAttached`, called with `context.WithoutCancel`), then renames to `body`; a failed rename resets via `DetachBrochure` (`filename = $name AND status = 'draft'`) and gives 500; every rollback path removes the temp file; `UpdateJob` removed everywhere, so no code writes a whole sheet row (closes the lost update from sheets phase 2); `StartJob` gets `AND filename <> ''`, a still-`draft` result gives 400 `Attach a brochure first`; download sets `Content-Type` from `BrochureType` (fallback `application/octet-stream`), `nosniff`, `mime.FormatMediaType` disposition and `Last-Modified` from the file's mtime; client filename stored raw; `mem` mirrors Postgres; Postgres integration tests for attach/detach/start; race detector clean in `golang:1.23`; manual Docker check of both brochure phases passed. Rejected: a detach on any claim error (`filename` is not unique per request). Known trade-offs recorded in `context/project-overview.md` for `NOTES.md`
