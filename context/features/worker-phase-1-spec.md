# Worker Phase 1 - Checklist runner

## Overview

`internal/worker` walks `running` sheets through the checklist with conditional writes, so it is safe to kill and restart. It is tested with a fake store, with no Postgres.

## Requirements

- `Run(ctx)` polls `running` sheets (a new store method, e.g. every second) and starts one goroutine per sheet. A mutex-guarded set tracks sheets in progress and drops a sheet when its goroutine ends.
- Per sheet: no pause before the first step. For each step: check the brochure at `board.BrochurePath` can be read, mark the first `pending` step `done`, save, then wait `STEP_PAUSE`. A `done` step is never redone. After the third step is saved, set `success` in a separate save right away.
- Every write is `WHERE id = $id AND status = 'running' AND run_id = $runId` and reports whether a row changed. When no row changed, the worker stops processing that sheet. A DB error while polling or saving never stops the worker: log it, end that sheet's goroutine (it leaves the in-progress set), and retry on the next tick. The sheet stays `running` and resumes from its first `pending` step.
- On context cancel (SIGTERM), the worker returns without writing anything, so the sheet stays `running`. On the next start it continues from the first `pending` step. The worker never changes `runId`.
- Brochure cannot be read: `status = 'failed'`, `error` exactly `Brochure is missing` (conditional too).
- Tests in `internal/worker` use a fake store and `t.TempDir()`.

## Files

`internal/worker/worker.go`, `internal/worker/worker_test.go`, `internal/store/store.go` (new methods)

## Notes

- The worker depends on a small interface declared in `internal/worker`; `*store.Store` implements it. Checklist JSON stays in the store.
- The pause uses `select` on `ctx.Done()` and a timer, not `time.Sleep`.

## Testing

1. `go test ./internal/worker -v` (pause of a few ms): happy path gives 3 step saves then `success`; missing file gives `failed` / `Brochure is missing`; step 1 already `done` gives only 2 step saves with the same `runId`; cancel during the pause gives no write after it; a changed `runId` stops the worker; two polls never start a second goroutine for one sheet; a fake store that fails the poll once, then one step save, keeps `Run` alive, and the sheet still ends `success` with each step `done` once.
2. `go test ./...` and `go vet ./...` pass.

## References

- `go doc context.Context`, `go doc time.NewTimer`
