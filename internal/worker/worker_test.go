package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"listingboard/internal/board"
	"listingboard/internal/store"
)

var _ Store = (*store.Store)(nil)

func TestRunWalksASheetToSuccess(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "success")
	stop()

	want := []write{
		stepWrite(job.RunID, "done", "pending", "pending"),
		stepWrite(job.RunID, "done", "done", "pending"),
		stepWrite(job.RunID, "done", "done", "done"),
		{kind: "success", runID: job.RunID, changed: true},
	}
	assertWrites(t, fs, want)
	got := fs.job(job.ID)
	if got.RunID != job.RunID || got.Error != "" {
		t.Fatalf("runId %q error %q, want %q and no error", got.RunID, got.Error, job.RunID)
	}
}

func TestRunFailsASheetWithoutABrochure(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	removeBrochure(t, dataDir, job)
	fs := newFakeStore(job)
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "failed")
	stop()

	assertWrites(t, fs, []write{{kind: "failed", runID: job.RunID, message: "Brochure is missing", changed: true}})
	got := fs.job(job.ID)
	if got.Error != "Brochure is missing" {
		t.Fatalf("error %q, want %q", got.Error, "Brochure is missing")
	}
	assertStates(t, got.Checklist, "pending", "pending", "pending")
}

func TestRunChecksTheBrochureBeforeEveryStep(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	fs.onSave = func(n int) {
		if n == 1 {
			removeBrochure(t, dataDir, job)
		}
	}
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "failed")
	stop()

	assertWrites(t, fs, []write{
		stepWrite(job.RunID, "done", "pending", "pending"),
		{kind: "failed", runID: job.RunID, message: "Brochure is missing", changed: true},
	})
}

func TestRunResumesFromTheFirstPendingStep(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	job.Checklist[0].State = "done"
	fs := newFakeStore(job)
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "success")
	stop()

	assertWrites(t, fs, []write{
		stepWrite(job.RunID, "done", "done", "pending"),
		stepWrite(job.RunID, "done", "done", "done"),
		{kind: "success", runID: job.RunID, changed: true},
	})
}

func TestRunFinishesASheetWithEveryStepDone(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	removeBrochure(t, dataDir, job)
	for i := range job.Checklist {
		job.Checklist[i].State = "done"
	}
	fs := newFakeStore(job)
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "success")
	stop()

	assertWrites(t, fs, []write{{kind: "success", runID: job.RunID, changed: true}})
}

func TestRunWritesNothingAfterCancelDuringThePause(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	w := newWorker(fs, dataDir)
	w.pause = time.Hour

	stop := startRun(t, w)
	waitFor(t, "the first step save", func() bool { return len(fs.writeLog()) == 1 })
	stop()

	assertWrites(t, fs, []write{stepWrite(job.RunID, "done", "pending", "pending")})
	if got := fs.job(job.ID); got.Status != "running" || got.RunID != job.RunID {
		t.Fatalf("status %s runId %q, want running %q", got.Status, got.RunID, job.RunID)
	}
	if len(w.active) != 0 {
		t.Fatalf("active sheets %v after Run returned, want none", w.active)
	}
}

func TestProcessStopsWhenTheRunIDChanges(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	fs.onSave = func(n int) {
		if n == 1 {
			fs.setRunID(job.ID, "another-run")
		}
	}
	w := newWorker(fs, dataDir)

	w.process(context.Background(), job)

	assertWrites(t, fs, []write{
		stepWrite(job.RunID, "done", "pending", "pending"),
		{kind: "step", runID: job.RunID, states: []string{"done", "done", "pending"}},
	})
	got := fs.job(job.ID)
	if got.Status != "running" || got.RunID != "another-run" {
		t.Fatalf("status %s runId %q, want running %q", got.Status, got.RunID, "another-run")
	}
}

func TestProcessStopsOnAStaleSnapshot(t *testing.T) {
	dataDir := t.TempDir()
	// The poll snapshot is two steps behind: a save from it would roll step 3
	// back to pending and then redo steps 2 and 3.
	stored := runningJob(t, dataDir)
	for i := range stored.Checklist {
		stored.Checklist[i].State = "done"
	}
	fs := newFakeStore(stored)
	stale := cloneJob(stored)
	stale.Checklist[1].State = "pending"
	stale.Checklist[2].State = "pending"
	w := newWorker(fs, dataDir)

	w.process(context.Background(), stale)

	assertWrites(t, fs, []write{{kind: "step", runID: stored.RunID, states: []string{"done", "done", "pending"}}})
	got := fs.job(stored.ID)
	if got.Status != "running" {
		t.Fatalf("status %s, want running", got.Status)
	}
	assertStates(t, got.Checklist, "done", "done", "done")
}

