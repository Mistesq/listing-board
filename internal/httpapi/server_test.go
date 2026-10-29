package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"listingboard/internal/board"
)

func TestStartMarksTheJobSuccessful(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
	uploadFile(t, srv, cookie, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")

	res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	got := decodeJob(t, res)
	if got.Status != "running" {
		t.Fatalf("status %s, want running", got.Status)
	}
	if got.RunID == "" {
		t.Fatal("runId is empty")
	}
	if len(got.Checklist) != 3 {
		t.Fatalf("checklist has %d steps, want 3", len(got.Checklist))
	}
	for _, step := range got.Checklist {
		if step.State != "pending" {
			t.Fatalf("step %q is %s, want pending", step.Label, step.State)
		}
	}

	again := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, "")
	if again.Code != http.StatusOK {
		t.Fatalf("second start: status %d: %s", again.Code, again.Body.String())
	}
	if second := decodeJob(t, again); second.Status != "running" || second.RunID != got.RunID {
		t.Fatalf("second start: status %s runId %q, want running %q", second.Status, second.RunID, got.RunID)
	}
}

func TestUploadKeepsTheClientFilename(t *testing.T) {
	srv, dir := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	userID := currentUserID(t, srv, cookie)
	job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
	data := []byte("Two bedrooms, south facing.\n")
	got := uploadFile(t, srv, cookie, job.ID, "../../escaped.txt", data, "text/plain")

	if got.Filename != "../../escaped.txt" {
		t.Fatalf("filename %q, want the client name ../../escaped.txt", got.Filename)
	}
	path := filepath.Join(dir, userID, job.ID, "body")
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("brochure was not stored at %s: %v", path, err)
	}
	if !bytes.Equal(stored, data) {
		t.Fatalf("stored %q, want %q", stored, data)
	}
	if files := filesUnder(t, dir); len(files) != 1 || files[0] != path {
		t.Fatalf("files under DATA_DIR %v, want only %s", files, path)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escaped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a file escaped DATA_DIR: %v", err)
	}
}

func TestListHidesOtherUsersJobs(t *testing.T) {
	srv, _ := newTestServer(t)
	ada := signUp(t, srv, "ada@example.com")
	grace := signUp(t, srv, "grace@example.com")
	job := createSheet(t, srv, ada, "Market loft", "10 Market Street")

	res := call(srv, http.MethodGet, "/api/jobs", grace, nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	var jobs []board.Job
	if err := json.Unmarshal(res.Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	for _, item := range jobs {
		if item.ID == job.ID {
			t.Fatal("another account can see this sheet in the list")
		}
	}
}

func TestStartRequiresASession(t *testing.T) {
	srv, _ := newTestServer(t)
	res := call(srv, http.MethodPost, "/api/jobs/"+board.NewID()+"/start", "", nil, "")
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", res.Code)
	}
}

func TestStartRequiresABrochure(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")

	res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", res.Code, res.Body.String())
	}
	if got := decodeError(t, res); got != "Attach a brochure first" {
		t.Fatalf("error %q", got)
	}
}

func TestStartRejectsASecondRunningSheet(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	first := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
	second := createSheet(t, srv, cookie, "Harbour flat", "2 Harbour Road")
	uploadFile(t, srv, cookie, first.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")
	uploadFile(t, srv, cookie, second.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")
	if res := call(srv, http.MethodPost, "/api/jobs/"+first.ID+"/start", cookie, nil, ""); res.Code != http.StatusOK {
		t.Fatalf("first start: status %d: %s", res.Code, res.Body.String())
	}

	res := call(srv, http.MethodPost, "/api/jobs/"+second.ID+"/start", cookie, nil, "")
	if res.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", res.Code, res.Body.String())
	}
	if got := decodeError(t, res); got != "Another sheet is already running" {
		t.Fatalf("error %q", got)
	}
	got := decodeJob(t, call(srv, http.MethodGet, "/api/jobs/"+second.ID, cookie, nil, ""))
	if got.Status != "draft" || got.RunID != "" {
		t.Fatalf("second sheet: status %s runId %q, want an untouched draft", got.Status, got.RunID)
	}
}

func TestStartChecksTheBrochureBeforeTheRunningSheet(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	running := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
	uploadFile(t, srv, cookie, running.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")
	if res := call(srv, http.MethodPost, "/api/jobs/"+running.ID+"/start", cookie, nil, ""); res.Code != http.StatusOK {
		t.Fatalf("first start: status %d: %s", res.Code, res.Body.String())
	}
	bare := createSheet(t, srv, cookie, "Harbour flat", "2 Harbour Road")

	res := call(srv, http.MethodPost, "/api/jobs/"+bare.ID+"/start", cookie, nil, "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", res.Code, res.Body.String())
	}
	if got := decodeError(t, res); got != "Attach a brochure first" {
		t.Fatalf("error %q", got)
	}
}

func TestStartHidesAStoreFailure(t *testing.T) {
	srv := New(failingStart{newMem()}, t.TempDir(), "")
	cookie := signUp(t, srv, "ada@example.com")
	job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
	uploadFile(t, srv, cookie, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")

	res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, "")
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", res.Code, res.Body.String())
	}
	if got := decodeError(t, res); got != "Could not start the sheet" {
		t.Fatalf("error %q", got)
	}
}

