package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"listingboard/internal/board"
)

// shared is opened once per package: schema.sql runs without a lock, and
// concurrent CREATE ... IF NOT EXISTS can race in Postgres.
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
			job.Status = status
			job.RunID = board.NewID()
			if err := s.UpdateJob(ctx, job); err != nil {
				t.Fatal(err)
			}

			got, err := s.StartJob(ctx, job.ID, ada.ID, board.NewID())
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != status || got.RunID != job.RunID {
				t.Fatalf("status %s runId %q, want %s %q", got.Status, got.RunID, status, job.RunID)
			}
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
	return user
}

func newDraft(t *testing.T, s *Store, userID string) board.Job {
	t.Helper()
	now := time.Now().UTC()
	job := board.Job{
		ID:        board.NewID(),
		UserID:    userID,
		Title:     "Market loft",
		Address:   "10 Market Street",
		Status:    "draft",
		Filename:  "brochure.pdf",
		Checklist: board.NewChecklist(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	return job
}