func TestPollNeverStartsASecondGoroutineForASheet(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	entered := make(chan struct{})
	release := make(chan struct{})
	fs.beforeSave = func(_ string, n int) {
		if n == 1 {
			close(entered)
			<-release
		}
	}
	w := newWorker(fs, dataDir)
	ctx := context.Background()
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() {
		unblock()
		w.wg.Wait()
	})

	w.poll(ctx)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first save never started")
	}
	w.poll(ctx)
	w.poll(ctx)
	if got := fs.saveCalls(); got != 1 {
		t.Fatalf("save calls %d while the first is blocked, want 1", got)
	}
	unblock()
	w.wg.Wait()

	assertWrites(t, fs, []write{
		stepWrite(job.RunID, "done", "pending", "pending"),
		stepWrite(job.RunID, "done", "done", "pending"),
		stepWrite(job.RunID, "done", "done", "done"),
		{kind: "success", runID: job.RunID, changed: true},
	})
	if len(w.active) != 0 {
		t.Fatalf("active sheets %v after the goroutine ended, want none", w.active)
	}
}

func TestRunSurvivesStoreErrors(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	fs.failPolls = 1
	fs.failSaves = 1
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "success")
	stop()

	assertWrites(t, fs, []write{
		{kind: "step", runID: job.RunID, states: []string{"done", "pending", "pending"}, err: true},
		stepWrite(job.RunID, "done", "pending", "pending"),
		stepWrite(job.RunID, "done", "done", "pending"),
		stepWrite(job.RunID, "done", "done", "done"),
		{kind: "success", runID: job.RunID, changed: true},
	})
	if got := fs.pollCalls(); got < 2 {
		t.Fatalf("poll calls %d, want a retry after the failed poll", got)
	}
	assertStates(t, fs.job(job.ID).Checklist, "done", "done", "done")
}

func TestRunFailsASheetWhoseBrochureIsADirectory(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	path := board.BrochurePath(dataDir, job.UserID, job.ID)
	removeBrochure(t, dataDir, job)
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	fs := newFakeStore(job)
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "failed")
	stop()

	assertWrites(t, fs, []write{{kind: "failed", runID: job.RunID, message: "Brochure is missing", changed: true}})
}

func TestProcessWritesNothingOnceCancelled(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dataDir string, job *board.Job)
	}{
		{"pending steps", func(t *testing.T, dataDir string, job *board.Job) {}},
		{"missing brochure", func(t *testing.T, dataDir string, job *board.Job) {
			removeBrochure(t, dataDir, *job)
		}},
		{"every step done", func(t *testing.T, dataDir string, job *board.Job) {
			for i := range job.Checklist {
				job.Checklist[i].State = "done"
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			job := runningJob(t, dataDir)
			tc.setup(t, dataDir, &job)
			fs := newFakeStore(job)
			w := newWorker(fs, dataDir)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			w.process(ctx, job)

			assertWrites(t, fs, nil)
			if got := fs.job(job.ID); got.Status != "running" {
				t.Fatalf("status %s, want running", got.Status)
			}
		})
	}
}

func TestRunRetriesAFailedFinishWithoutRedoingSteps(t *testing.T) {
	dataDir := t.TempDir()
	job := runningJob(t, dataDir)
	fs := newFakeStore(job)
	fs.failFinishes = 1
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	waitForStatus(t, fs, job.ID, "success")
	stop()

	assertWrites(t, fs, []write{
		stepWrite(job.RunID, "done", "pending", "pending"),
		stepWrite(job.RunID, "done", "done", "pending"),
		stepWrite(job.RunID, "done", "done", "done"),
		{kind: "success", runID: job.RunID, err: true},
		{kind: "success", runID: job.RunID, changed: true},
	})
}

func TestRunProcessesSheetsInParallel(t *testing.T) {
	dataDir := t.TempDir()
	blocked := runningJob(t, dataDir)
	free := runningJob(t, dataDir)
	fs := newFakeStore(blocked, free)
	release := make(chan struct{})
	fs.beforeSave = func(id string, _ int) {
		if id == blocked.ID {
			<-release
		}
	}
	w := newWorker(fs, dataDir)

	stop := startRun(t, w)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	// Runs before stop, so a failed assertion does not leave Run waiting.
	t.Cleanup(unblock)
	waitForStatus(t, fs, free.ID, "success")
	if got := fs.job(blocked.ID); got.Status != "running" {
		t.Fatalf("blocked sheet is %s, want running", got.Status)
	}
	unblock()
	waitForStatus(t, fs, blocked.ID, "success")
	stop()

	assertStates(t, fs.job(blocked.ID).Checklist, "done", "done", "done")
	assertStates(t, fs.job(free.ID).Checklist, "done", "done", "done")
}

// write is one call to SaveChecklist or FinishJob as the fake store saw it.
type write struct {
	kind    string // "step", "success" or "failed"
	runID   string
	states  []string
	message string
	changed bool
	err     bool
}

func stepWrite(runID string, states ...string) write {
	return write{kind: "step", runID: runID, states: states, changed: true}
}

// fakeStore mirrors the Postgres semantics: writes change a sheet only while
// it is running under the same runID, and a step save only while the stored
// checklist still equals prev.
type fakeStore struct {
	mu           sync.Mutex
	jobs         map[string]board.Job
	writes       []write
	polls        int
	saves        int
	failPolls    int
	failSaves    int
	failFinishes int
	// beforeSave runs before the n-th SaveChecklist call, onSave after it,
	// both outside the lock.
	beforeSave func(id string, n int)
	onSave     func(n int)
}