type failingStart struct{ *mem }

func (failingStart) StartJob(context.Context, string, string, string) (board.Job, error) {
	return board.Job{}, errors.New("connection reset by peer")
}

func TestStartRunsSheetsOfDifferentAccountsAtOnce(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, email := range []string{"ada@example.com", "grace@example.com"} {
		cookie := signUp(t, srv, email)
		job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
		uploadFile(t, srv, cookie, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")
		if res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, ""); res.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", email, res.Code, res.Body.String())
		}
	}
}

func TestStartLeavesAFinishedSheetUnchanged(t *testing.T) {
	for _, status := range []string{"success", "failed"} {
		t.Run(status, func(t *testing.T) {
			store := newMem()
			srv := New(store, t.TempDir(), "")
			cookie := signUp(t, srv, "ada@example.com")
			job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
			uploadFile(t, srv, cookie, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")
			started := decodeJob(t, call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, ""))
			store.setStatus(job.ID, status)

			res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, "")
			if res.Code != http.StatusOK {
				t.Fatalf("status %d: %s", res.Code, res.Body.String())
			}
			if got := decodeJob(t, res); got.Status != status || got.RunID != started.RunID {
				t.Fatalf("status %s runId %q, want %s %q", got.Status, got.RunID, status, started.RunID)
			}
		})
	}
}

func TestConcurrentStartsChooseOneRunID(t *testing.T) {
	srv, _ := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
	uploadFile(t, srv, cookie, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")

	const n = 10
	results := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/start", cookie, nil, "")
		}()
	}
	wg.Wait()

	var runID string
	for i, res := range results {
		if res.Code != http.StatusOK {
			t.Fatalf("start %d: status %d: %s", i, res.Code, res.Body.String())
		}
		got := decodeJob(t, res)
		if runID == "" {
			runID = got.RunID
		}
		if got.RunID == "" || got.RunID != runID {
			t.Fatalf("start %d: runId %q, want %q", i, got.RunID, runID)
		}
	}
}

