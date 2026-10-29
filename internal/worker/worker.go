package worker

import (
	"context"
	"log"
	"os"
	"slices"
	"sync"
	"time"

	"listingboard/internal/board"
)

const brochureMissing = "Brochure is missing"

// Store is what the worker needs from the database. Writes report false when
// the sheet is no longer running under the given runID; SaveChecklist also
// when the stored checklist is no longer prev.
type Store interface {
	RunningJobs(ctx context.Context) ([]board.Job, error)
	SaveChecklist(ctx context.Context, id, runID string, prev, next []board.Step) (bool, error)
	FinishJob(ctx context.Context, id, runID, status, message string) (bool, error)
}

type Worker struct {
	store     Store
	dataDir   string
	pause     time.Duration
	pollEvery time.Duration

	mu     sync.Mutex
	active map[string]bool
	wg     sync.WaitGroup
}

func New(store Store, dataDir string, pause time.Duration) *Worker {
	return &Worker{
		store:     store,
		dataDir:   dataDir,
		pause:     pause,
		pollEvery: time.Second,
		active:    map[string]bool{},
	}
}

// Run polls running sheets until ctx is cancelled, then waits for every sheet
// goroutine to return. No write starts after the cancel; one already in flight
// may still commit, which is safe because every write is conditional.
func (w *Worker) Run(ctx context.Context) {
	defer w.wg.Wait()
	ticker := time.NewTicker(w.pollEvery)
	defer ticker.Stop()
	for {
		w.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) poll(ctx context.Context) {
	jobs, err := w.store.RunningJobs(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("poll running sheets: %v", err)
		}
		return
	}
	for _, job := range jobs {
		if !w.claim(job.ID) {
			continue
		}
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			defer w.release(job.ID)
			w.process(ctx, job)
		}()
	}
}

func (w *Worker) claim(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active[id] {
		return false
	}
	w.active[id] = true
	return true
}

func (w *Worker) release(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.active, id)
}

// process runs the pending steps of one sheet. It returns on cancel, on a
// store error and when a write changes no row; the next poll picks the sheet
// up again if it is still running.
func (w *Worker) process(ctx context.Context, job board.Job) {
	path := board.BrochurePath(w.dataDir, job.UserID, job.ID)
	checklist := job.Checklist
	for n := 0; ; n++ {
		i := firstPending(checklist)
		if i < 0 {
			break
		}
		if n > 0 && !w.wait(ctx) {
			return
		}
		if _, err := os.ReadFile(path); err != nil {
			w.finish(ctx, job, "failed", brochureMissing)
			return
		}
		next := slices.Clone(checklist)
		next[i].State = "done"
		if !w.write(ctx, job, "save step", func() (bool, error) {
			return w.store.SaveChecklist(ctx, job.ID, job.RunID, checklist, next)
		}) {
			return
		}
		checklist = next
	}
	w.finish(ctx, job, "success", "")
}

func (w *Worker) finish(ctx context.Context, job board.Job, status, message string) {
	w.write(ctx, job, "set "+status, func() (bool, error) {
		return w.store.FinishJob(ctx, job.ID, job.RunID, status, message)
	})
}

// write skips the call once ctx is cancelled and reports whether processing
// may go on.
func (w *Worker) write(ctx context.Context, job board.Job, what string, call func() (bool, error)) bool {
	if ctx.Err() != nil {
		return false
	}
	ok, err := call()
	switch {
	case err != nil:
		if ctx.Err() == nil {
			log.Printf("sheet %s: %s: %v", job.ID, what, err)
		}
		return false
	case !ok:
		log.Printf("sheet %s: run %s is no longer running", job.ID, job.RunID)
		return false
	}
	return true
}

func (w *Worker) wait(ctx context.Context) bool {
	timer := time.NewTimer(w.pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func firstPending(checklist []board.Step) int {
	return slices.IndexFunc(checklist, func(s board.Step) bool { return s.State == "pending" })
}
