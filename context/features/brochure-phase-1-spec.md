# Brochure Phase 1 - Streamed upload, byte-based type, fixed path

## Overview

Upload reads the multipart body as a stream, enforces the exact 1 MiB limit, takes the type from the bytes, and stores the file at `{DATA_DIR}/{userID}/{jobID}/body`.

## Requirements

- One shared helper in `internal/board` (e.g. `BrochurePath(dataDir, userID, jobID)`) builds `{DATA_DIR}/{userID}/{jobID}/body`. Upload, download and the worker all use it, and client input never takes part in a path. Download switches to this path in this phase; its headers are left to phase 2.
- Read the body with `r.MultipartReader()`, not `ParseMultipartForm`. A missing `file` part (or one without a filename) gives 400 `Choose a brochure file`; broken multipart gives 400 `Brochure upload was incomplete`.
- Exact limit: read at most 1048577 bytes; 1048576 bytes pass, 1048577 bytes give 400 `Brochure is too large`. `MaxBytesReader` on the whole body leaves headroom for the envelope (e.g. +64 KiB), and hitting it (`*http.MaxBytesError`) is also 400 with the same text `Brochure is too large`, never 413.
- A pure function in `internal/board` takes the type from the bytes: `%PDF-` prefix means `application/pdf`; valid UTF-8 with no NUL and no `%PDF-` prefix means `text/plain; charset=utf-8`; empty or anything else gives 400 `Brochure must be a PDF or a text file`. Remove `headerAllows`.
- Type and size are validated before anything is written under `DATA_DIR`.
- Rewrite `TestUploadKeepsTheClientFilename` (keep the name): bytes are at `{dir}/{userID}/{jobID}/body`, `filename` equals the client name, and `../../escaped.txt` creates nothing outside `{dir}/{userID}/{jobID}`.

## Notes

- `filename` stores the raw client name, e.g. `../../escaped.txt`, as metadata only. `multipart.Part.FileName()` strips the directory, so read the name from `Content-Disposition` with `mime.ParseMediaType`, as `filenameFromHeader` does today.
- This phase still saves through `UpdateJob`; phase 2 replaces that with the atomic attach.

## Testing

1. `go test ./internal/board ./internal/httpapi -run 'Brochure|Upload' -v`: in new tests, a table covers PDF, text, empty, NUL, invalid UTF-8, `%PDF-` inside a `.txt` name, PDF bytes sent as `text/plain`, 1048576 vs 1048577 bytes. Every rejected case leaves `DATA_DIR` empty.
2. The rewritten `TestUploadKeepsTheClientFilename` passes.
3. A new test uploads PDF and text bytes, then `GET /api/jobs/{id}/file` returns 200 with exactly the same bytes, read from `{dir}/{userID}/{jobID}/body`.

## References

- `go doc net/http.Request.MultipartReader`, `go doc net/http.MaxBytesReader`, `go doc unicode/utf8.Valid`