func TestIDRoutesHideAnotherOwnersSheet(t *testing.T) {
	srv, _ := newTestServer(t)
	ada := signUp(t, srv, "ada@example.com")
	grace := signUp(t, srv, "grace@example.com")
	job := createSheet(t, srv, ada, "Market loft", "10 Market Street")
	uploadFile(t, srv, ada, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")

	for _, path := range []string{"/api/jobs/" + job.ID, "/api/jobs/" + job.ID + "/file"} {
		if res := call(srv, http.MethodGet, path, ada, nil, ""); res.Code != http.StatusOK {
			t.Fatalf("owner GET %s: status %d: %s", path, res.Code, res.Body.String())
		}
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/jobs/" + job.ID},
		{http.MethodPost, "/api/jobs/" + job.ID + "/file"},
		{http.MethodPost, "/api/jobs/" + job.ID + "/start"},
		{http.MethodGet, "/api/jobs/" + job.ID + "/file"},
	}
	sessions := []struct {
		name    string
		cookie  string
		status  int
		message string
	}{
		{"another account", grace, http.StatusNotFound, "Not found"},
		{"no session", "", http.StatusUnauthorized, "Sign in"},
	}
	for _, session := range sessions {
		for _, route := range routes {
			t.Run(session.name+" "+route.method+" "+route.path, func(t *testing.T) {
				res := call(srv, route.method, route.path, session.cookie, nil, "")
				if res.Code != session.status {
					t.Fatalf("status %d, want %d: %s", res.Code, session.status, res.Body.String())
				}
				if got := decodeError(t, res); got != session.message {
					t.Fatalf("error %q, want %q", got, session.message)
				}
			})
		}
	}
}

func TestAnotherOwnersSheetLooksLikeAnUnknownOne(t *testing.T) {
	srv, _ := newTestServer(t)
	ada := signUp(t, srv, "ada@example.com")
	grace := signUp(t, srv, "grace@example.com")
	job := createSheet(t, srv, ada, "Market loft", "10 Market Street")
	uploadFile(t, srv, ada, job.ID, "brochure.pdf", []byte("%PDF-1.4\n"), "application/pdf")
	unknown := board.NewID()

	routes := []struct {
		method string
		suffix string
	}{
		{http.MethodGet, ""},
		{http.MethodPost, "/file"},
		{http.MethodPost, "/start"},
		{http.MethodGet, "/file"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.suffix, func(t *testing.T) {
			theirs := call(srv, route.method, "/api/jobs/"+job.ID+route.suffix, grace, nil, "")
			missing := call(srv, route.method, "/api/jobs/"+unknown+route.suffix, grace, nil, "")
			if theirs.Code != missing.Code || theirs.Body.String() != missing.Body.String() {
				t.Fatalf("another account's sheet: %d %q, unknown sheet: %d %q",
					theirs.Code, theirs.Body.String(), missing.Code, missing.Body.String())
			}
		})
	}
}

func TestUploadChecksTheOwnerBeforeReadingTheBody(t *testing.T) {
	srv, _ := newTestServer(t)
	ada := signUp(t, srv, "ada@example.com")
	grace := signUp(t, srv, "grace@example.com")
	job := createSheet(t, srv, ada, "Market loft", "10 Market Street")

	body := &readRecorder{r: strings.NewReader("--x\r\n")}
	res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/file", grace, body, "multipart/form-data; boundary=x")
	if res.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %s", res.Code, res.Body.String())
	}
	if body.read {
		t.Fatal("the request body was read before the ownership check")
	}
}

