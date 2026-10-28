<!-- BEGIN:ai-harness-rules -->
# This is NOT the library version you know

The framework and library versions in this repo may have **breaking changes** since your training cutoff. APIs, conventions, config files and file structure can all differ from what you remember.

Before writing code against a framework or library:

- Check the installed version in `go.mod` / `go.sum` (Go) and `web/package.json` + `web/package-lock.json` (UI).
- Read the installed docs first when they ship in the package (e.g. `go doc <pkg>.<Symbol>` for Go), or fetch that exact version's docs (Context7 MCP or a targeted web search) before relying on remembered APIs.
- Heed deprecation notices in build, lint and test output.
- When a pattern in this repo looks unusual, do NOT assume it is wrong. Verify against the installed version first.

Stack notes for this repo:

- **Go 1.23** (`go.mod`: `go 1.23.0`; Docker `golang:1.23-alpine`). Routing uses the stdlib `http.ServeMux` method+path patterns (`"POST /api/jobs/{id}/start"`) and `r.PathValue("id")`, available since Go 1.22. No router library.
- **pgx v5.7.4** used through `database/sql` (`_ "github.com/jackc/pgx/v5/stdlib"`, driver name `"pgx"`), not the native `pgx.Conn`/`pgxpool` API. Errors are inspected via `*pgconn.PgError`.
- **golang.org/x/crypto v0.36.0** for `bcrypt`.
- **Postgres 16** (`postgres:16-alpine` in compose).
- **UI**: React 18.3.1, Vite 5.4.21, `@vitejs/plugin-react` 4.7.0, TypeScript 5.9.3 (strict). Node 22 in the Docker build stage. No router, no state or data library.
<!-- END:ai-harness-rules -->
