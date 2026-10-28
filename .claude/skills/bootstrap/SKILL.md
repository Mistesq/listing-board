---
name: bootstrap
description: Adapt this AI harness to the current repository. Fills CLAUDE.md, AGENTS.md notes, project-overview, coding-standards, the Testing section and the permission allow list from what is actually in the repo. Documentation only, no source changes.
argument-hint: [existing|new]
---

# Bootstrap the AI harness

Mode: $ARGUMENTS

- `existing`: the repo already has real code. Describe and codify what IS there.
- `new`: fresh scaffold. Fill only what is known; the product description comes later from `context/project-spec.md`.
- empty: detect. If there is source code beyond a framework scaffold, use `existing`, otherwise `new`.

## Rules

- Documentation only. You may edit: `CLAUDE.md`, `AGENTS.md` (only inside the `ai-harness-rules` block), `context/*`, `.claude/settings.json`, `.claude/agents/*`. Do NOT touch source code, configs or dependencies.
- Facts only from the repo. If something is unknown, write `TODO:` instead of guessing.
- Never read `.env` files. `.env.example` is fine (variable names only).
- Keep every file short. Remove template comments you have resolved.
- Work read-first: finish steps 1-2 before writing anything.

## Steps

1. **Detect the stack**: `package.json` (scripts, dependencies with installed versions), lockfile type (npm/pnpm/yarn), framework configs, `tsconfig.json`, ORM schema and migrations, `docker-compose*.yml`, `.env.example`, CI config, `README`.
2. **Sample the code** (existing mode): 5-10 representative files: an API route or controller, a service or data-access module, a page and a component, a test file, the error handling path.
3. **CLAUDE.md**: replace `{{PROJECT_NAME}}` and `{{ONE_LINE_DESCRIPTION}}`. Rewrite Commands from the real scripts (dev, build, lint, typecheck, test, DB). Delete the Database Access Rules block unless `.mcp.json` configures a DB server.
4. **AGENTS.md**: inside the `ai-harness-rules` block, replace the `Stack notes` TODO with the real stack and installed versions of the libraries most likely to have changed since your training. If the framework manages its own block (for example Next.js `nextjs-agent-rules`), leave that block untouched.
5. **context/project-overview.md**:
   - existing: purpose (from README and routes), main modules and where they live, data model (from schema/migrations), integrations, how to run and test, known gaps.
   - new: fill only Tech Stack. Leave the rest for the spec flow.
6. **context/coding-standards.md**:
   - existing: rewrite from the code's real conventions: layering, validation, error handling and status codes, data access, naming, file layout, test style. Where the code is inconsistent, record the dominant pattern and note the exception.
   - new: trim the baseline to the installed stack and fill File Organization.
7. **context/ai-interaction.md**: adapt only the Testing section to the real runner, config file, test location and mocking approach. If there is no test runner, say so and suggest one, without installing it.
8. **.claude/settings.json**: match the allow list to the real package manager and scripts (e.g. `pnpm` instead of `npm`).
9. **Report**: a short table (file, what changed) and a list of TODOs that need my decision.
