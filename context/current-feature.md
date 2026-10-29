# Current Feature: Worker Phase 2 - Worker binary, safe migration, Docker

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] `cmd/worker/main.go`: reads `DATABASE_URL` (required), `DATA_DIR` (default `data`, same as the API, not created by the worker), `STEP_PAUSE`; retries `store.Open` for 60 seconds with its own copy of `waitStore`; runs `worker.New(st, dataDir, pause).Run(ctx)` under `signal.NotifyContext(SIGINT, SIGTERM)`; closes the store after `Run` returns
- [x] `STEP_PAUSE`: unset or empty means `2s`, `0` is allowed, an unparsable or negative value makes the worker exit at startup with a clear error naming `STEP_PAUSE`. Parsing lives in a small function in `cmd/worker/main.go`
- [x] Table-driven test `cmd/worker/main_test.go` for the `STEP_PAUSE` parsing: unset, empty, `0`, `2s`, `bogus`, `-1s`
- [x] `store.migrate` runs `schema.sql` in one transaction that first takes `SELECT pg_advisory_xact_lock(migrateLockKey)` (named constant in `store.go`), so API and worker can start at the same time; the `TestMain` comment in `store_test.go` is updated
- [x] Dockerfile build stage: `RUN CGO_ENABLED=0 go build -o /app/api ./cmd/api && CGO_ENABLED=0 go build -o /app/worker ./cmd/worker` exactly as in README.md (with the `\` line break); the final stage copies both binaries; `CMD ["/app/api"]` stays
- [x] `docker-compose.yml` gets the `worker` service exactly as in README.md (`command: ["/app/worker"]`, `STEP_PAUSE: 2s`, `boarddata:/data`, `depends_on` postgres healthy, `restart: "no"`); `api` unchanged
- [x] `internal/store/store_test.go`: every user a test creates is registered with `t.Cleanup`, which deletes that user's sessions, sheets and the user row (test-only SQL on `s.db`, no new `Store` method)
- [x] After the changes, show the `worker` block of `docker-compose.yml` and the README.md block side by side; they match word for word
- [x] Manual Docker check at the end of the phase, reported as a table "request, expected, actual":
  - `docker compose up --build -d`: no schema errors in the `api` and `worker` logs
  - `docker compose restart api worker`: no errors either
  - a sheet with a text brochure moves through the steps about every 2s and becomes `success`
  - mid-run `docker compose kill worker` leaves the sheet `running`; `docker compose start worker` continues from the first `pending` step with the same `runId`
  - deleting `body` mid-run gives `failed` and `Brochure is missing`
  - `docker compose run --rm -e STEP_PAUSE=bogus worker` and `-e STEP_PAUSE=-1s` exit with a clear error
  - two sheets of one account: the second `start` gives 409 (while the first is still `running`)
  - at the end `docker compose down -v`

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

- `main.go` stays wiring only: env, retry, signals. All logic lives in `internal/worker`. `worker.Run` is the existing `(*Worker).Run`, which waits for every sheet goroutine before it returns.
- Decided at load: named advisory lock constant in `store.go`; `waitStore` duplicated in `cmd/worker` (retry loops belong in `cmd/<binary>/main.go`), not shared; empty `STEP_PAUSE=` is treated as unset (same as the API's `env()` helper); `STEP_PAUSE` parsing gets a unit test; the worker does not `MkdirAll` `DATA_DIR` (a missing file already gives `Brochure is missing`).
- README asks for a retry of at least 30 seconds; the project overview and this spec fix it at 60 seconds for the worker. The API keeps its 30 seconds.
- `schema.sql` is multi-statement. Today it runs through `db.ExecContext` with no args, which pgx stdlib sends over the simple protocol. Check that `tx.ExecContext` with no args inside the transaction still runs all statements (verify against pgx v5.7.4 source/docs, not memory).
- The advisory lock is transaction-scoped (`pg_advisory_xact_lock`), so it is released on commit or rollback; no unlock call. The migrate timeout (10s today) also covers the wait for the lock.
- The `TestMain` comment in `store_test.go` says schema.sql runs without a lock; update it once the lock exists.
- Store integration tests today leave `running` sheets with a `filename` but no `body` in the shared database; a live worker would fail them with `Brochure is missing` while parallel tests still read them. Cleanup must delete in FK order: sessions, jobs, users.
- Known from worker phase 1, left as is: a write already in flight at SIGTERM may still commit (safe, every write is conditional); no new write starts after the cancel.
- Out of scope: page polling, `NOTES.md` (page phase 1); any change to `internal/worker` logic.
- Constraints: Go 1.23 APIs only (no `t.Context`), no new dependencies, `go test ./...` passes without Postgres, compose ships `STEP_PAUSE: 2s`.

### Testing (from the spec)

1. `docker compose up --build`: api and worker start with no schema errors. `docker compose restart api worker` also gives no errors.
2. curl with a cookie jar (`-c jar -b jar`): register, create, upload a `.txt`, start, then `GET /api/jobs/$ID` each second. Steps turn `done` about 2s apart, then `success`.
3. Mid-run, `docker compose kill worker`: the sheet stays `running`. `docker compose start worker` resumes from the first pending step with the same `runId`.
4. `docker compose exec api rm /data/$USER_ID/$ID/body` during a run (`USER_ID` from `GET /api/me`): `failed`, `error` = `Brochure is missing`.
5. `docker compose run -e STEP_PAUSE=bogus worker` and `-e STEP_PAUSE=-1s` both exit at startup with a clear error.
6. Store integration tests with only Postgres up, no `worker` service: `docker compose up -d postgres` (stop `worker` first), then `DATABASE_URL=postgres://board:board@localhost:54329/board?sslmode=disable go test ./internal/store -count=1 -v`. Afterwards `docker compose exec postgres psql -U board -d board -c "SELECT count(*) FROM users WHERE email LIKE 'test-%@example.com'"` gives `0`.

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
- Sheets Phase 2 - Start moves a draft to running: `Store.StartJob(id, userID, runID)` does a conditional `UPDATE ... WHERE status = 'draft'` and re-reads, so `runId` is chosen once even under concurrent starts; partial unique index `jobs_one_running_per_user` maps by `ConstraintName` to `board.ErrAlreadyRunning` -> 409 `Another sheet is already running`; non-draft sheets come back unchanged with 200; `mem` mirrors Postgres; `TestStartMarksTheJobSuccessful` rewritten to the spec; Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`). Known gap left for brochure phase 2: upload's whole-row `UpdateJob` can revert a concurrent start (spec now removes `UpdateJob` and adds `AND filename <> ''` to the start `UPDATE`)
- Brochure Phase 1 - Streamed upload, byte-based type, fixed path: upload streams via `r.MultipartReader()` (first `file` part wins) under `MaxBytesReader` (1 MiB + 64 KiB, `*http.MaxBytesError` -> 400 `Brochure is too large`, never 413, checked over real TCP); exact 1048576/1048577 limit; `board.BrochureType` decides PDF/text from bytes only (`headerAllows` removed); `board.BrochurePath` builds `{DATA_DIR}/{userID}/{jobID}/body` for upload and download; raw client filename from `Content-Disposition` is metadata only; new 400 `Brochure file name is not valid` for non-UTF-8 or >255-byte names (found by code-scanner review); check order is name, size, type, then `MkdirAll`/write, so rejected uploads leave `DATA_DIR` empty; `TestUploadKeepsTheClientFilename` rewritten to the spec. Left for brochure phase 2: tests for upload's 500 branches (rewritten by temp/claim/rename) and download's 404 on an empty `filename` (not yet in the phase 2 Testing list)
- Brochure Phase 2 - One brochure per sheet, safe download: early 400 `A brochure is already attached` before the body is read; upload writes `body-*.tmp`, claims the slot with one `UPDATE ... AND filename = '' RETURNING` (`board.ErrBrochureAttached`, called with `context.WithoutCancel`), then renames to `body`; a failed rename resets via `DetachBrochure` (`filename = $name AND status = 'draft'`) and gives 500; every rollback path removes the temp file; `UpdateJob` removed everywhere, so no code writes a whole sheet row (closes the lost update from sheets phase 2); `StartJob` gets `AND filename <> ''`, a still-`draft` result gives 400 `Attach a brochure first`; download sets `Content-Type` from `BrochureType` (fallback `application/octet-stream`), `nosniff`, `mime.FormatMediaType` disposition and `Last-Modified` from the file's mtime; client filename stored raw; `mem` mirrors Postgres; Postgres integration tests for attach/detach/start; race detector clean in `golang:1.23`; manual Docker check of both brochure phases passed. Rejected: a detach on any claim error (`filename` is not unique per request). Known trade-offs recorded in `context/project-overview.md` for `NOTES.md`
- Worker Phase 1 - Checklist runner: `internal/worker` with its own `Store` interface (`RunningJobs`, `SaveChecklist`, `FinishJob`, implemented by `*store.Store`, checked at compile time); `Run` polls every second, one goroutine per sheet under a mutex-guarded in-progress set (`release` deferred), `sync.WaitGroup` wait before return; no pause before the first step, `STEP_PAUSE` via `select` on `ctx.Done()` and a timer, brochure read with `os.ReadFile` before every step (a directory or missing file gives `failed` / `Brochure is missing`), `success` right after the third step, `success` at once on resume with every step `done`; every write `WHERE status = 'running' AND run_id`, step saves also `AND checklist = $prev` so a stale poll snapshot cannot redo or roll back a `done` step (L1 from code-scanner review); no row changed stops the sheet, DB errors are logged and retried next tick, no new write after cancel; 14 worker tests with a fake store mirroring Postgres, 100% statement coverage, mutation-checked; Postgres integration tests for the running poll, conditional writes incl. stale `prev`, and `ListJobs` after the `queryJobs` refactor; race detector clean in `golang:1.23`. Moved to worker phase 2: store integration tests must clean up their rows and run without the `worker` service (M1). Left as is: `FinishJob` accepts any status; `assertNoChange` ignores `updated_at`
