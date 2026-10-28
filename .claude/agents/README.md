# Review Subagents

Custom subagents are the review layer: focused auditors you invoke after a chunk of work or before `/feature complete`. They run in their own context and report findings without editing code.

Included:

- **code-scanner**: general pass for security, correctness, performance and quality on the code you give it. It reads `CLAUDE.md` and `context/coding-standards.md` for the stack and standards, so it works in any project without editing. Usage:

  ```text
  Run git diff main --name-only, then use the code-scanner subagent on those files against the goals in @context/current-feature.md.
  ```

Worth adding per project (sharper scope = fewer false positives):

- **auth-auditor**: deep pass on auth and account flows (hashing, verification and reset tokens, session checks). Add when the project has real auth.
- **refactor-scanner**: folder-scoped DRY / extraction hunt. Add once there is enough code to have duplication.
- **ui-reviewer**: inspects the rendered page via Playwright MCP for visual, responsive and a11y defects. Needs the dev server running.

To add one: drop a `name.md` here with frontmatter (`name`, `description`, `tools`, `model`) and a precise scope and methodology that names real paths.
