package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"listingboard/internal/board"
)

// migrateLockKey serializes schema.sql between the API and the worker, which
// both run it on start.
const migrateLockKey int64 = 7290001

type Store struct {
	db *sql.DB
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(5)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrateLockKey); err != nil {
		return err
	}
	// No args: pgx sends schemaSQL over the simple protocol, which runs every
	// statement in it.
	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (board.User, error) {
	user := board.User{ID: board.NewID(), Email: email, PasswordHash: passwordHash}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, password_hash, created_at)
		VALUES ($1, $2, $3, $4)
	`, user.ID, user.Email, user.PasswordHash, time.Now().UTC())
	if isUnique(err) {
		return board.User{}, board.ErrDuplicate
	}
	if err != nil {
		return board.User{}, err
	}
	return user, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (board.User, error) {
	var user board.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, password_hash FROM users WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.PasswordHash)
	return user, mapNotFound(err)
}

func (s *Store) CreateSession(ctx context.Context, userID, token string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, $3)
	`, token, userID, expires.UTC())
	return err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

func (s *Store) UserByToken(ctx context.Context, token string) (board.User, error) {
	var user board.User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email
		FROM users u
		JOIN sessions s ON s.user_id = u.id
		WHERE s.token = $1 AND s.expires_at > now()
	`, token).Scan(&user.ID, &user.Email)
	return user, mapNotFound(err)
}

func (s *Store) ListJobs(ctx context.Context, userID string) ([]board.Job, error) {
	return s.queryJobs(ctx, jobSelect+` WHERE user_id = $1 ORDER BY created_at DESC`, userID)
}

// RunningJobs returns the running sheets of every account, for the worker.
func (s *Store) RunningJobs(ctx context.Context) ([]board.Job, error) {
	return s.queryJobs(ctx, jobSelect+` WHERE status = 'running' ORDER BY created_at`)
}

func (s *Store) queryJobs(ctx context.Context, query string, args ...any) ([]board.Job, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []board.Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if jobs == nil {
		jobs = []board.Job{}
	}
	return jobs, rows.Err()
}

func (s *Store) CreateJob(ctx context.Context, job board.Job) error {
	raw, err := json.Marshal(job.Checklist)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO jobs (
			id, user_id, title, address, status, error, run_id, filename, checklist, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, job.ID, job.UserID, job.Title, job.Address, job.Status, job.Error, job.RunID, job.Filename, string(raw), job.CreatedAt.UTC(), job.UpdatedAt.UTC())
	return err
}

func (s *Store) JobForUser(ctx context.Context, id, userID string) (board.Job, error) {
	row := s.db.QueryRowContext(ctx, jobSelect+` WHERE id = $1 AND user_id = $2`, id, userID)
	job, err := scanJob(row)
	return job, mapNotFound(err)
}

// AttachBrochure claims and returns the sheet in one statement, so no second
// query can fail after the claim is committed.
func (s *Store) AttachBrochure(ctx context.Context, id, userID, filename string) (board.Job, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE jobs
		SET filename = $1, updated_at = $2
		WHERE id = $3 AND user_id = $4 AND filename = ''
		RETURNING `+jobColumns, filename, time.Now().UTC(), id, userID)
	job, err := scanJob(row)
	if !errors.Is(err, sql.ErrNoRows) {
		return job, err
	}
	// No row changed: tell an unknown sheet from one that has a brochure.
	if _, err := s.JobForUser(ctx, id, userID); err != nil {
		return board.Job{}, err
	}
	return board.Job{}, board.ErrBrochureAttached
}

func (s *Store) DetachBrochure(ctx context.Context, id, filename string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET filename = '', updated_at = $1
		WHERE id = $2 AND filename = $3 AND status = 'draft'
	`, time.Now().UTC(), id, filename)
	return err
}

func (s *Store) StartJob(ctx context.Context, id, userID, runID string) (board.Job, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = 'running', run_id = $1, updated_at = $2
		WHERE id = $3 AND user_id = $4 AND status = 'draft' AND filename <> ''
	`, runID, time.Now().UTC(), id, userID)
	if isConstraint(err, runningIndex) {
		return board.Job{}, board.ErrAlreadyRunning
	}
	if err != nil {
		return board.Job{}, err
	}
	return s.JobForUser(ctx, id, userID)
}

// SaveChecklist replaces prev with next and reports false when the sheet is no
// longer running under runID or its checklist is no longer prev, so a stale
// snapshot never writes over a newer one.
func (s *Store) SaveChecklist(ctx context.Context, id, runID string, prev, next []board.Step) (bool, error) {
	rawPrev, err := json.Marshal(prev)
	if err != nil {
		return false, err
	}
	rawNext, err := json.Marshal(next)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET checklist = $1, updated_at = $2
		WHERE id = $3 AND status = 'running' AND run_id = $4 AND checklist = $5
	`, string(rawNext), time.Now().UTC(), id, runID, string(rawPrev))
	return changed(res, err)
}

// FinishJob sets a final status and reports false when the sheet is no longer
// running under runID.
func (s *Store) FinishJob(ctx context.Context, id, runID, status, message string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE jobs
		SET status = $1, error = $2, updated_at = $3
		WHERE id = $4 AND status = 'running' AND run_id = $5
	`, status, message, time.Now().UTC(), id, runID)
	return changed(res, err)
}

func changed(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

const runningIndex = "jobs_one_running_per_user"

const jobColumns = `id, user_id, title, address, status, error, run_id, filename, checklist, created_at, updated_at`

const jobSelect = `
	SELECT ` + jobColumns + `
	FROM jobs`

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(row scanner) (board.Job, error) {
	var job board.Job
	var raw string
	err := row.Scan(
		&job.ID, &job.UserID, &job.Title, &job.Address, &job.Status, &job.Error,
		&job.RunID, &job.Filename, &raw, &job.CreatedAt, &job.UpdatedAt,
	)
	if err != nil {
		return board.Job{}, err
	}
	if err := json.Unmarshal([]byte(raw), &job.Checklist); err != nil {
		return board.Job{}, err
	}
	if job.Checklist == nil {
		job.Checklist = []board.Step{}
	}
	return job, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return board.ErrNotFound
	}
	return err
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func isConstraint(err error, name string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == name
}