func TestUploadDecidesTheBrochureFromTheBytes(t *testing.T) {
	cases := []struct {
		name        string
		filename    string
		contentType string
		data        []byte
		want        string
	}{
		{"pdf", "brochure.pdf", "application/pdf", []byte("%PDF-1.4\n%binary\n"), ""},
		{"text", "brochure.txt", "text/plain", []byte("Two bedrooms, south facing.\n"), ""},
		{"utf-8 text", "brochure.txt", "text/plain", []byte("Дві спальні, вид на південь.\n"), ""},
		{"%PDF- inside a txt name", "notes.txt", "text/plain", []byte("The plan says %PDF-1.4 in the middle.\n"), ""},
		{"pdf bytes sent as text/plain", "brochure.pdf", "text/plain", []byte("%PDF-1.7\n"), ""},
		{"exactly 1 MiB", "big.txt", "text/plain", bytes.Repeat([]byte("a"), 1048576), ""},
		{"file name of 255 bytes", strings.Repeat("a", 251) + ".txt", "text/plain", []byte("Two bedrooms\n"), ""},
		{"file name of 256 bytes", strings.Repeat("a", 252) + ".txt", "text/plain", []byte("Two bedrooms\n"), "Brochure file name is not valid"},
		{"file name not valid utf-8", "\xff.txt", "text/plain", []byte("Two bedrooms\n"), "Brochure file name is not valid"},
		{"empty", "brochure.txt", "text/plain", nil, "Brochure must be a PDF or a text file"},
		{"nul byte", "brochure.txt", "text/plain", []byte("Two\x00bedrooms"), "Brochure must be a PDF or a text file"},
		{"invalid utf-8", "brochure.txt", "text/plain", []byte("Two \xff\xfe bedrooms"), "Brochure must be a PDF or a text file"},
		{"jpeg bytes named and typed as pdf", "brochure.pdf", "application/pdf", []byte("\xff\xd8\xff\xe0JFIF"), "Brochure must be a PDF or a text file"},
		{"one byte over 1 MiB", "big.txt", "text/plain", bytes.Repeat([]byte("a"), 1048577), "Brochure is too large"},
		{"one byte over 1 MiB of a wrong type", "big.bin", "application/octet-stream", make([]byte, 1048577), "Brochure is too large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, dir := newTestServer(t)
			cookie := signUp(t, srv, "ada@example.com")
			userID := currentUserID(t, srv, cookie)
			job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")

			res := postFile(t, srv, cookie, job.ID, tc.filename, tc.data, tc.contentType)
			if tc.want != "" {
				if res.Code != http.StatusBadRequest {
					t.Fatalf("status %d, want 400: %s", res.Code, res.Body.String())
				}
				if got := decodeError(t, res); got != tc.want {
					t.Fatalf("error %q, want %q", got, tc.want)
				}
				assertEmptyDir(t, dir)
				return
			}
			if res.Code != http.StatusOK {
				t.Fatalf("status %d, want 200: %s", res.Code, res.Body.String())
			}
			if got := decodeJob(t, res); got.Filename != tc.filename {
				t.Fatalf("filename %q, want %q", got.Filename, tc.filename)
			}
			stored, err := os.ReadFile(filepath.Join(dir, userID, job.ID, "body"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(stored, tc.data) {
				t.Fatalf("stored %d bytes, want the %d uploaded", len(stored), len(tc.data))
			}
		})
	}
}

func TestUploadRejectsABrokenForm(t *testing.T) {
	file := formPart{disposition: `form-data; name="file"; filename="brochure.txt"`, data: []byte("Two bedrooms\n")}
	cases := []struct {
		name string
		body func(t *testing.T) (io.Reader, string)
		want string
	}{
		{"no file part", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, formPart{disposition: `form-data; name="notes"`, data: []byte("hi")})
		}, "Choose a brochure file"},
		{"file part without a filename", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, formPart{disposition: `form-data; name="file"`, data: []byte("Two bedrooms\n")})
		}, "Choose a brochure file"},
		{"blank filename", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, formPart{disposition: `form-data; name="file"; filename="  "`, data: []byte("Two bedrooms\n")})
		}, "Choose a brochure file"},
		{"cut before the closing boundary", func(t *testing.T) (io.Reader, string) {
			buf, formType := multipartBody(t, file)
			raw := buf.Bytes()
			return bytes.NewReader(raw[:bytes.LastIndex(raw, []byte("\r\n--"))]), formType
		}, "Brochure upload was incomplete"},
		{"not multipart", func(t *testing.T) (io.Reader, string) {
			return strings.NewReader("Two bedrooms\n"), "text/plain"
		}, "Brochure upload was incomplete"},
		{"request body over the limit before the file", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t, formPart{disposition: `form-data; name="notes"`, data: bytes.Repeat([]byte("a"), 2<<20)}, file)
		}, "Brochure is too large"},
		{"request body over the limit inside a 1 MiB file", func(t *testing.T) (io.Reader, string) {
			return multipartBody(t,
				formPart{disposition: `form-data; name="notes"`, data: bytes.Repeat([]byte("a"), 100<<10)},
				formPart{disposition: `form-data; name="file"; filename="big.txt"`, data: bytes.Repeat([]byte("a"), 1048576)},
			)
		}, "Brochure is too large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, dir := newTestServer(t)
			cookie := signUp(t, srv, "ada@example.com")
			job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")

			body, formType := tc.body(t)
			res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/file", cookie, body, formType)
			if res.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %s", res.Code, res.Body.String())
			}
			if got := decodeError(t, res); got != tc.want {
				t.Fatalf("error %q, want %q", got, tc.want)
			}
			assertEmptyDir(t, dir)
		})
	}
}

