package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"listingboard/internal/board"
)

// shared is opened once per package, so the schema migration runs once and
// parallel tests share one connection pool.
var shared *Store

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn != "" {
		s, err := Open(dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open store:", err)
			os.Exit(1)
		}
		shared = s
	}
	code := m.Run()
	if shared != nil {
		_ = shared.Close()
	}
	os.Exit(code)
}

func TestJobForUserHidesAnotherOwnersSheet(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	grace := newUser(t, s)
	job := newDraft(t, s, ada.ID)

	if _, err := s.JobForUser(ctx, job.ID, ada.ID); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if _, err := s.JobForUser(ctx, job.ID, grace.ID); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("another account: err %v, want ErrNotFound", err)
	}
}

func TestStartJobMovesADraftToRunningOnce(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	job := newDraft(t, s, ada.ID)

	first, err := s.StartJob(ctx, job.ID, ada.ID, board.NewID())
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "running" || first.RunID == "" {
		t.Fatalf("status %s runId %q, want running with a runId", first.Status, first.RunID)
	}
	for _, step := range first.Checklist {
		if step.State != "pending" {
			t.Fatalf("step %q is %s, want pending", step.Label, step.State)
		}
	}

	second, err := s.StartJob(ctx, job.ID, ada.ID, board.NewID())
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "running" || second.RunID != first.RunID {
		t.Fatalf("second start: status %s runId %q, want running %q", second.Status, second.RunID, first.RunID)
	}
}

func TestStartJobIgnoresAnotherOwnersSheet(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	grace := newUser(t, s)
	job := newDraft(t, s, ada.ID)

	if _, err := s.StartJob(ctx, job.ID, grace.ID, board.NewID()); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("another account: err %v, want ErrNotFound", err)
	}
	got, err := s.JobForUser(ctx, job.ID, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "draft" || got.RunID != "" {
		t.Fatalf("status %s runId %q, want an untouched draft", got.Status, got.RunID)
	}
}

func TestStartJobConcurrentCallsChooseOneRunID(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	job := newDraft(t, s, ada.ID)

	const n = 10
	jobs := make([]board.Job, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobs[i], errs[i] = s.StartJob(ctx, job.ID, ada.ID, board.NewID())
		}()
	}
	wg.Wait()

	stored, err := s.JobForUser(ctx, job.ID, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("start %d: %v", i, errs[i])
		}
		if jobs[i].RunID != stored.RunID || jobs[i].Status != "running" {
			t.Fatalf("start %d: status %s runId %q, want running %q", i, jobs[i].Status, jobs[i].RunID, stored.RunID)
		}
	}
}

