---
name: specs
description: Turn task descriptions into context/project-overview.md, then split it into small feature spec files (sized like the examples) for /feature load
argument-hint: overview [source] | split [area]
---

# Specs

Execute: $ARGUMENTS

| Action | What it does |
| ------ | ------------ |
| `overview [source]` | Generate or update `context/project-overview.md` from task descriptions |
| `split [area]` | Split the overview (or one area of it) into spec files in `context/features/` |

If no action is given, explain the two actions.

## overview

1. Source: the files or text named in $ARGUMENTS or in my message (client brief, task list, tickets, `context/project-spec.md`). If nothing is given, ask for it.
2. If `context/project-overview.md` already has content, update it: keep what still holds, change only what the new input changes.
3. Fill the sections of `project-overview.md` from the source. For every gap the source leaves open, decide whether it is **blocking**: it changes the data model, adds or removes a spec, or needs an external service or integration.
   - **Not blocking**: choose the simplest option that fits the source, write it into the right section, and add one line under `## Assumptions`: `- <topic>: <choice>. <what it means in plain words or why, a few words>. Alt: <option>.`
   - Defaults the end user sees (UI language, currency, phone and address formats, units) follow the source's context: country, audience, market. For a shop in Ukraine that means a Ukrainian UI and UAH, not English and USD.
   - **Blocking**: add it under `## Open Questions` as one line: `<n>. <question> Recommended: <option>, <reason in a few words>. Alternative: <option>.`
4. Report in this shape and nothing longer:

   ```text
   Overview written: <one line on what the project is>.

   Need your decision (recommended option first):
   1. <question>: <recommended>, <reason>. Alt: <option>.
   2. ...

   Assumed (change any by number):
   - A1 <topic>: <choice>. <what it means, a few words>.
   - A2 ...

   Reply "ok" to accept everything, or e.g. "2: uploads, A1: per size". Then run /specs split when ready.
   ```

5. On my reply: apply the decisions to the overview, clear the answered Open Questions (record the choices in the relevant sections), list in a few lines what changed, and STOP. Do not start `split` until I run `/specs split`.
6. Do not write code.

## split

1. Read `context/project-overview.md` (only the named area if one is given) and the existing files in `context/features/`, so you do not duplicate or contradict them. If blocking Open Questions are still unanswered, list them with their recommendations and ask before splitting.
2. Read the examples in [examples/](examples/) and match their size and shape (see Size rules).
3. Group the work into areas (auth, catalog, cart, ...). Inside an area order the phases: setup/config, then backend/API, then UI. Each phase builds only on earlier ones.
4. Write one file per spec, `context/features/<area>-phase-<n>-spec.md`, following [template.md](template.md).
5. Update `## Roadmap` in `project-overview.md`: the ordered list of spec files, one line each, with dependencies.
6. Report a table (file, one-line scope, depends on) and any open questions. Do not write code.

## Size rules (match the examples)

- One cohesive capability, implementable in a single `/feature start` run and verifiable end to end by its Testing steps.
- About 40-60 lines of markdown, 5-8 requirements, 3-8 files touched.
- One main layer per spec: setup/config, API, or UI. A big area becomes several phases instead of one large spec.
- Fewer than ~3 requirements: merge it into a neighbouring spec.
- Say WHAT and the key constraints, not the code: route contracts (method, path, input, responses), files to create, version-sensitive gotchas, env variable names.
- Every spec ends with concrete Testing steps (curl commands, pages to open, expected result) and References to the docs to verify against.
