# Current Feature

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

Not Started

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
- Sheets Phase 1 - Ownership on every id route: `Store.JobForUser(id, userID)` replaces `Job(id)`; all four id routes return 404 `Not found` for another account's sheet (same as an unknown id), ownership checked before the upload body is read; `mem` mirrors Postgres; ownership tests incl. owner control check and a read-recording upload body; verified on real Postgres via curl
- Sheets Phase 2 - Start moves a draft to running: `Store.StartJob(id, userID, runID)` does a conditional `UPDATE ... WHERE status = 'draft'` and re-reads, so `runId` is chosen once even under concurrent starts; partial unique index `jobs_one_running_per_user` maps by `ConstraintName` to `board.ErrAlreadyRunning` -> 409 `Another sheet is already running`; non-draft sheets come back unchanged with 200; `mem` mirrors Postgres; `TestStartMarksTheJobSuccessful` rewritten to the spec; Postgres integration tests in `internal/store/store_test.go` (skip without `DATABASE_URL`). Known gap left for brochure phase 2: upload's whole-row `UpdateJob` can revert a concurrent start (spec now removes `UpdateJob` and adds `AND filename <> ''` to the start `UPDATE`)
- Brochure Phase 1 - Streamed upload, byte-based type, fixed path: upload streams via `r.MultipartReader()` (first `file` part wins) under `MaxBytesReader` (1 MiB + 64 KiB, `*http.MaxBytesError` -> 400 `Brochure is too large`, never 413, checked over real TCP); exact 1048576/1048577 limit; `board.BrochureType` decides PDF/text from bytes only (`headerAllows` removed); `board.BrochurePath` builds `{DATA_DIR}/{userID}/{jobID}/body` for upload and download; raw client filename from `Content-Disposition` is metadata only; new 400 `Brochure file name is not valid` for non-UTF-8 or >255-byte names (found by code-scanner review); check order is name, size, type, then `MkdirAll`/write, so rejected uploads leave `DATA_DIR` empty; `TestUploadKeepsTheClientFilename` rewritten to the spec. Left for brochure phase 2: tests for upload's 500 branches (rewritten by temp/claim/rename) and download's 404 on an empty `filename` (not yet in the phase 2 Testing list)