func TestStartJobAllowsOneRunningSheetPerAccount(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	grace := newUser(t, s)
	first := newDraft(t, s, ada.ID)
	second := newDraft(t, s, ada.ID)
	theirs := newDraft(t, s, grace.ID)

	if _, err := s.StartJob(ctx, first.ID, ada.ID, board.NewID()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartJob(ctx, second.ID, ada.ID, board.NewID()); !errors.Is(err, board.ErrAlreadyRunning) {
		t.Fatalf("second sheet: err %v, want ErrAlreadyRunning", err)
	}
	got, err := s.JobForUser(ctx, second.ID, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "draft" || got.RunID != "" {
		t.Fatalf("second sheet: status %s runId %q, want an untouched draft", got.Status, got.RunID)
	}
	if _, err := s.StartJob(ctx, theirs.ID, grace.ID, board.NewID()); err != nil {
		t.Fatalf("another account: %v", err)
	}
}

func TestStartJobLeavesAFinishedSheetUnchanged(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	for _, status := range []string{"success", "failed"} {
		t.Run(status, func(t *testing.T) {
			ada := newUser(t, s)
			job := newDraft(t, s, ada.ID)
			runID := board.NewID()
			setStatus(t, s, job.ID, status, runID)

			got, err := s.StartJob(ctx, job.ID, ada.ID, board.NewID())
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != status || got.RunID != runID {
				t.Fatalf("status %s runId %q, want %s %q", got.Status, got.RunID, status, runID)
			}
		})
	}
}

func TestStartJobNeedsABrochure(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	job := newDraftWith(t, s, ada.ID, "")

	got, err := s.StartJob(ctx, job.ID, ada.ID, board.NewID())
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "draft" || got.RunID != "" {
		t.Fatalf("status %s runId %q, want an untouched draft", got.Status, got.RunID)
	}
}

func TestAttachBrochureClaimsTheSlotOnce(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	grace := newUser(t, s)
	job := newDraftWith(t, s, ada.ID, "")

	if _, err := s.AttachBrochure(ctx, job.ID, grace.ID, "theirs.pdf"); !errors.Is(err, board.ErrNotFound) {
		t.Fatalf("another account: err %v, want ErrNotFound", err)
	}
	got, err := s.AttachBrochure(ctx, job.ID, ada.ID, " brochure.pdf ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Filename != " brochure.pdf " {
		t.Fatalf("filename %q, want the raw name", got.Filename)
	}
	if _, err := s.AttachBrochure(ctx, job.ID, ada.ID, "second.pdf"); !errors.Is(err, board.ErrBrochureAttached) {
		t.Fatalf("second attach: err %v, want ErrBrochureAttached", err)
	}
	stored, err := s.JobForUser(ctx, job.ID, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Filename != " brochure.pdf " {
		t.Fatalf("stored filename %q, want the first one", stored.Filename)
	}
}

func TestAttachBrochureConcurrentCallsHaveOneWinner(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	job := newDraftWith(t, s, ada.ID, "")

	const n = 5
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.AttachBrochure(ctx, job.ID, ada.ID, fmt.Sprintf("brochure-%d.pdf", i))
		}()
	}
	wg.Wait()

	winner := -1
	for i, err := range errs {
		switch {
		case err == nil && winner == -1:
			winner = i
		case err == nil:
			t.Fatalf("attach %d and %d both won", winner, i)
		case !errors.Is(err, board.ErrBrochureAttached):
			t.Fatalf("attach %d: %v", i, err)
		}
	}
	if winner == -1 {
		t.Fatal("no attach won")
	}
	stored, err := s.JobForUser(ctx, job.ID, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("brochure-%d.pdf", winner); stored.Filename != want {
		t.Fatalf("stored filename %q, want the winner's %q", stored.Filename, want)
	}
}

func TestDetachBrochureResetsOnlyAMatchingDraft(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)

	draft := newDraftWith(t, s, ada.ID, "brochure.pdf")
	if err := s.DetachBrochure(ctx, draft.ID, "other.pdf"); err != nil {
		t.Fatal(err)
	}
	if got := filenameOf(t, s, draft.ID, ada.ID); got != "brochure.pdf" {
		t.Fatalf("another filename: filename %q, want it kept", got)
	}
	if err := s.DetachBrochure(ctx, draft.ID, "brochure.pdf"); err != nil {
		t.Fatal(err)
	}
	if got := filenameOf(t, s, draft.ID, ada.ID); got != "" {
		t.Fatalf("matching draft: filename %q, want it reset", got)
	}

	running := newDraftWith(t, s, ada.ID, "brochure.pdf")
	if _, err := s.StartJob(ctx, running.ID, ada.ID, board.NewID()); err != nil {
		t.Fatal(err)
	}
	if err := s.DetachBrochure(ctx, running.ID, "brochure.pdf"); err != nil {
		t.Fatal(err)
	}
	if got := filenameOf(t, s, running.ID, ada.ID); got != "brochure.pdf" {
		t.Fatalf("running sheet: filename %q, want it kept", got)
	}
}

func TestListJobsShowsOnlyTheOwnersSheetsNewestFirst(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	grace := newUser(t, s)

	empty, err := s.ListJobs(ctx, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("no sheets: got %#v, want an empty non-nil slice", empty)
	}

	now := time.Now().UTC()
	older := newDraftAt(t, s, ada.ID, now.Add(-time.Minute))
	newer := newDraftAt(t, s, ada.ID, now)
	newDraft(t, s, grace.ID)

	jobs, err := s.ListJobs(ctx, ada.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].ID != newer.ID || jobs[1].ID != older.ID {
		t.Fatalf("got %d sheets, want ada's two, newest first", len(jobs))
	}
}

func TestRunningJobsListsEveryAccount(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	grace := newUser(t, s)
	adas := newRunning(t, s, ada.ID)
	graces := newRunning(t, s, grace.ID)
	draft := newDraft(t, s, ada.ID)

	jobs, err := s.RunningJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]board.Job{}
	for _, job := range jobs {
		if job.Status != "running" {
			t.Fatalf("sheet %s is %s, want only running sheets", job.ID, job.Status)
		}
		found[job.ID] = job
	}
	for _, want := range []board.Job{adas, graces} {
		got, ok := found[want.ID]
		if !ok {
			t.Fatalf("running sheet %s is missing", want.ID)
		}
		if got.UserID != want.UserID || got.RunID != want.RunID || len(got.Checklist) != 3 {
			t.Fatalf("sheet %s: user %q runId %q steps %d, want %q %q 3",
				want.ID, got.UserID, got.RunID, len(got.Checklist), want.UserID, want.RunID)
		}
	}
	if _, ok := found[draft.ID]; ok {
		t.Fatal("a draft is listed as running")
	}
}

