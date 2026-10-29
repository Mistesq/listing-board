# Sheets Phase 1 - Ownership on every id route

## Overview

Load a sheet by id and owner, so another account's sheet is a 404 on every id route. This phase also sets the rule for the `mem` test store that later phases follow.

## Requirements

- Add a store method that loads a sheet by id **and** user id (e.g. `JobForUser(ctx, id, userID)`), returning `board.ErrNotFound` when either does not match. `Server.load` takes the current user and uses it for `GET /api/jobs/{id}`, `POST /api/jobs/{id}/file`, `POST /api/jobs/{id}/start` and `GET /api/jobs/{id}/file`.
- Check order on every id route: 401 `Sign in`, then 404 `Not found`, then the route's own checks.
- Upload checks ownership before it reads the request body.
- The `mem` store implements every new store method with the same semantics as Postgres: conditional updates, one running sheet per account, one brochure per sheet, the same sentinels. Every later phase extends `mem` this way.

## Files

`internal/board/board.go`, `internal/store/store.go`, `internal/httpapi/server.go`, `internal/httpapi/server_test.go`

## Notes

- Do not change the bodies of `TestListHidesOtherUsersJobs` and `TestStartRequiresASession`. Shared helpers and `mem` may change.
- Drop `Job(ctx, id)` from the interface if nothing uses it anymore.

## Testing

1. `go test ./internal/httpapi -run Owner -v`: a table-driven test calls all four id routes with another account's cookie; every case is 404 with `{"error":"Not found"}`. The same table with no cookie gives 401.
2. Upload to another account's sheet with a body reader that records reads: 404, and the body was never read.
3. `go test ./...`, `go vet ./...` and `gofmt -l .` pass and print nothing unexpected.

## References

- `go doc net/http.Request.PathValue`
