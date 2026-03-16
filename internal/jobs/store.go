package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/david-loe/volume-mover/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

type ListFilter struct {
	Host      string
	Operation string
	Status    string
	Limit     int
}

func DefaultDBPath(configPath string) string {
	if configPath == "" {
		configDir, err := os.UserConfigDir()
		if err == nil {
			return filepath.Join(configDir, "volume-mover", "jobs.db")
		}
		return filepath.Join(".", "jobs.db")
	}
	return filepath.Join(filepath.Dir(configPath), "jobs.db")
}

func NewStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			operation TEXT NOT NULL,
			source_host TEXT NOT NULL,
			destination_host TEXT NOT NULL,
			status TEXT NOT NULL,
			allow_live INTEGER NOT NULL,
			quiesce_source INTEGER NOT NULL,
			requested_by TEXT,
			error TEXT,
			created_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS job_items (
			job_id TEXT NOT NULL,
			item_index INTEGER NOT NULL,
			source_volume TEXT NOT NULL,
			destination_volume TEXT NOT NULL,
			status TEXT NOT NULL,
			bytes_estimated INTEGER NOT NULL DEFAULT 0,
			bytes_copied INTEGER NOT NULL DEFAULT 0,
			warnings_json TEXT NOT NULL DEFAULT '[]',
			error TEXT,
			source_cleanup TEXT,
			PRIMARY KEY(job_id, item_index)
		);`,
		`CREATE TABLE IF NOT EXISTS job_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL,
			item_index INTEGER NOT NULL,
			level TEXT NOT NULL,
			step TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	return nil
}

func (s *Store) RecoverInterruptedJobs(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = ?, error = ?, finished_at = ? WHERE status IN (?, ?, ?)`,
		string(model.JobStatusFailed),
		"job interrupted by process restart",
		now,
		string(model.JobStatusQueued),
		string(model.JobStatusRunning),
		string(model.JobStatusCancelling),
	)
	return err
}

func (s *Store) CreateJob(ctx context.Context, job model.TransferJob) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO jobs (id, operation, source_host, destination_host, status, allow_live, quiesce_source, requested_by, error, created_at, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID,
		string(job.Operation),
		job.SourceHost,
		job.DestinationHost,
		string(job.Status),
		boolInt(job.AllowLive),
		boolInt(job.QuiesceSource),
		job.RequestedBy,
		job.Error,
		job.CreatedAt.UTC().Format(time.RFC3339Nano),
		nullTime(job.StartedAt),
		nullTime(job.FinishedAt),
	)
	if err != nil {
		return err
	}
	for _, item := range job.Items {
		warningsJSON, _ := json.Marshal(item.Warnings)
		_, err = tx.ExecContext(ctx, `INSERT INTO job_items (job_id, item_index, source_volume, destination_volume, status, bytes_estimated, bytes_copied, warnings_json, error, source_cleanup)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			job.ID,
			item.Index,
			item.SourceVolume,
			item.DestinationVolume,
			string(item.Status),
			item.BytesEstimated,
			item.BytesCopied,
			string(warningsJSON),
			item.Error,
			item.SourceCleanup,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateJobStatus(ctx context.Context, jobID string, status model.TransferJobStatus, errText string, startedAt *time.Time, finishedAt *time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = ?, error = ?, started_at = COALESCE(?, started_at), finished_at = ? WHERE id = ?`,
		string(status), errText, nullTime(startedAt), nullTime(finishedAt), jobID,
	)
	return err
}

func (s *Store) UpdateJobItem(ctx context.Context, jobID string, item model.TransferJobItem) error {
	warningsJSON, _ := json.Marshal(item.Warnings)
	_, err := s.db.ExecContext(ctx, `UPDATE job_items SET status = ?, bytes_estimated = ?, bytes_copied = ?, warnings_json = ?, error = ?, source_cleanup = ? WHERE job_id = ? AND item_index = ?`,
		string(item.Status), item.BytesEstimated, item.BytesCopied, string(warningsJSON), item.Error, item.SourceCleanup, jobID, item.Index,
	)
	return err
}

func (s *Store) AppendEvent(ctx context.Context, event model.TransferJobEvent) (model.TransferJobEvent, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO job_events (job_id, item_index, level, step, message, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		event.JobID, event.ItemIndex, event.Level, event.Step, event.Message, event.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return model.TransferJobEvent{}, err
	}
	id, _ := res.LastInsertId()
	event.ID = id
	return event, nil
}

func (s *Store) GetJob(ctx context.Context, id string) (model.TransferJob, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, operation, source_host, destination_host, status, allow_live, quiesce_source, requested_by, error, created_at, started_at, finished_at FROM jobs WHERE id = ?`, id)
	job, err := scanJob(row)
	if err != nil {
		return model.TransferJob{}, err
	}
	items, err := s.jobItems(ctx, id)
	if err != nil {
		return model.TransferJob{}, err
	}
	events, err := s.JobEvents(ctx, id)
	if err != nil {
		return model.TransferJob{}, err
	}
	job.Items = items
	job.Events = events
	if job.Items == nil {
		job.Items = []model.TransferJobItem{}
	}
	if job.Events == nil {
		job.Events = []model.TransferJobEvent{}
	}
	job.Summary = summarize(items)
	return job, nil
}

