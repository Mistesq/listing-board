# Current Feature: Page Phase 1 - Live sheet progress and NOTES.md

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] `Job` type in `web/src/App.tsx` gets `runId: string`
- [x] While the open sheet is `running`, poll `GET /api/jobs/{id}` about once a second (`setTimeout` chained after each response, not `setInterval`); update both the open sheet and its list entry from the response
- [x] Polling stops when the status leaves `running`, when another sheet is opened, and on unmount (cleanup in the `useEffect` return); a response for a sheet that is no longer open is ignored
- [x] A `failed` sheet shows its `error` in `<p className="error">`
- [x] The error from `POST /start` is shown, including 409 `Another sheet is already running`
- [x] A failed upload after create still adds the sheet to the list and shows the error
- [x] An open `draft` without a brochure shows a file input that uploads to `POST /api/jobs/{id}/file` and updates the sheet (and list entry) from the response
- [x] `NOTES.md` at the repo root: one rejected AI suggestion and how the result was checked; a few lines on decisions where the spec leaves room (start on final sheets, filename kept raw, brochure check per step, page polling, `STEP_PAUSE` parsing)

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

- Files: `web/src/App.tsx`, `web/src/styles.css` (only if needed), `NOTES.md`. No Go changes.
- No new npm dependencies, no SSE, no router or data library. All requests go through `api<T>()`.
- React 18.3.1, TypeScript strict. Style: double quotes, semicolons, trailing commas; local `useState`; errors via an `error` string and `<p className="error">`.
- API contract used: `GET /api/jobs/{id}` (200 sheet, 404 `Not found`), `POST /api/jobs/{id}/start` (200 sheet, 400 `Attach a brochure first`, 409 `Another sheet is already running`), `POST /api/jobs/{id}/file` (multipart field `file`; 200 sheet; 400 texts from the upload rules, incl. `A brochure is already attached`).
- Already in place: `Sheet` renders `job.error` and the start handler passes the error to `onError`. Check these still hold after the changes rather than rewriting them.
- Known trade-offs from `context/project-overview.md` (claim/rename window, separate `SELECT` after `UPDATE` in `StartJob`, lost connection mid-`AttachBrochure`, temp files before the claim, stale `body-*.tmp`) are marked "each goes into `NOTES.md`".
- Rejected suggestions available for `NOTES.md` are in `context/rejected-suggestions.md`.
- Out of scope: frontend tests, SSE, any API change.

- Decided at load: NOTES.md uses the `DetachBrochure`-on-any-claim-error suggestion and gets a Known trade-offs section; a failed poll shows the error and retries next tick, except 401/404 which stop polling; the create form is cleared once the sheet exists, even if the upload fails.

- UX change after the first browser check: the create form rejects 0-byte and >1048576-byte files in the browser with the server texts (`Brochure must be a PDF or a text file` / `Brochure is too large`) and creates no sheet; the type is still decided only by the server. A failed upload after create opens the new sheet and shows `Sheet created, but the brochure was not attached: <server text>` next to its Brochure field, not above the form; a draft without a brochure shows only the file input, no Start; a successful attach clears that error. Because of this, Testing step 4 needs a non-empty file of the wrong type (e.g. a renamed binary) instead of an empty one.

- After the Playwright check: while a sheet other than the open one is `running`, the list refreshes via `GET /api/jobs` about every 2s (same `setTimeout` chain, cleanup on change/unmount); it only updates existing entries (never adds or drops, skips the open sheet) and stops when no other sheet is running; errors retry silently, 401 stops. `.layout` gets `align-items: start` so list rows keep their 8px gap when the sheet panel is taller.

- After review: the sheet's file input is disabled while its upload is in flight (a second pick could only get 400 `A brochure is already attached`); `Sheet` is keyed by sheet id so that state does not carry over to another sheet. Left as is (L1): a list response requested before a local change (start or attach on sheet X) can land after the user has switched away from X and put X's list entry back to its older state, and if no other sheet is running the list poll then stops. Accepted because the window is one list request (tens of milliseconds) and it only affects how the list entry is displayed; a reload fixes it, and for a reverted start, Start also does (opening the sheet does not, it reuses the list entry, and a reverted attach gets 400 `A brochure is already attached` until a reload); comparing `updatedAt` would add code for that window. No shared polling hook either: the two chains are small and differ in what they update and when they stop.

