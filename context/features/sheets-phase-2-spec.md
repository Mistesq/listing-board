# Sheets Phase 2 - Start moves a draft to running

## Overview

`POST /api/jobs/{id}/start` only moves a draft to `running` and chooses `runId` once. The worker (later phases) finishes the sheet.

## Requirements

- Schema: `CREATE UNIQUE INDEX IF NOT EXISTS` on `jobs (user_id) WHERE status = 'running'`. A violation, recognized by `PgError.ConstraintName`, maps to a new sentinel in `internal/board` (not `ErrDuplicate`). The handler returns 409 `Another sheet is already running`.
- A store method does `UPDATE jobs SET status = 'running', run_id = $runId, updated_at = $now WHERE id = $id AND user_id = $user AND status = 'draft'`. When no row changed, it re-reads and returns the current sheet, so `runId` is chosen exactly once, even under concurrent requests.
- Handler: 400 `Attach a brochure first` when `filename` is empty. Otherwise it calls the conditional start with `board.NewID()` and returns the resulting sheet. `running`, `success` and `failed` come back unchanged with 200. The handler never reads the disk, never sets `success` or `failed`, and leaves steps `pending`.
- Rewrite `TestStartMarksTheJobSuccessful` (keep the name): status `running`, non-empty `runId`, all three steps `pending`, a second start returns the same `runId`.

## Route Contract

`POST /api/jobs/{id}/start` returns 200 with the sheet, or 401 / 404 / 400 `Attach a brochure first` / 409 `Another sheet is already running` / 500 `Could not start the sheet`.

Check order: 401, 404, 400 without a brochure, then status logic (non-draft gives 200 unchanged), then 409. The conditional `UPDATE` gives this order by itself: a non-draft sheet changes no row and never touches the index.

## Notes

- `mem.StartJob` mirrors Postgres: only `draft` changes; another `running` sheet of the same user gives the same sentinel.

## Testing

1. `go test ./internal/httpapi -run Start -v`: the rewritten test passes. New tests: a second sheet of the same account gives 409 with the exact text; no brochure gives 400; `failed`/`success` (set through `mem`) come back unchanged; 10 concurrent starts on one draft all return the same `runId`.
2. After phase 6, with Docker: start two sheets of one account; the second `curl -b jar -X POST localhost:8080/api/jobs/$ID2/start` gives 409.

## References

- `go doc github.com/jackc/pgx/v5/pgconn.PgError`
- https://www.postgresql.org/docs/16/indexes-partial.html
