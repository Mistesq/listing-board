# Worker Phase 2 - Worker binary, safe migration, Docker

## Overview

Ship `cmd/worker` in the same image next to the API, with a schema migration that both processes can run at the same moment.

## Requirements

- `cmd/worker/main.go` reads env, retries `store.Open` for 60 seconds, and runs `worker.Run` under `signal.NotifyContext(SIGINT, SIGTERM)`. `STEP_PAUSE`: unset means `2s`, `0` is allowed, and an unparsable or negative value makes the worker exit at startup with a clear error.
- `store.migrate` runs `schema.sql` in one transaction that first takes `SELECT pg_advisory_xact_lock(<fixed key>)`, so API and worker can start at the same time.
- Dockerfile build stage: `RUN CGO_ENABLED=0 go build -o /app/api ./cmd/api && CGO_ENABLED=0 go build -o /app/worker ./cmd/worker`, exactly as in README.md. The final stage copies both binaries.
- `docker-compose.yml` gets the `worker` service exactly as in README.md (`command: ["/app/worker"]`, `STEP_PAUSE: 2s`, `boarddata:/data`, `restart: "no"`).
- The Postgres integration tests in `internal/store/store_test.go` clean up after themselves: every user a test creates is registered with `t.Cleanup`, which deletes that user's sessions, sheets and the user row. Today they leave `running` sheets without a brochure in the shared database, and a live worker would fail them with `Brochure is missing` while parallel tests still read them.

## Notes

- `main.go` stays wiring only: env, retry, signals. All logic lives in `internal/worker`.
- `schema.sql` is multi-statement; check that `tx.ExecContext` with no args still runs it through pgx stdlib.

## Environment Variables

`DATABASE_URL`, `DATA_DIR`, `STEP_PAUSE`

## Testing

1. `docker compose up --build`: api and worker start with no schema errors. `docker compose restart api worker` also gives no errors.
2. Use curl with a cookie jar (`-c jar -b jar`): register, create, upload a `.txt`, start, then `GET /api/jobs/$ID` each second. Steps turn `done` about 2s apart, then `success`.
3. Mid-run, `docker compose kill worker`: the sheet stays `running`. `docker compose start worker` resumes from the first pending step with the same `runId`.
4. Run `docker compose exec api rm /data/$USER_ID/$ID/body` during a run (`USER_ID` from `GET /api/me`): `failed`, `error` = `Brochure is missing`.
5. `docker compose run -e STEP_PAUSE=bogus worker` and `-e STEP_PAUSE=-1s` both exit at startup with a clear error.
6. Store integration tests run with only Postgres up, without the `worker` service: `docker compose up -d postgres` (stop `worker` first if it runs), then `DATABASE_URL=postgres://board:board@localhost:54329/board?sslmode=disable go test ./internal/store -count=1 -v`. Afterwards no test user is left: `docker compose exec postgres psql -U board -d board -c "SELECT count(*) FROM users WHERE email LIKE 'test-%@example.com'"` gives `0`.

## References

- `go doc os/signal.NotifyContext`, `go doc time.ParseDuration`
- https://www.postgresql.org/docs/16/functions-admin.html#FUNCTIONS-ADVISORY-LOCKS
