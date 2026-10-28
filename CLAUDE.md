@AGENTS.md

# Listing board

A board for real-estate listing sheets: a Go API with Postgres, a background worker that walks each sheet through a checklist, and a React page that shows progress. `README.md` is the spec; where code disagrees with it, the code is wrong.

## Hard constraints

- README.md is the spec. Where the code disagrees with it, change the code. Error texts, labels and status codes must match README.md exactly.
- Go 1.23 only. Do not use APIs added in Go 1.24 or later (t.Context, os.Root, strings.Lines, testing.B.Loop and similar). Do not change the go line in go.mod and do not add a toolchain directive.
- No new Go or npm dependencies without asking me first.
- go test ./... must pass without Postgres. Any test that needs a database skips when DATABASE_URL is unset.
- Never open a pull request and never run gh pr create. Push only to origin.
- TestStartMarksTheJobSuccessful and TestUploadKeepsTheClientFilename: keep their names, rewrite their bodies to the spec. Never delete, skip or weaken them.
- TestListHidesOtherUsersJobs and TestStartRequiresASession: do not change their bodies. Shared helpers and the mem store may change.
- docker-compose.yml ships with STEP_PAUSE set to 2s. The worker service block matches README.md exactly.
- Keep every API route from README.md. JSON errors are {"error":"..."}.
- When I reject one of your suggestions, append one line to context/rejected-suggestions.md: what you suggested, why I rejected it, how I checked the result.

## Context Files

Read the following to get the full context of the project:

- @context/project-overview.md
- @context/coding-standards.md
- @context/ai-interaction.md
- @context/current-feature.md

## Commands

Go commands run from the repo root; npm commands run from `web/` (or use `npm --prefix web run <script>`).

- **Full stack (Docker)**: `docker compose up --build` (or `make up`). App at http://localhost:8080, Postgres at `localhost:54329` (`board`/`board`, db `board`)
- **API locally**: `DATABASE_URL=postgres://board:board@localhost:54329/board?sslmode=disable go run ./cmd/api` (needs the compose Postgres running)
- **UI dev server**: `npm run dev` in `web/`. Serves http://localhost:5173 and proxies `/api` to :8080
- **Build**: `go build ./...` and `npm run build` in `web/` (the web build runs `tsc --noEmit` first)
- **Typecheck**: `go vet ./...` and `npx tsc --noEmit` in `web/`
- **Format check**: `gofmt -l .` (must print nothing)
- **Lint**: intentionally none. `go vet`, `gofmt` and `tsc` are the checks
- **Test**: `go test ./...` (or `make test`). No Postgres needed. There are no frontend tests
- **Install UI deps**: `npm ci` in `web/`
