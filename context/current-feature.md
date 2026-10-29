# Current Feature: Brochure Phase 2 - One brochure per sheet, safe download

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] Early check: after ownership and before the body is read, a sheet with a non-empty `filename` gets 400 `A brochure is already attached` (shortcut only, the conditional `UPDATE` is the guarantee)
- [x] Upload writes the validated bytes to a temp file in the target directory (`os.CreateTemp(dir, "body-*.tmp")`)
- [x] Claim the slot with `UPDATE ... SET filename = $name WHERE id = $id AND user_id = $user AND filename = ''`; no row changed maps to a new sentinel in `internal/board`
- [x] No row changed: remove the temp file, 400 `A brochure is already attached`
- [x] Row changed: rename the temp file to `body`
- [x] Rename failed: reset `filename = ''` `WHERE id = $id AND filename = $name`, remove the temp file, 500 `Could not store the brochure`
- [x] Download sets `Content-Type` explicitly from the stored bytes via `board.BrochureType`: `application/pdf` or `text/plain; charset=utf-8`
- [x] Download sets `X-Content-Type-Options: nosniff`
- [x] Download builds `Content-Disposition` with `mime.FormatMediaType("attachment", map[string]string{"filename": job.Filename})`
- [x] `UpdateJob` removed from `board.Store`, `store.Store` and `mem`; no code writes a whole sheet row any more (closes the lost update from sheets phase 2)
- [x] `TestStartJobLeavesAFinishedSheetUnchanged` sets `success`/`failed` through a test-only helper in `package store` that runs the `UPDATE` directly
- [x] `StartJob`'s `UPDATE` gets `AND filename <> ''`; when `StartJob` returns a sheet that is still `draft`, the handler answers 400 `Attach a brochure first`
- [x] `mem` implements the new store methods with the same semantics as Postgres (conditional claim, conditional reset, `filename <> ''` on start, same sentinels)
- [x] Test: an upload that overlaps a start leaves the `running` sheet's `status` and `runId` unchanged (Testing step 2 interleaving: A blocks in its body, B completes, start gives `runId=R`, A is released and gets 400 `A brochure is already attached`; sheet stays `running`, `runId=R`, B's `filename`)
- [x] Left from brochure phase 1: `filename` is stored exactly as the client sent it (A3, README "keep the original filename"); `TrimSpace` is used only to check the name is not empty; the 255-byte limit counts the raw name. Test: `" brochure.txt "` is stored with its spaces
- [x] Left from brochure phase 1: update the "Filename check" line in Assumptions of `context/project-overview.md` (drop "after trimming spaces")
- [x] Left from brochure phase 1: add `Brochure file name is not valid` to the "Error texts not fixed by README" line in `context/project-overview.md`
- [x] Manual check on a real server for both brochure phases, at the end of this phase: `docker compose up --build -d`, then curl: exactly 1048576 bytes -> 200, 1048577 -> 400; empty file and a file with NUL -> 400; `../../escaped.txt` keeps `filename` as is and `find / -name escaped.txt` in the container finds nothing; `ls -R /data` shows only `{userID}/{jobID}/body`; a second upload -> 400 `A brochure is already attached`; download returns the same bytes with the three headers. Report as a table (request, expected, actual). Finish with `docker compose down -v`

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