- After code-scanner: the create button is disabled while its requests are in flight (`submitting`, early return, reset in `finally`), so a double click cannot create two sheets; the open-sheet poll removes its own error in the cleanup, so it does not stay after switching sheets. Other Lows left as is (global `error` not cleared on row click, no restart after 401/404, logout without `.catch`, initial list fetch without cleanup, `JSON.parse` on non-JSON bodies).

- For this phase's History entry (one line): code-scanner ran on every phase; for the phases where History has no record: sheets phase 1 had no findings, sheets phase 2 found the upload/start race (closed in brochure phase 2), brochure phase 2 found the stuck brochure slot that led to the rejected suggestion in NOTES.md.

- Layout pass after a browser check: the sign-in/register form gets 24px under the heading and 16px above the mode switch button (`.narrow form`), error paragraphs inside grid forms lose their default margins (`form .error`), so the form keeps an even 16px rhythm; `.layout` columns use `minmax(0, 1fr)` and long unbroken titles, addresses and file names wrap (`overflow-wrap: anywhere` on the sheet panel and the list row's text, not its status), so there is no horizontal scroll at 1160px or 390px (was 1155px/1549px of overflow).

### Testing (from spec)

1. In `web/`: `npx tsc --noEmit` and `npm run build` pass.
2. `docker compose up --build`, open http://localhost:8080. Create a sheet with a text brochure and start it: the checklist ticks about every 2s and ends `success`, the list entry updates too, no reload.
3. Start a second sheet while one is running: the 409 text is shown.
4. Create a sheet with an empty file: the error is shown and the sheet appears in the list. Open it, attach a valid `.txt` through the file input: `filename` is filled in and start works.
5. Delete the brochure from the volume mid-run: the page shows `Brochure is missing`.
6. The network tab shows polling stops after `success`/`failed`.

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
- Sheets Phase 2 - Start moves a draft to running: `Store.StartJob(id, userID, runID)` does a conditional `UPDATE ... WHERE status = 'draft'` and re-reads, so `runId` is chosen once even under concurrent starts; partial unique index `jobs_one_running_per_user` maps by `ConstraintName` to `board.ErrAlreadyRunning` -> 409 `Another sheet is already running`; non-draft sheets come back unchanged with 200; `mem` mirrors Postgres; `TestStartMarksTheJobSuccessful` rewritten to the spec; Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`). Known gap left for brochure phase 2: upload's whole-row `UpdateJob` can revert a concurrent start (spec now removes `UpdateJob` and adds `AND filename <> ''` to the start `UPDATE`)
- Brochure Phase 1 - Streamed upload, byte-based type, fixed path: upload streams via `r.MultipartReader()` (first `file` part wins) under `MaxBytesReader` (1 MiB + 64 KiB, `*http.MaxBytesError` -> 400 `Brochure is too large`, never 413, checked over real TCP); exact 1048576/1048577 limit; `board.BrochureType` decides PDF/text from bytes only (`headerAllows` removed); `board.BrochurePath` builds `{DATA_DIR}/{userID}/{jobID}/body` for upload and download; raw client filename from `Content-Disposition` is metadata only; new 400 `Brochure file name is not valid` for non-UTF-8 or >255-byte names (found by code-scanner review); check order is name, size, type, then `MkdirAll`/write, so rejected uploads leave `DATA_DIR` empty; `TestUploadKeepsTheClientFilename` rewritten to the spec. Left for brochure phase 2: tests for upload's 500 branches (rewritten by temp/claim/rename) and download's 404 on an empty `filename` (not yet in the phase 2 Testing list)
- Brochure Phase 2 - One brochure per sheet, safe download: early 400 `A brochure is already attached` before the body is read; upload writes `body-*.tmp`, claims the slot with one `UPDATE ... AND filename = '' RETURNING` (`board.ErrBrochureAttached`, called with `context.WithoutCancel`), then renames to `body`; a failed rename resets via `DetachBrochure` (`filename = $name AND status = 'draft'`) and gives 500; every rollback path removes the temp file; `UpdateJob` removed everywhere, so no code writes a whole sheet row (closes the lost update from sheets phase 2); `StartJob` gets `AND filename <> ''`, a still-`draft` result gives 400 `Attach a brochure first`; download sets `Content-Type` from `BrochureType` (fallback `application/octet-stream`), `nosniff`, `mime.FormatMediaType` disposition and `Last-Modified` from the file's mtime; client filename stored raw; `mem` mirrors Postgres; Postgres integration tests for attach/detach/start; race detector clean in `golang:1.23`; manual Docker check of both brochure phases passed. Rejected: a detach on any claim error (`filename` is not unique per request). Known trade-offs recorded in `context/project-overview.md` for `NOTES.md`
- Worker Phase 1 - Checklist runner: `internal/worker` with its own `Store` interface (`RunningJobs`, `SaveChecklist`, `FinishJob`, implemented by `*store.Store`, checked at compile time); `Run` polls every second, one goroutine per sheet under a mutex-guarded in-progress set (`release` deferred), `sync.WaitGroup` wait before return; no pause before the first step, `STEP_PAUSE` via `select` on `ctx.Done()` and a timer, brochure read with `os.ReadFile` before every step (a directory or missing file gives `failed` / `Brochure is missing`), `success` right after the third step, `success` at once on resume with every step `done`; every write `WHERE status = 'running' AND run_id`, step saves also `AND checklist = $prev` so a stale poll snapshot cannot redo or roll back a `done` step (L1 from code-scanner review); no row changed stops the sheet, DB errors are logged and retried next tick, no new write after cancel; 14 worker tests with a fake store mirroring Postgres, 100% statement coverage, mutation-checked; Postgres integration tests for the running poll, conditional writes incl. stale `prev`, and `ListJobs` after the `queryJobs` refactor; race detector clean in `golang:1.23`. Moved to worker phase 2: store integration tests must clean up their rows and run without the `worker` service (M1). Left as is: `FinishJob` accepts any status; `assertNoChange` ignores `updated_at`
- Worker Phase 2 - Worker binary, safe migration, Docker: `cmd/worker/main.go` is wiring only (env, 60s `waitStore` copy, `signal.NotifyContext`, `worker.New(...).Run`, store closed after `Run`); `stepPause` parses `STEP_PAUSE` before the DB connect (unset/empty `2s`, `0` allowed, unparsable or negative exits with an error naming `STEP_PAUSE`), table-driven test; `store.migrate` runs `schema.sql` in one transaction after `pg_advisory_xact_lock(migrateLockKey)` (pgx v5.7.4 sends a no-arg exec over the simple protocol, checked in source); `TestMigrateWaitsForTheLock` (deterministic) and `TestOpenMigratesAnEmptySchemaConcurrently` (fresh schema via `search_path`), both fail 5/5 without the lock (`pg_type_typname_nsp_index`); store integration tests delete every user they create with sessions and sheets (0 test users left); Dockerfile `RUN` and compose `worker` block word for word as README.md (diffed), both binaries in the image; manual Docker check passed (concurrent start x3 with no errors, restart, steps saved 2.003s/2.010s apart by `updatedAt`, kill/start resumes with the same `runId`, deleted `body` gives `Brochure is missing`, bad `STEP_PAUSE` exits 1, second start 409); race detector clean in `golang:1.23`; code-scanner: no High/Medium. Left as is (Low): `NotifyContext` after `waitStore` (default SIGTERM exit, same as API), a second signal during shutdown is swallowed until `Run` returns, `TestStepPauseDefaultsWhenUnset` overlaps the empty case; `CREATE UNIQUE INDEX IF NOT EXISTS` briefly takes a SHARE lock on `jobs` at start