func (s *Store) ListJobs(ctx context.Context, filter ListFilter) ([]model.TransferJob, error) {
	query := `SELECT id, operation, source_host, destination_host, status, allow_live, quiesce_source, requested_by, error, created_at, started_at, finished_at FROM jobs`
	var clauses []string
	var args []any
	if filter.Host != "" {
		clauses = append(clauses, `(source_host = ? OR destination_host = ?)`)
		args = append(args, filter.Host, filter.Host)
	}
	if filter.Operation != "" {
		clauses = append(clauses, `operation = ?`)
		args = append(args, filter.Operation)
	}
	if filter.Status != "" {
		clauses = append(clauses, `status = ?`)
		args = append(args, filter.Status)
	}
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	query += ` ORDER BY datetime(created_at) DESC`
	if filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filter.Limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []model.TransferJob
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		items, err := s.jobItems(ctx, job.ID)
		if err != nil {
			return nil, err
		}
		job.Items = items
		if job.Items == nil {
			job.Items = []model.TransferJobItem{}
		}
		job.Summary = summarize(items)
		jobs = append(jobs, job)
	}
	if jobs == nil {
		jobs = []model.TransferJob{}
	}
	return jobs, rows.Err()
}

func (s *Store) JobEvents(ctx context.Context, jobID string) ([]model.TransferJobEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, job_id, item_index, level, step, message, created_at FROM job_events WHERE job_id = ? ORDER BY id ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []model.TransferJobEvent
	for rows.Next() {
		var (
			event   model.TransferJobEvent
			created string
		)
		if err := rows.Scan(&event.ID, &event.JobID, &event.ItemIndex, &event.Level, &event.Step, &event.Message, &created); err != nil {
			return nil, err
		}
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	if events == nil {
		events = []model.TransferJobEvent{}
	}
	return events, rows.Err()
}

func (s *Store) CancelJob(ctx context.Context, jobID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status = ? WHERE id = ? AND status IN (?, ?, ?)`,
		string(model.JobStatusCancelling), jobID,
		string(model.JobStatusQueued), string(model.JobStatusValidating), string(model.JobStatusRunning),
	)
	return err
}

func (s *Store) jobItems(ctx context.Context, jobID string) ([]model.TransferJobItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_index, source_volume, destination_volume, status, bytes_estimated, bytes_copied, warnings_json, error, source_cleanup FROM job_items WHERE job_id = ? ORDER BY item_index ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []model.TransferJobItem
	for rows.Next() {
		var item model.TransferJobItem
		var warningsJSON string
		if err := rows.Scan(&item.Index, &item.SourceVolume, &item.DestinationVolume, &item.Status, &item.BytesEstimated, &item.BytesCopied, &warningsJSON, &item.Error, &item.SourceCleanup); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(warningsJSON), &item.Warnings)
		items = append(items, item)
	}
	if items == nil {
		items = []model.TransferJobItem{}
	}
	return items, rows.Err()
}

func scanJob(scanner interface{ Scan(dest ...any) error }) (model.TransferJob, error) {
	var (
		job                model.TransferJob
		operation, status  string
		allowLive, quiesce int
		createdAt          string
		startedAt          sql.NullString
		finishedAt         sql.NullString
	)
	if err := scanner.Scan(&job.ID, &operation, &job.SourceHost, &job.DestinationHost, &status, &allowLive, &quiesce, &job.RequestedBy, &job.Error, &createdAt, &startedAt, &finishedAt); err != nil {
		return model.TransferJob{}, err
	}
	job.Operation = model.TransferOperation(operation)
	job.Status = model.TransferJobStatus(status)
	job.AllowLive = allowLive == 1
	job.QuiesceSource = quiesce == 1
	job.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		job.StartedAt = &t
	}
	if finishedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
		job.FinishedAt = &t
	}
	return job, nil
}

func summarize(items []model.TransferJobItem) model.TransferJobSummary {
	s := model.TransferJobSummary{TotalItems: len(items)}
	for _, item := range items {
		s.BytesEstimated += item.BytesEstimated
		s.BytesCopied += item.BytesCopied
		switch item.Status {
		case model.JobStatusCompleted:
			s.CompletedItems++
		case model.JobStatusFailed:
			s.FailedItems++
		case model.JobStatusCancelled:
			s.CancelledItems++
		case model.JobStatusRunning, model.JobStatusValidating, model.JobStatusCancelling:
			s.RunningItems++
		default:
			s.QueuedItems++
		}
	}
	return s
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
