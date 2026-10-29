# Current Feature: Brochure Phase 1 - Streamed upload, byte-based type, fixed path

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] Shared helper in `internal/board` (e.g. `BrochurePath(dataDir, userID, jobID)`) builds `{DATA_DIR}/{userID}/{jobID}/body`; upload and download use it (the worker will later); client input never takes part in a path
- [x] Download reads from the new path (headers stay as they are until phase 2)
- [x] Upload reads the body with `r.MultipartReader()`, not `ParseMultipartForm`
- [x] Missing `file` part, or one without a filename -> 400 `Choose a brochure file`; broken multipart -> 400 `Brochure upload was incomplete`
- [x] Exact limit: read at most 1048577 bytes; 1048576 pass, 1048577 -> 400 `Brochure is too large`
- [x] `MaxBytesReader` on the whole body with envelope headroom (e.g. +64 KiB); `*http.MaxBytesError` -> 400 `Brochure is too large`, never 413
- [x] Pure function in `internal/board` detects the type from bytes: `%PDF-` -> `application/pdf`; valid UTF-8, no NUL, no `%PDF-` prefix -> `text/plain; charset=utf-8`; empty or anything else -> 400 `Brochure must be a PDF or a text file`
- [x] `headerAllows` removed
- [x] Type and size validated before anything is written under `DATA_DIR`
- [x] `filename` is read raw from the part's `Content-Disposition` via `mime.ParseMediaType`, not `part.FileName()` (which strips `../`); `../../escaped.txt` is stored in `filename` as is
- [x] "Before anything is written" includes directories: `os.MkdirAll` runs only after type and size pass, so a rejected upload leaves not even an empty folder in `DATA_DIR`
- [x] `TestUploadChecksTheOwnerBeforeReadingTheBody` (sheets phase 1) still passes: `MaxBytesReader` and `MultipartReader` are set up only after `s.load`
- [x] A filename that is not valid UTF-8 or longer than 255 bytes gives 400 `Brochure file name is not valid`, before `MkdirAll` and any write (added after the code-scanner review)
- [x] `TestUploadKeepsTheClientFilename` rewritten (same name): bytes at `{dir}/{userID}/{jobID}/body`, `filename` equals the client name, `../../escaped.txt` creates nothing outside `{dir}/{userID}/{jobID}`

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

- `filename` stores the raw client name (e.g. `../../escaped.txt`) as metadata only. `multipart.Part.FileName()` strips the directory, so read the name from the part's `Content-Disposition` with `mime.ParseMediaType`, like `filenameFromHeader` does today (it takes a `*multipart.FileHeader`; adapt it to a `*multipart.Part`).
- Today an empty filename gives `Brochure must be a PDF or a text file`; the spec changes it to `Choose a brochure file`.
- `*http.MaxBytesError` can surface from `NextPart()` as well as from reading the part; check with `errors.As` in both places before mapping to `Brochure upload was incomplete`.
- Rejected cases must leave `DATA_DIR` empty: no `MkdirAll` before validation passes.
- This phase still saves through `UpdateJob`; phase 2 replaces it with the atomic attach (temp file, conditional claim, rename) and adds `A brochure is already attached`. In this phase a second upload still overwrites `body`.
- Agreed defaults: take the first part named `file`, skip earlier parts (they count toward `MaxBytesReader`), don't read later ones; size is checked before type (an oversized file of a wrong type gets `Brochure is too large`); a whitespace-only name counts as missing (`Choose a brochure file`); download keeps `http.ServeContent` keyed on `filepath.Base(job.Filename)` until phase 2.
- Left uncovered on purpose, covered in brochure phase 2 (which rewrites this code): upload's 500 `Could not store the brochure` branches (`MkdirAll`, write, `UpdateJob`) are replaced by temp/claim/rename, whose 500 test is in the phase 2 spec (Testing step 1); download's 404 for an empty `filename` is not in the phase 2 Testing list yet, add it there with the download header tests.
- Out of scope (phase 2): one brochure per sheet, download `Content-Type` / `nosniff` / `mime.FormatMediaType` disposition.
- Constraints: Go 1.23 APIs only, no new dependencies, `go test ./...` without Postgres.

### Testing (from spec)

1. `go test ./internal/board ./internal/httpapi -run 'Brochure|Upload' -v`: in new tests, a table covers PDF, text, empty, NUL, invalid UTF-8, `%PDF-` inside a `.txt` name, PDF bytes sent as `text/plain`, 1048576 vs 1048577 bytes. Every rejected case leaves `DATA_DIR` empty.
2. The rewritten `TestUploadKeepsTheClientFilename` passes.
3. A new test uploads PDF and text bytes, then `GET /api/jobs/{id}/file` returns 200 with exactly the same bytes, read from `{dir}/{userID}/{jobID}/body`.

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
- Sheets Phase 2 - Start moves a draft to running: `Store.StartJob(id, userID, runID)` does a conditional `UPDATE ... WHERE status = 'draft'` and re-reads, so `runId` is chosen once even under concurrent starts; partial unique index `jobs_one_running_per_user` maps by `ConstraintName` to `board.ErrAlreadyRunning` -> 409 `Another sheet is already running`; non-draft sheets come back unchanged with 200; `mem` mirrors Postgres; `TestStartMarksTheJobSuccessful` rewritten to the spec; Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`). Known gap left for brochure phase 2: upload's whole-row `UpdateJob` can revert a concurrent start (spec now removes `UpdateJob` and adds `AND filename <> ''` to the start `UPDATE`)