- Route contract: `GET /api/jobs/{id}/file` returns 200 with the bytes, or 401 / 404 `Not found` (another account's sheet, no brochure, file missing on disk).
- The temp file lives in the target directory, so the rename never crosses filesystems.
- `http.ServeContent` keeps a `Content-Type` that was set before it is called.
- Carried over from brochure phase 1: test upload's 500 branch (here: the failed rename) and download's 404 on an empty `filename`.
- Phase 1 rules stay: validation (name, size, type) before anything is written under `DATA_DIR`; `TestUploadChecksTheOwnerBeforeReadingTheBody` keeps passing, so the early `filename` check also runs before the body is touched.
- Known trade-off (goes into `NOTES.md` later): between the claim in the database and the rename to `body` there is a short window where `filename` is already set but the file does not exist yet. A start that lands exactly then lets the worker find no file and fail the sheet with `Brochure is missing`. A normal user cannot hit this, because the upload response arrives only after the rename; deliberately not made more complex.
- Agreed defaults (confirmed after load):
  - The reset is `UPDATE ... SET filename = '' WHERE id = $id AND filename = $name AND status = 'draft'`: a sheet started between claim and failed rename keeps its `filename`, and the worker later fails it with `Brochure is missing`.
  - Download reads the whole `body` (at most 1 MiB) for `BrochureType` and serves it via `http.ServeContent` over a `bytes.Reader`; bytes that fail `BrochureType` are served as `application/octet-stream` (still `nosniff` and `attachment`), no new error text.
  - Store methods: `AttachBrochure(ctx, id, userID, name) (Job, error)` returns the sheet or `board.ErrBrochureAttached`; `DetachBrochure(ctx, id, name) error` is the conditional reset.
  - Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`) cover the claim, the reset and `filename <> ''` in `StartJob`.
- After the code-scanner review: `AttachBrochure` is one `UPDATE ... RETURNING` (no read after the commit, the fallback `JobForUser` runs only when no row changed) and is called with `context.WithoutCancel`; `DetachBrochure` stays only on the failed-rename path (a detach on any claim error was rejected: `filename` is not unique per request); download's `Last-Modified` comes from the file's `ModTime()`, not `job.UpdatedAt`. Trade-offs recorded in `context/project-overview.md` "Known trade-offs".
- Docker is available on this machine: the manual check runs against `docker compose up --build -d`, not `go run`.
- `mime.FormatMediaType("attachment", {"filename": ...})` never returns `""` for a valid UTF-8 name (checked in Go 1.23 source): non-ASCII and control bytes are encoded as `filename*=utf-8''...`.
- Constraints: Go 1.23 APIs only, no new dependencies, `go test ./...` without Postgres; Postgres tests skip without `DATABASE_URL`.

### Testing (from spec)

1. `go test ./internal/httpapi -run 'Attach|Download' -v`: a second upload gives 400 with the exact text; 5 concurrent uploads give one 200, the rest 400, `body` holds the winner's bytes and no `*.tmp` is left; a directory pre-created at the `body` path gives 500, `filename` back to `""`, no temp file.
2. An upload that overlaps a start does not change `status` or `runId` of the `running` sheet. Interleaving: upload A on a draft without a brochure passes the early check and blocks in its body (a reader that waits on a channel); upload B completes; start returns `running` with `runId=R`; then A's body is released. A gets 400 `A brochure is already attached`, and the sheet is still `running` with `runId=R` and B's `filename`.
3. Download tests check the three headers for PDF and text; a filename with quotes and non-ASCII parses back via `mime.ParseMediaType`; a sheet without a brochure (empty `filename`) gives 404 `Not found`.
4. Docker: `curl -sD - -b jar localhost:8080/api/jobs/$ID/file -o /dev/null` shows the three headers (part of the manual check goal above).

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
- Sheets Phase 2 - Start moves a draft to running: `Store.StartJob(id, userID, runID)` does a conditional `UPDATE ... WHERE status = 'draft'` and re-reads, so `runId` is chosen once even under concurrent starts; partial unique index `jobs_one_running_per_user` maps by `ConstraintName` to `board.ErrAlreadyRunning` -> 409 `Another sheet is already running`; non-draft sheets come back unchanged with 200; `mem` mirrors Postgres; `TestStartMarksTheJobSuccessful` rewritten to the spec; Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`). Known gap left for brochure phase 2: upload's whole-row `UpdateJob` can revert a concurrent start (spec now removes `UpdateJob` and adds `AND filename <> ''` to the start `UPDATE`)
- Brochure Phase 1 - Streamed upload, byte-based type, fixed path: upload streams via `r.MultipartReader()` (first `file` part wins) under `MaxBytesReader` (1 MiB + 64 KiB, `*http.MaxBytesError` -> 400 `Brochure is too large`, never 413, checked over real TCP); exact 1048576/1048577 limit; `board.BrochureType` decides PDF/text from bytes only (`headerAllows` removed); `board.BrochurePath` builds `{DATA_DIR}/{userID}/{jobID}/body` for upload and download; raw client filename from `Content-Disposition` is metadata only; new 400 `Brochure file name is not valid` for non-UTF-8 or >255-byte names (found by code-scanner review); check order is name, size, type, then `MkdirAll`/write, so rejected uploads leave `DATA_DIR` empty; `TestUploadKeepsTheClientFilename` rewritten to the spec. Left for brochure phase 2: tests for upload's 500 branches (rewritten by temp/claim/rename) and download's 404 on an empty `filename` (not yet in the phase 2 Testing list)
