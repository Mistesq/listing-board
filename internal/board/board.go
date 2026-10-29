package board

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"time"
	"unicode/utf8"
)

// MaxBrochureSize is the largest brochure accepted, in bytes.
const MaxBrochureSize = 1 << 20

var (
	ErrNotFound       = errors.New("not found")
	ErrDuplicate      = errors.New("duplicate")
	ErrAlreadyRunning = errors.New("another sheet is already running")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
}

type Step struct {
	Label string `json:"label"`
	State string `json:"state"`
}

type Job struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	Title     string    `json:"title"`
	Address   string    `json:"address"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	RunID     string    `json:"runId"`
	Filename  string    `json:"filename"`
	Checklist []Step    `json:"checklist"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func NewChecklist() []Step {
	return []Step{
		{Label: "Reading the brochure", State: "pending"},
		{Label: "Checking the address", State: "pending"},
		{Label: "Publishing the sheet", State: "pending"},
	}
}

func NewID() string {
	return randomHex(16)
}

func NewToken() string {
	return randomHex(32)
}

// BrochurePath is where a sheet's brochure bytes live. Only server-chosen ids
// take part in it, never the client's filename.
func BrochurePath(dataDir, userID, jobID string) string {
	return filepath.Join(dataDir, userID, jobID, "body")
}

// BrochureType names the content type of a brochure from its bytes. It
// reports false for anything that is neither a PDF nor UTF-8 text.
func BrochureType(data []byte) (string, bool) {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "application/pdf", true
	case len(data) == 0, bytes.IndexByte(data, 0) >= 0, !utf8.Valid(data):
		return "", false
	default:
		return "text/plain; charset=utf-8", true
	}
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

// Store is the persistence used by the HTTP API.
type Store interface {
	CreateUser(ctx context.Context, email, passwordHash string) (User, error)
	UserByEmail(ctx context.Context, email string) (User, error)
	CreateSession(ctx context.Context, userID, token string, expires time.Time) error
	DeleteSession(ctx context.Context, token string) error
	UserByToken(ctx context.Context, token string) (User, error)
	ListJobs(ctx context.Context, userID string) ([]Job, error)
	CreateJob(ctx context.Context, job Job) error
	JobForUser(ctx context.Context, id, userID string) (Job, error)
	UpdateJob(ctx context.Context, job Job) error
	// StartJob moves a draft to running with runID. Any other status is
	// returned unchanged, so a sheet gets its runID exactly once.
	StartJob(ctx context.Context, id, userID, runID string) (Job, error)
}
