# Current Feature: Sheets Phase 1 - Ownership on every id route

<!-- H1 gets the feature name when active, e.g. "# Current Feature: Add Navbar" -->

## Status

<!-- Not Started | In Progress | Complete -->

In Progress

## Goals

<!-- Checklist of what success looks like, filled by `/feature load`. Each goal is checked off when done, so the list doubles as plan vs actual. -->

- [x] Add `JobForUser(ctx, id, userID)` to `board.Store`; Postgres implementation filters by `id` **and** `user_id`, returns `board.ErrNotFound` when either does not match
- [x] `Server.load` takes the current user and uses `JobForUser` for `GET /api/jobs/{id}`, `POST /api/jobs/{id}/file`, `POST /api/jobs/{id}/start`, `GET /api/jobs/{id}/file`
- [x] Check order on every id route: 401 `Sign in` -> 404 `Not found` -> the route's own checks
- [x] Upload checks ownership before the request body is read
- [x] `mem` store implements `JobForUser` with the same semantics as Postgres (same sentinel); this is the parity rule every later phase follows
- [x] Drop `Job(ctx, id)` from the interface, `store.Store` and `mem` if nothing uses it anymore
- [x] Table-driven ownership test (name matches `-run Owner`): all four id routes with another account's cookie -> 404 `{"error":"Not found"}`; same table without a cookie -> 401
- [x] Control check in the same test: the owner gets 200 on `GET /api/jobs/{id}` and `GET /api/jobs/{id}/file`, proving the sheet and its brochure exist and the 404 comes from the ownership check
- [x] Upload to another account's sheet with a read-recording body: 404, body never read

## Notes

<!-- Additional context, constraints, API contract, out of scope. -->

- Files: `internal/board/board.go`, `internal/store/store.go`, `internal/httpapi/server.go`, `internal/httpapi/server_test.go`.
- Do not change the bodies of `TestListHidesOtherUsersJobs` and `TestStartRequiresASession`. Shared helpers and `mem` may change.
- A sheet owned by another account must be indistinguishable from an unknown id: same status, same body.
- In the ownership test, give the other account's sheet a brochure, so the download/start 404 comes from ownership and not from "no file" / "Attach a brochure first".
- Decisions: `load` takes the user id explicitly (`s.load(w, r, user.ID)`); `UpdateJob` stays unconditional in this phase; the other account's sheet has a brochure in the ownership test; the owner control check covers only the two GET routes (upload/start would change the sheet).
- Out of scope (later phases): start transition and 409 (sheets-phase-2), streamed upload, byte type, path helper (brochure-phase-1), conditional brochure claim (brochure-phase-2). `UpdateJob` stays as is for now; later phases replace its uses with conditional updates.
- Testing steps (review checks against them):
  1. `go test ./internal/httpapi -run Owner -v`: a table-driven test calls all four id routes with another account's cookie; every case is 404 with `{"error":"Not found"}`. The same table with no cookie gives 401.
  2. Upload to another account's sheet with a body reader that records reads: 404, and the body was never read.
  3. `go test ./...`, `go vet ./...` and `gofmt -l .` pass and print nothing unexpected.

## History

<!-- Completed features, append only, oldest to newest. -->

- Project setup and boilerplate cleanup
