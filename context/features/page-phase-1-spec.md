# Page Phase 1 - Live sheet progress and NOTES.md

## Overview

The open sheet follows the worker without a reload, and the repo gets `NOTES.md`.

## Requirements

- The `Job` type in `web/src/App.tsx` gets `runId: string`.
- While the open sheet is `running`, poll `GET /api/jobs/{id}` about once a second. Update both the open sheet and its entry in the list from the response. Stop when the status changes, another sheet is opened, or on unmount (clean up in the `useEffect` return).
- Show `error` with `<p className="error">` when the sheet has `failed`.
- Show the error message from `POST /start`, including the 409 `Another sheet is already running`.
- If the upload after creating a sheet fails, the sheet is still added to the list and the error is shown. An open `draft` without a brochure shows a file input that uploads to `POST /api/jobs/{id}/file` and updates the sheet from the response.
- `NOTES.md` at the repo root covers one rejected AI suggestion and how the result was checked, plus a few lines on decisions where the spec leaves room (start on final sheets, filename kept raw, brochure check per step, page polling, `STEP_PAUSE` parsing).

## Notes

- Files: `web/src/App.tsx`, `web/src/styles.css` (only if needed), `NOTES.md`.
- No new npm dependencies, no SSE. All requests go through `api<T>()`.
- Use `setTimeout` chained after each response, not a fixed `setInterval`, so slow responses do not pile up.

## Testing

1. In `web/`: `npx tsc --noEmit` and `npm run build` pass.
2. Run `docker compose up --build` and open http://localhost:8080. Create a sheet with a text brochure and start it. The checklist ticks about every 2s and ends `success`, and the list entry updates too, with no reload.
3. Start a second sheet while one is running: the 409 text is shown.
4. Create a sheet with an empty file: the error is shown and the sheet appears in the list. Open it, attach a valid `.txt` through the file input, and `filename` is filled in and start works.
5. Delete the brochure from the volume mid-run: the page shows `Brochure is missing`.
6. The network tab shows polling stops after `success`/`failed`.

## References

- https://react.dev/reference/react/useEffect (React 18.3)