func TestSaveChecklistNeedsTheCurrentRun(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	ada := newUser(t, s)
	done := board.NewChecklist()
	done[0].State = "done"

	t.Run("current run", func(t *testing.T) {
		job := newRunning(t, s, ada.ID)
		ok, err := s.SaveChecklist(ctx, job.ID, job.RunID, job.Checklist, done)
		if err != nil || !ok {
			t.Fatalf("ok %v err %v, want a changed row", ok, err)
		}
		got := jobOf(t, s, job.ID, ada.ID)
		if got.Status != "running" || got.RunID != job.RunID || got.Checklist[0].State != "done" {
			t.Fatalf("status %s runId %q steps %+v, want running %q with step 1 done", got.Status, got.RunID, got.Checklist, job.RunID)
		}
	})
	t.Run("old run", func(t *testing.T) {
		job := newRunning(t, s, newUser(t, s).ID)
		assertNoChange(t, s, job, func() (bool, error) {
			return s.SaveChecklist(ctx, job.ID, board.NewID(), job.Checklist, done)
		})
	})
	t.Run("stale checklist", func(t *testing.T) {
		job := newRunning(t, s, newUser(t, s).ID)
		stale := job.Checklist
		job.Checklist = board.NewChecklist()
		for i := range job.Checklist {
			job.Checklist[i].State = "done"
		}
		if ok, err := s.SaveChecklist(ctx, job.ID, job.RunID, stale, job.Checklist); err != nil || !ok {
			t.Fatalf("ok %v err %v, want a changed row", ok, err)
		}
		// A save from the all-pending snapshot would roll steps 2 and 3 back.
		assertNoChange(t, s, job, func() (bool, error) {
			return s.SaveChecklist(ctx, job.ID, job.RunID, stale, done)
		})
	})
	for _, status := range []string{"draft", "success", "failed"} {
		t.Run(status, func(t *testing.T) {
			job := newRunning(t, s, newUser(t, s).ID)
			setStatus(t, s, job.ID, status, job.RunID)
			job.Status = status
			assertNoChange(t, s, job, func() (bool, error) {
				return s.SaveChecklist(ctx, job.ID, job.RunID, job.Checklist, done)
			})
		})
	}
}

func TestFinishJobNeedsTheCurrentRun(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	for _, tc := range []struct{ status, message string }{
		{"success", ""},
		{"failed", "Brochure is missing"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			ada := newUser(t, s)
			job := newRunning(t, s, ada.ID)

			assertNoChange(t, s, job, func() (bool, error) {
				return s.FinishJob(ctx, job.ID, board.NewID(), tc.status, tc.message)
			})
			ok, err := s.FinishJob(ctx, job.ID, job.RunID, tc.status, tc.message)
			if err != nil || !ok {
				t.Fatalf("ok %v err %v, want a changed row", ok, err)
			}
			got := jobOf(t, s, job.ID, ada.ID)
			if got.Status != tc.status || got.Error != tc.message || got.RunID != job.RunID {
				t.Fatalf("status %s error %q runId %q, want %s %q %q", got.Status, got.Error, got.RunID, tc.status, tc.message, job.RunID)
			}

			// A finished sheet is final: a second write with the same run changes nothing.
			job = got
			assertNoChange(t, s, job, func() (bool, error) {
				return s.FinishJob(ctx, job.ID, job.RunID, "running", "")
			})
		})
	}
}

