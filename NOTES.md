# Notes

## Rejected AI suggestion

During code review, the AI suggested calling `DetachBrochure` whenever the upload's claim (`AttachBrochure`) failed, in case the claim had committed just before the error. I rejected it because the reset matches on `filename = $name`, and two uploads to one sheet can carry the same name. If A's claim fails before commit and B then claims the slot, A's reset clears B's `filename`. B gets a 200, but the sheet shows no brochure. Instead, I made the claim a single `UPDATE ... RETURNING` with no read after the commit, called with `context.WithoutCancel`, and dropped the blind reset. I checked it with a handler test that a cancelled request still claims the slot (it fails with 500 once `WithoutCancel` is removed) and with Postgres integration tests for a single claim, concurrent claims and a reset that only touches a matching draft.

## Decisions where the spec leaves room

- Start on a `success` or `failed` sheet returns 200 and the sheet unchanged; a finished sheet stays finished.
- `filename` stores the raw client name from the multipart header (e.g. `../../x.txt`); it is metadata only and never part of a path.
- The worker checks that the brochure is readable before every step, so a file lost mid-run fails the sheet with `Brochure is missing`.
- The page polls `GET /api/jobs/{id}` about once a second while the open sheet is `running`, and `GET /api/jobs` about every 2 seconds while another sheet in the list is `running`, each request after the previous response; no SSE.
- `STEP_PAUSE`: unset or empty means `2s`, `0` is allowed, an unparsable or negative value stops the worker at startup with an error.
- New error texts only for new cases: `Another sheet is already running` (409 on start), `A brochure is already attached` (second upload), `Brochure file name is not valid` (non-UTF-8 or over 255 bytes); existing texts are kept.

## Known trade-offs

- Upload claims the slot in the database before renaming the temp file to `body`; a start in that short window makes the worker fail the sheet with `Brochure is missing`. A normal user cannot hit it, because the upload response arrives after the rename.
- `StartJob` re-reads the sheet in a second query after the conditional `UPDATE`, so the response can already show a later worker step, and a failed re-read after a committed start gives 500 while a retry gives the `running` sheet.
- If the database connection breaks during the claim `UPDATE`, the outcome is unknown; the upload answers 500 and the sheet can be left with `filename` and no `body` (see the rejected suggestion for why there is no reset).
- Every concurrent upload to a sheet writes its validated bytes (up to 1 MiB) to its own temp file before the claim picks a winner; losers remove theirs.
- A process killed between writing the temp file and the rename leaves a `body-*.tmp` behind; nothing sweeps them.

## How to verify

```sh
docker compose up --build    # app at http://localhost:8080
go test ./...                # no Postgres needed
```

The store integration tests run against Postgres alone, without the `worker` service, which would pick up the tests' `running` sheets:

```sh
docker compose down             # stop the full stack if it is running
docker compose up -d postgres
DATABASE_URL='postgres://board:board@localhost:54329/board?sslmode=disable' go test ./internal/store -v
```

## How this was built

The work went spec by spec (`context/features/`), with the rules for the AI in `CLAUDE.md` and `context/`. Each phase was reviewed by a code-scanner subagent and by me before merging.
