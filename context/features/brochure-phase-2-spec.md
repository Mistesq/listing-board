# Brochure Phase 2 - One brochure per sheet, safe download

## Overview

A sheet takes exactly one brochure, even under concurrent uploads, and the download serves it with explicit, safe headers.

## Requirements

- Cheap early check: after ownership and before the body is read, a sheet with a non-empty `filename` gets 400 `A brochure is already attached`. The conditional `UPDATE` below is what guarantees this.
- Write order: (1) write the bytes to a temp file in the target directory; (2) claim the slot with `UPDATE ... SET filename = $name WHERE id = $id AND user_id = $user AND filename = ''`, a new sentinel when no row changed; (3) no row changed: remove the temp file, 400 `A brochure is already attached`; (4) row changed: rename the temp file to `body`; (5) rename failed: reset `filename = ''` `WHERE id = $id AND filename = $name`, remove the temp file, 500 `Could not store the brochure`.
- Download sets `Content-Type` explicitly from the stored bytes (the phase 1 type function): `application/pdf` or `text/plain; charset=utf-8`.
- Download sets `X-Content-Type-Options: nosniff`.
- Download builds `Content-Disposition` with `mime.FormatMediaType("attachment", map[string]string{"filename": job.Filename})`.

## Route Contract

`GET /api/jobs/{id}/file` returns 200 with the bytes, or 401 / 404 `Not found` (another account's sheet, no brochure, file missing on disk).

## Notes

- Temp file: `os.CreateTemp(dir, "body-*.tmp")` in the target directory, so the rename never crosses filesystems.
- `http.ServeContent` keeps a `Content-Type` that was set before it is called.

## Testing

1. `go test ./internal/httpapi -run 'Attach|Download' -v`: a second upload gives 400 with the exact text; 5 concurrent uploads give one 200, the rest 400, `body` holds the winner's bytes and no `*.tmp` is left; a directory pre-created at the `body` path gives 500, `filename` back to `""`, no temp file.
2. Download tests check the three headers for PDF and text; a filename with quotes and non-ASCII parses back via `mime.ParseMediaType`.
3. Docker: `curl -sD - -b jar localhost:8080/api/jobs/$ID/file -o /dev/null` shows the three headers.

## References

- `go doc mime.FormatMediaType`, `go doc os.CreateTemp`, `go doc net/http.ServeContent`