func TestUploadTakesTheFirstFilePart(t *testing.T) {
	srv, dir := newTestServer(t)
	cookie := signUp(t, srv, "ada@example.com")
	userID := currentUserID(t, srv, cookie)
	job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")

	body, formType := multipartBody(t,
		formPart{disposition: `form-data; name="notes"`, data: []byte("call before visiting")},
		formPart{disposition: `form-data; name="file"; filename="first.txt"`, data: []byte("First brochure\n")},
		formPart{disposition: `form-data; name="file"; filename="second.txt"`, data: []byte("Second brochure\n")},
	)
	res := call(srv, http.MethodPost, "/api/jobs/"+job.ID+"/file", cookie, body, formType)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if got := decodeJob(t, res); got.Filename != "first.txt" {
		t.Fatalf("filename %q, want first.txt", got.Filename)
	}
	stored, err := os.ReadFile(filepath.Join(dir, userID, job.ID, "body"))
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != "First brochure\n" {
		t.Fatalf("stored %q, want the first file part", stored)
	}
}

func TestDownloadReturnsTheStoredBrochure(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		data     []byte
	}{
		{"pdf", "brochure.pdf", []byte("%PDF-1.4\n%binary\n")},
		{"text", "brochure.txt", []byte("Two bedrooms, south facing.\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, dir := newTestServer(t)
			cookie := signUp(t, srv, "ada@example.com")
			userID := currentUserID(t, srv, cookie)
			job := createSheet(t, srv, cookie, "Market loft", "10 Market Street")
			uploadFile(t, srv, cookie, job.ID, tc.filename, tc.data, "application/octet-stream")

			res := call(srv, http.MethodGet, "/api/jobs/"+job.ID+"/file", cookie, nil, "")
			if res.Code != http.StatusOK {
				t.Fatalf("status %d: %s", res.Code, res.Body.String())
			}
			if !bytes.Equal(res.Body.Bytes(), tc.data) {
				t.Fatalf("downloaded %q, want %q", res.Body.Bytes(), tc.data)
			}

			if err := os.Remove(filepath.Join(dir, userID, job.ID, "body")); err != nil {
				t.Fatal(err)
			}
			if res := call(srv, http.MethodGet, "/api/jobs/"+job.ID+"/file", cookie, nil, ""); res.Code != http.StatusNotFound {
				t.Fatalf("after removing body: status %d, want 404", res.Code)
			}
		})
	}
}

type readRecorder struct {
	r    io.Reader
	read bool
}

func (b *readRecorder) Read(p []byte) (int, error) {
	b.read = true
	return b.r.Read(p)
}

func decodeError(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", res.Body.String(), err)
	}
	return body.Error
}

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	return New(newMem(), dir, ""), dir
}

