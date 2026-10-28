# AI Interaction Guidelines

## Communication

- Be concise and direct
- Explain non-obvious decisions briefly
- Ask before large refactors or architectural changes
- Don't add features not in the project spec
- Never delete files without clarification
- I may write prompts in Ukrainian or Russian. Reply in the language I wrote in.
- Everything written to files is in English: code, comments, specs, context files, current-feature.md, commit messages, README.

## Workflow

This is the common workflow that we will use for every single feature/fix:

1. **Document** - Document the feature in @context/current-feature.md.
2. **Branch** - Create new branch for feature, fix, etc
3. **Implement** - Implement the feature/fix that I create in @context/current-feature.md
4. **Test** - Verify it works (browser for UI, curl or a test for APIs). Add/maintain unit tests for the logic you touch, see [Testing](#testing). Run the typecheck from CLAUDE.md and fix any errors. Run the build before `/feature complete`
5. **Iterate** - Iterate and change things if needed
6. **Commit** - Only after typecheck and tests pass and everything works
7. **Merge** - Merge to main
8. **Delete Branch** - Delete branch after merge
9. **Review** - Review AI-generated code periodically and on demand.
10. Mark as completed in @context/current-feature.md and add to history

Do NOT commit without permission and until typecheck and tests pass. If they fail, fix the issues first.

## Branching

We will create a new branch for every feature/fix. Name branch **feature/[feature]** or **fix[fix]**, etc. Ask to delete the branch once merged.

## Commits

- Ask before committing (don't auto-commit)
- Use conventional commit messages (feat:, fix:, chore:, etc.)
- Keep commits focused (one feature/fix per commit)
- Never put "Generated With Claude" in the commit messages

## When Stuck

- If something isn't working after 2-3 attempts, stop and explain the issue
- Don't keep trying random fixes
- Ask for clarification if requirements are unclear

## Code Changes

- Make minimal changes to accomplish the task
- Don't refactor unrelated code unless asked
- Don't add "nice to have" features
- Preserve existing patterns in the codebase
- Don't add dependencies without asking
- Never read or print .env values

## Testing

<!-- /bootstrap adapts this section to the real test runner, config and mocking style. -->

Go stdlib `testing` + `net/http/httptest`. Run with `go test ./...`. No extra test libraries.

- **Scope: Go backend logic.** Handlers, state transitions, worker steps, upload validation, pure helpers. No UI tests unless asked.
- Co-locate tests as `*_test.go` in the same package (e.g. `internal/httpapi/server_test.go`, `package httpapi`).
- Keep tests true units: **no real Postgres or network**. Use the in-memory fake `mem` in `server_test.go`, which implements `board.Store`; extend it whenever the interface changes. Use `t.TempDir()` for `DATA_DIR`.
- Drive HTTP through `srv.Handler()` with the existing helpers (`signUp`, `createSheet`, `uploadFile`, `call`, `decodeJob`); helpers call `t.Helper()`.
- Name tests as behavior sentences (`TestStartRequiresASession`). Cover the happy path plus expected failures (400, 401, 404 incl. another account's sheet, 409).
- Do not delete, skip or weaken the README's named tests (see `context/project-overview.md`).
- `web/` has no test runner. If UI logic grows enough to need tests, suggest Vitest (ask before adding it).

Run `go test ./...`, `go vet ./...` and the `web/` typecheck before committing.

## Code Review

Review AI-generated code periodically, especially for:

- Security (auth checks, input validation)
- Performance (unnecessary re-renders, N+1 queries)
- Logic errors (edge cases)
- Patterns (matches existing codebase?)