func TestIsConstraintTellsUniqueViolationsApart(t *testing.T) {
	running := &pgconn.PgError{Code: "23505", ConstraintName: runningIndex}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"running index", running, true},
		{"wrapped running index", fmt.Errorf("exec: %w", running), true},
		{"email unique", &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}, false},
		{"other code on the index", &pgconn.PgError{Code: "23503", ConstraintName: runningIndex}, false},
		{"not a pg error", errors.New(runningIndex), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isConstraint(tc.err, runningIndex); got != tc.want {
				t.Fatalf("isConstraint = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMigrateWaitsForTheLock(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrateLockKey); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.migrate() }()
	select {
	case err := <-done:
		t.Fatalf("migrate finished while another transaction held the lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("migrate did not finish after the lock was released")
	}
}

func TestOpenMigratesAnEmptySchemaConcurrently(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	schema := "test_" + board.NewID()
	if _, err := s.db.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := s.db.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})
	dsn := withSearchPath(t, os.Getenv("DATABASE_URL"), schema)

	const n = 4
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			st, err := Open(dsn)
			if err == nil {
				err = st.Close()
			}
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("open %d: %v", i, err)
		}
	}
	var tables int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables WHERE table_schema = $1
	`, schema).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 3 {
		t.Fatalf("tables in %s = %d, want 3", schema, tables)
	}
}

// withSearchPath points a URL-style DSN at schema, which pgx sends to Postgres
// as a runtime parameter.
func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		t.Skip("DATABASE_URL is not a URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func openStore(t *testing.T) *Store {
	t.Helper()
	if shared == nil {
		t.Skip("DATABASE_URL is not set")
	}
	t.Parallel()
	return shared
}

func newUser(t *testing.T, s *Store) board.User {
	t.Helper()
	user, err := s.CreateUser(context.Background(), "test-"+board.NewID()+"@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteUser(t, s, user.ID) })
	return user
}

// deleteUser removes a test user with its sessions and sheets, so no running
// sheet without a brochure is left for a live worker to fail.
func deleteUser(t *testing.T, s *Store, userID string) {
	t.Helper()
	for _, query := range []string{
		`DELETE FROM sessions WHERE user_id = $1`,
		`DELETE FROM jobs WHERE user_id = $1`,
		`DELETE FROM users WHERE id = $1`,
	} {
		if _, err := s.db.ExecContext(context.Background(), query, userID); err != nil {
			t.Errorf("clean up user %s: %v", userID, err)
			return
		}
	}
}

func newDraft(t *testing.T, s *Store, userID string) board.Job {
	t.Helper()
	return newDraftWith(t, s, userID, "brochure.pdf")
}

func newDraftWith(t *testing.T, s *Store, userID, filename string) board.Job {
	t.Helper()
	return createDraft(t, s, userID, filename, time.Now().UTC())
}

func newDraftAt(t *testing.T, s *Store, userID string, created time.Time) board.Job {
	t.Helper()
	return createDraft(t, s, userID, "brochure.pdf", created)
}

func createDraft(t *testing.T, s *Store, userID, filename string, now time.Time) board.Job {
	t.Helper()
	job := board.Job{
		ID:        board.NewID(),
		UserID:    userID,
		Title:     "Market loft",
		Address:   "10 Market Street",
		Status:    "draft",
		Filename:  filename,
		Checklist: board.NewChecklist(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	return job
}

func filenameOf(t *testing.T, s *Store, id, userID string) string {
	t.Helper()
	return jobOf(t, s, id, userID).Filename
}

func jobOf(t *testing.T, s *Store, id, userID string) board.Job {
	t.Helper()
	job, err := s.JobForUser(context.Background(), id, userID)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func newRunning(t *testing.T, s *Store, userID string) board.Job {
	t.Helper()
	job := newDraft(t, s, userID)
	started, err := s.StartJob(context.Background(), job.ID, userID, board.NewID())
	if err != nil {
		t.Fatal(err)
	}
	return started
}

// assertNoChange runs a conditional write that must not match and checks the
// stored sheet kept its status, error, runId and checklist.
func assertNoChange(t *testing.T, s *Store, job board.Job, write func() (bool, error)) {
	t.Helper()
	ok, err := write()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("write reported a changed row, want none")
	}
	got := jobOf(t, s, job.ID, job.UserID)
	if got.Status != job.Status || got.Error != job.Error || got.RunID != job.RunID ||
		!slices.Equal(got.Checklist, job.Checklist) {
		t.Fatalf("sheet changed to %+v, want %+v", got, job)
	}
}

// setStatus stands in for the worker, which is the only writer of final statuses.
func setStatus(t *testing.T, s *Store, id, status, runID string) {
	t.Helper()
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE jobs SET status = $1, run_id = $2 WHERE id = $3`, status, runID, id)
	if err != nil {
		t.Fatal(err)
	}
}