var errStore = errors.New("store is down")

func newFakeStore(jobs ...board.Job) *fakeStore {
	fs := &fakeStore{jobs: map[string]board.Job{}}
	for _, job := range jobs {
		fs.jobs[job.ID] = cloneJob(job)
	}
	return fs
}

func (f *fakeStore) RunningJobs(ctx context.Context) ([]board.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	if f.failPolls > 0 {
		f.failPolls--
		return nil, errStore
	}
	var jobs []board.Job
	for _, job := range f.jobs {
		if job.Status == "running" {
			jobs = append(jobs, cloneJob(job))
		}
	}
	return jobs, nil
}

func (f *fakeStore) SaveChecklist(ctx context.Context, id, runID string, prev, next []board.Step) (bool, error) {
	f.mu.Lock()
	f.saves++
	n := f.saves
	before, after := f.beforeSave, f.onSave
	f.mu.Unlock()
	if before != nil {
		before(id, n)
	}
	ok, err := f.saveChecklist(id, runID, prev, next)
	if after != nil {
		after(n)
	}
	return ok, err
}

func (f *fakeStore) saveChecklist(id, runID string, prev, next []board.Step) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := write{kind: "step", runID: runID, states: states(next)}
	if f.failSaves > 0 {
		f.failSaves--
		w.err = true
		f.writes = append(f.writes, w)
		return false, errStore
	}
	job, ok := f.jobs[id]
	if ok && job.Status == "running" && job.RunID == runID && slices.Equal(job.Checklist, prev) {
		job.Checklist = slices.Clone(next)
		f.jobs[id] = job
		w.changed = true
	}
	f.writes = append(f.writes, w)
	return w.changed, nil
}

func (f *fakeStore) FinishJob(ctx context.Context, id, runID, status, message string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w := write{kind: status, runID: runID, message: message}
	if f.failFinishes > 0 {
		f.failFinishes--
		w.err = true
		f.writes = append(f.writes, w)
		return false, errStore
	}
	job, ok := f.jobs[id]
	if ok && job.Status == "running" && job.RunID == runID {
		job.Status = status
		job.Error = message
		f.jobs[id] = job
		w.changed = true
	}
	f.writes = append(f.writes, w)
	return w.changed, nil
}

func (f *fakeStore) job(id string) board.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneJob(f.jobs[id])
}

func (f *fakeStore) setRunID(id, runID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	job := f.jobs[id]
	job.RunID = runID
	f.jobs[id] = job
}

func (f *fakeStore) writeLog() []write {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.writes)
}

func (f *fakeStore) saveCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saves
}

func (f *fakeStore) pollCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

func cloneJob(job board.Job) board.Job {
	job.Checklist = slices.Clone(job.Checklist)
	return job
}

func states(checklist []board.Step) []string {
	out := make([]string, len(checklist))
	for i, step := range checklist {
		out[i] = step.State
	}
	return out
}

// runningJob returns a running sheet whose brochure exists under dataDir.
func runningJob(t *testing.T, dataDir string) board.Job {
	t.Helper()
	job := board.Job{
		ID:        board.NewID(),
		UserID:    board.NewID(),
		Status:    "running",
		RunID:     board.NewID(),
		Filename:  "brochure.pdf",
		Checklist: board.NewChecklist(),
	}
	path := board.BrochurePath(dataDir, job.UserID, job.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("%PDF-1.4 brochure"), 0o644); err != nil {
		t.Fatal(err)
	}
	return job
}

func removeBrochure(t *testing.T, dataDir string, job board.Job) {
	t.Helper()
	if err := os.Remove(board.BrochurePath(dataDir, job.UserID, job.ID)); err != nil {
		t.Error(err)
	}
}

func newWorker(fs *fakeStore, dataDir string) *Worker {
	w := New(fs, dataDir, 5*time.Millisecond)
	w.pollEvery = 2 * time.Millisecond
	return w
}

// startRun runs w in the background; the returned stop cancels it and waits
// for Run to return.
func startRun(t *testing.T, w *Worker) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not return after cancel")
		}
	}
	t.Cleanup(stop)
	return stop
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForStatus(t *testing.T, fs *fakeStore, id, status string) {
	t.Helper()
	waitFor(t, "status "+status, func() bool { return fs.job(id).Status == status })
}

func assertWrites(t *testing.T, fs *fakeStore, want []write) {
	t.Helper()
	got := fs.writeLog()
	if len(got) != len(want) {
		t.Fatalf("writes %+v, want %+v", got, want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.kind != w.kind || g.runID != w.runID || g.message != w.message ||
			g.changed != w.changed || g.err != w.err || !slices.Equal(g.states, w.states) {
			t.Fatalf("write %d is %+v, want %+v", i, g, w)
		}
	}
}

func assertStates(t *testing.T, checklist []board.Step, want ...string) {
	t.Helper()
	if got := states(checklist); !slices.Equal(got, want) {
		t.Fatalf("step states %v, want %v", got, want)
	}
}
