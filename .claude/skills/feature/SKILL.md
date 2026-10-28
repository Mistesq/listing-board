---
name: feature
description: Manage current feature workflow - load, start, test, review, explain or complete
argument-hint: load|start|test|review|explain|complete
---

# Feature Workflow

Manages the full lifecycle of a feature from spec to merge.

## Working File

@context/current-feature.md

### File Structure

current-feature.md has these sections:

- `# Current Feature` - H1 heading with feature name when active
- `## Status` - Not Started | In Progress | Complete
- `## Goals` - Checklist (`- [ ]` / `- [x]`) of what success looks like
- `## Notes` - Additional context, constraints, API contract, out of scope
- `## History` - Completed features (append only)

## Task

Execute the requested action: $ARGUMENTS

| Action     | Description                                                     |
| ---------- | --------------------------------------------------------------- |
| `load`     | Load a feature spec or inline description                       |
| `start`    | Create branch, implement goals and check them off               |
| `review`   | Check goals met (plan vs actual), code quality, scope creep     |
| `test`     | Add tests for testable business logic and utilities             |
| `explain`  | Document what changed and why                                   |
| `complete` | Commit, push, merge, reset                                      |

See [actions/](actions/) for detailed instructions.

If no action provided, explain the available options.