func signUp(t *testing.T, srv *Server, email string) string {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"password123"}`)
	res := call(srv, http.MethodPost, "/api/register", "", body, "application/json")
	if res.Code != http.StatusCreated {
		t.Fatalf("register %d: %s", res.Code, res.Body.String())
	}
	for _, cookie := range res.Result().Cookies() {
		if cookie.Name == sessionCookie {
			return cookie.Value
		}
	}
	t.Fatal("register did not set a session")
	return ""
}

func createSheet(t *testing.T, srv *Server, cookie, title, address string) board.Job {
	t.Helper()
	body := strings.NewReader(`{"title":"` + title + `","address":"` + address + `"}`)
	res := call(srv, http.MethodPost, "/api/jobs", cookie, body, "application/json")
	if res.Code != http.StatusCreated {
		t.Fatalf("create %d: %s", res.Code, res.Body.String())
	}
	return decodeJob(t, res)
}

func uploadFile(t *testing.T, srv *Server, cookie, id, name string, data []byte, contentType string) board.Job {
	t.Helper()
	res := postFile(t, srv, cookie, id, name, data, contentType)
	if res.Code != http.StatusOK {
		t.Fatalf("upload %d: %s", res.Code, res.Body.String())
	}
	return decodeJob(t, res)
}

func postFile(t *testing.T, srv *Server, cookie, id, name string, data []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	body, formType := multipartBody(t, formPart{
		disposition: fmt.Sprintf(`form-data; name="file"; filename="%s"`, name),
		contentType: contentType,
		data:        data,
	})
	return call(srv, http.MethodPost, "/api/jobs/"+id+"/file", cookie, body, formType)
}

type formPart struct {
	disposition string
	contentType string
	data        []byte
}

func multipartBody(t *testing.T, parts ...formPart) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for _, p := range parts {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", p.disposition)
		if p.contentType != "" {
			header.Set("Content-Type", p.contentType)
		}
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(p.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, writer.FormDataContentType()
}

func currentUserID(t *testing.T, srv *Server, cookie string) string {
	t.Helper()
	res := call(srv, http.MethodGet, "/api/me", cookie, nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("me %d: %s", res.Code, res.Body.String())
	}
	var me account
	if err := json.Unmarshal(res.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	return me.ID
}

func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// assertEmptyDir fails unless dir has no entries at all, not even folders.
func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("DATA_DIR is not empty: %v", entries)
	}
}

func decodeJob(t *testing.T, res *httptest.ResponseRecorder) board.Job {
	t.Helper()
	var job board.Job
	if err := json.Unmarshal(res.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	return job
}

func call(srv *Server, method, path, cookie string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	return res
}

type mem struct {
	mu       sync.Mutex
	users    map[string]board.User
	byEmail  map[string]string
	sessions map[string]memSession
	jobs     map[string]board.Job
}

type memSession struct {
	userID  string
	expires time.Time
}

func newMem() *mem {
	return &mem{
		users:    map[string]board.User{},
		byEmail:  map[string]string{},
		sessions: map[string]memSession{},
		jobs:     map[string]board.Job{},
	}
}

func (m *mem) CreateUser(_ context.Context, email, passwordHash string) (board.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.byEmail[email]; ok {
		return board.User{}, board.ErrDuplicate
	}
	user := board.User{ID: board.NewID(), Email: email, PasswordHash: passwordHash}
	m.users[user.ID] = user
	m.byEmail[email] = user.ID
	return user, nil
}

func (m *mem) UserByEmail(_ context.Context, email string) (board.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byEmail[email]
	if !ok {
		return board.User{}, board.ErrNotFound
	}
	return m.users[id], nil
}

func (m *mem) CreateSession(_ context.Context, userID, token string, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[token] = memSession{userID: userID, expires: expires}
	return nil
}

func (m *mem) DeleteSession(_ context.Context, token string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, token)
	return nil
}

func (m *mem) UserByToken(_ context.Context, token string) (board.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.sessions[token]
	if !ok || time.Now().After(session.expires) {
		return board.User{}, board.ErrNotFound
	}
	user, ok := m.users[session.userID]
	if !ok {
		return board.User{}, board.ErrNotFound
	}
	return user, nil
}

func (m *mem) ListJobs(_ context.Context, userID string) ([]board.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var jobs []board.Job
	for _, job := range m.jobs {
		if job.UserID == userID {
			jobs = append(jobs, cloneJob(job))
		}
	}
	if jobs == nil {
		jobs = []board.Job{}
	}
	return jobs, nil
}

func (m *mem) CreateJob(_ context.Context, job board.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.ID] = cloneJob(job)
	return nil
}

func (m *mem) JobForUser(_ context.Context, id, userID string) (board.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok || job.UserID != userID {
		return board.Job{}, board.ErrNotFound
	}
	return cloneJob(job), nil
}

func (m *mem) UpdateJob(_ context.Context, job board.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.jobs[job.ID]; !ok {
		return board.ErrNotFound
	}
	m.jobs[job.ID] = cloneJob(job)
	return nil
}

func (m *mem) StartJob(_ context.Context, id, userID, runID string) (board.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok || job.UserID != userID {
		return board.Job{}, board.ErrNotFound
	}
	if job.Status != "draft" {
		return cloneJob(job), nil
	}
	for _, other := range m.jobs {
		if other.UserID == userID && other.Status == "running" {
			return board.Job{}, board.ErrAlreadyRunning
		}
	}
	job.Status = "running"
	job.RunID = runID
	job.UpdatedAt = time.Now().UTC()
	m.jobs[id] = job
	return cloneJob(job), nil
}

// setStatus stands in for the worker, which is the only writer of final statuses.
func (m *mem) setStatus(id, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	job.Status = status
	m.jobs[id] = job
}

func cloneJob(job board.Job) board.Job {
	job.Checklist = append([]board.Step(nil), job.Checklist...)
	return job
}
