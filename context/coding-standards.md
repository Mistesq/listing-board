# Coding Standards

Rules recorded from the existing code. Follow them unless the README spec requires otherwise.

## Go: Layering

- `cmd/<binary>/main.go`: env, wiring, retry loops, signal handling. No business logic.
- `internal/board`: domain types, constructors (`NewChecklist`, `NewID`), sentinel errors and the `Store` interface. No I/O besides `crypto/rand`.
- `internal/store`: SQL only. Implements `board.Store`. Maps driver errors to `board` sentinels (`sql.ErrNoRows` -> `ErrNotFound`, pg `23505` -> `ErrDuplicate`).
- `internal/httpapi`: handlers are methods on `*Server` and depend on the `board.Store` interface, never on `*store.Store`.
- Exception: there is no service layer. Handlers currently validate, change state and call the store directly. New shared logic (e.g. state transitions used by both API and worker) belongs in `internal/board` or a new `internal/` package, not duplicated in handlers.

## Go: HTTP Handlers

- Register routes in `Server.routes()` with method+path patterns; read params with `r.PathValue`.
- Order inside a handler: `s.current(w, r)` (401) -> `s.load(w, r)` (404) -> decode/validate -> store call -> `writeJSON`. Helpers write the error response and return `ok=false`; the handler just returns.
- JSON bodies go through `decode()` (1 MiB `MaxBytesReader`). Validate manually: `strings.TrimSpace`, emptiness and length checks. Bad input is 400.
- Errors: `writeErr(w, status, message)` -> `{"error": "..."}`. Messages are short user-facing sentences ("Attach a brochure first", "Not found", "Sign in"). Never return `err.Error()` or internal details.
- Status codes: 400 invalid input, 401 no/expired session, 404 unknown sheet **and** a sheet owned by someone else, 409 conflict (duplicate email; this account already has a different sheet `running`), 500 store/disk failure with a generic message.
- Each kind of unique violation gets its own sentinel in `internal/board`. The "one running sheet per account" partial unique index maps to its own sentinel, not `ErrDuplicate`. The store tells them apart by `PgError.ConstraintName`.
- Check sentinels with `errors.Is`.

## Go: Data

- Schema lives in `internal/store/schema.sql` and must stay idempotent (`CREATE TABLE IF NOT EXISTS`, `ADD COLUMN IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`). It runs on every start of both the API and the worker, so it must be safe when both run it at the same time. `IF NOT EXISTS` alone can still race in Postgres; serialize the migration (e.g. `pg_advisory_xact_lock` in one transaction).
- Queries use `$n` placeholders and `...Context` methods. Job columns are listed once in `jobSelect` and scanned by `scanJob`; extend both together.
- Times are stored and set as UTC (`time.Now().UTC()`).
- The checklist is a JSON text column; (un)marshal in the store, never in handlers.
- Use unique constraints or conditional `UPDATE ... WHERE` for anything that must be atomic (e.g. state transitions shared by the API and the worker), not check-then-write.

## Go: Style

- `gofmt` formatting. Standard Go naming: exported `PascalCase`, unexported `camelCase`, short receiver names (`s`, `m`).
- Status values are string literals (`"draft"`, `"running"`, `"success"`, `"failed"`); step states are `"pending"`/`"done"`.
- JSON field names are camelCase via struct tags; hide internal fields with `json:"-"`.
- Doc comments are sparse; add one only where intent is not obvious.

## Go: Tests

- `go test ./...` never needs Postgres or network. A test that does need a database calls `t.Skip` when `DATABASE_URL` is unset.
- Handler tests use the in-memory `mem` store in `internal/httpapi/server_test.go`. Every new `Store` method gets a `mem` implementation with the same semantics as Postgres: conditional updates, one `running` sheet per account, one brochure per sheet, the same sentinel errors.
- Use table-driven tests with `t.Run` for input matrices (file types, size limits, ownership on every id route).
- Helpers call `t.Helper()`.

## Security

- Passwords: bcrypt (`DefaultCost`). Sessions: random token in an `HttpOnly`, `SameSite=Lax` cookie `lb_session`, 24h.
- Every route that takes a sheet id must verify the sheet belongs to the caller and return 404 otherwise.
- Never build file paths from client input; filenames are metadata only.
- Config comes from env (`DATABASE_URL`, `DATA_DIR`, `PORT`, `WEB_DIR`). No secrets in code.

## Web (`web/`)

- React 18 function components with hooks, TypeScript strict. Everything is in `src/App.tsx` (`App`, `Auth`, `Create`, `Sheet`); split only when a file grows unwieldy.
- Types are local (`type Job = {...}`) and mirror the API JSON.
- All requests go through the `api<T>(path, init)` helper (`credentials: "include"`, throws `Error` with the server's `error` message).
- State is local `useState`; errors are shown via an `error` string and `<p className="error">`.
- Style: double quotes, semicolons, trailing commas. Plain CSS classes in `src/styles.css`.

## File Organization

```text
cmd/<binary>/main.go     one directory per binary (api, worker)
internal/board/          domain types + Store interface
internal/store/          Postgres implementation + schema.sql
internal/httpapi/        HTTP server + server_test.go
web/src/                 React UI
```

## Code Quality

- No commented-out code, no unused imports or variables (`go vet` must pass).
- Keep functions under ~50 lines where possible.
