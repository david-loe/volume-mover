package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/david-loe/volume-mover/internal/model"
	"github.com/david-loe/volume-mover/internal/service"
)

type Manager struct {
	store     *Store
	service   *service.Service
	mu        sync.RWMutex
	subs      map[string]map[chan model.TransferJobEvent]struct{}
	cancelled map[string]bool
}

func NewManager(store *Store, svc *service.Service) (*Manager, error) {
	if store == nil || svc == nil {
		return nil, errors.New("store and service are required")
	}
	m := &Manager{
		store:     store,
		service:   svc,
		subs:      map[string]map[chan model.TransferJobEvent]struct{}{},
		cancelled: map[string]bool{},
	}
	if err := store.RecoverInterruptedJobs(context.Background()); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) CreateJob(ctx context.Context, req model.CreateTransferJobRequest) (model.TransferJob, error) {
	if err := validateJobRequest(req); err != nil {
		return model.TransferJob{}, err
	}
	now := time.Now().UTC()
	job := model.TransferJob{
		ID:              randomID(),
		Operation:       req.Operation,
		SourceHost:      req.SourceHost,
		DestinationHost: req.DestinationHost,
		Status:          model.JobStatusQueued,
		AllowLive:       req.AllowLive,
		QuiesceSource:   req.QuiesceSource,
		CreatedAt:       now,
		Items:           make([]model.TransferJobItem, 0, len(req.Items)),
	}
	for i, item := range req.Items {
		job.Items = append(job.Items, model.TransferJobItem{
			Index:             i,
			SourceVolume:      item.SourceVolume,
			DestinationVolume: item.DestinationVolume,
			Status:            model.JobStatusQueued,
		})
	}
	job.Summary = summarize(job.Items)
	if err := m.store.CreateJob(ctx, job); err != nil {
		return model.TransferJob{}, err
	}
	go m.run(job.ID)
	return m.store.GetJob(context.Background(), job.ID)
}

func (m *Manager) ListJobs(ctx context.Context, filter ListFilter) ([]model.TransferJob, error) {
	return m.store.ListJobs(ctx, filter)
}

func (m *Manager) Job(ctx context.Context, id string) (model.TransferJob, error) {
	return m.store.GetJob(ctx, id)
}

func (m *Manager) Events(ctx context.Context, id string) ([]model.TransferJobEvent, error) {
	return m.store.JobEvents(ctx, id)
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	m.mu.Lock()
	m.cancelled[id] = true
	m.mu.Unlock()
	if err := m.store.CancelJob(ctx, id); err != nil {
		return err
	}
	_, _ = m.emit(context.Background(), model.TransferJobEvent{JobID: id, ItemIndex: -1, Level: "warning", Step: "cancel", Message: "cancel requested", CreatedAt: time.Now().UTC()})
	return nil
}

func (m *Manager) Subscribe(jobID string) (<-chan model.TransferJobEvent, func()) {
	ch := make(chan model.TransferJobEvent, 32)
	m.mu.Lock()
	if _, ok := m.subs[jobID]; !ok {
		m.subs[jobID] = map[chan model.TransferJobEvent]struct{}{}
	}
	m.subs[jobID][ch] = struct{}{}
	m.mu.Unlock()
	cancel := func() {
		m.mu.Lock()
		if subs, ok := m.subs[jobID]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(m.subs, jobID)
			}
		}
		close(ch)
		m.mu.Unlock()
	}
	return ch, cancel
}

func (m *Manager) run(jobID string) {
	ctx := context.Background()
	job, err := m.store.GetJob(ctx, jobID)
	if err != nil {
		return
	}
	started := time.Now().UTC()
	_ = m.store.UpdateJobStatus(ctx, jobID, model.JobStatusValidating, "", &started, nil)
	_, _ = m.emit(ctx, model.TransferJobEvent{JobID: jobID, ItemIndex: -1, Level: "info", Step: "queue", Message: "job accepted", CreatedAt: time.Now().UTC()})
	_ = m.store.UpdateJobStatus(ctx, jobID, model.JobStatusRunning, "", &started, nil)

	for idx, item := range job.Items {
		if m.cancelRequested(jobID) {
			m.finishCancelled(ctx, jobID, idx)
			return
		}
		item.Status = model.JobStatusRunning
		_ = m.store.UpdateJobItem(ctx, jobID, item)
		req := model.TransferRequest{
			Operation:         job.Operation,
			SourceHost:        job.SourceHost,
			SourceVolume:      item.SourceVolume,
			DestinationHost:   job.DestinationHost,
			DestinationVolume: item.DestinationVolume,
			AllowLive:         job.AllowLive,
			QuiesceSource:     job.QuiesceSource,
		}
		result, err := m.service.TransferWithCallbacks(ctx, req, service.TransferCallbacks{
			OnStep: func(step, message string) {
				_, _ = m.emit(context.Background(), model.TransferJobEvent{JobID: jobID, ItemIndex: idx, Level: "info", Step: step, Message: message, CreatedAt: time.Now().UTC()})
			},
			OnWarning: func(message string) {
				_, _ = m.emit(context.Background(), model.TransferJobEvent{JobID: jobID, ItemIndex: idx, Level: "warning", Step: "warning", Message: message, CreatedAt: time.Now().UTC()})
			},
			CheckCancel: func(step string) error {
				if m.cancelRequested(jobID) {
					return errors.New("job cancelled")
				}
				return nil
			},
		})
		item.BytesEstimated = result.BytesCopied
		item.BytesCopied = result.BytesCopied
		item.Warnings = append([]string{}, result.Warnings...)
		item.SourceCleanup = result.SourceCleanup
		if err != nil {
			if m.cancelRequested(jobID) {
				item.Status = model.JobStatusCancelled
				item.Error = "cancelled"
				_ = m.store.UpdateJobItem(ctx, jobID, item)
				m.finishCancelled(ctx, jobID, idx)
				return
			}
			item.Status = model.JobStatusFailed
			item.Error = err.Error()
			_ = m.store.UpdateJobItem(ctx, jobID, item)
			finished := time.Now().UTC()
			_ = m.store.UpdateJobStatus(ctx, jobID, model.JobStatusFailed, err.Error(), &started, &finished)
			_, _ = m.emit(ctx, model.TransferJobEvent{JobID: jobID, ItemIndex: idx, Level: "error", Step: "failed", Message: err.Error(), CreatedAt: finished})
			return
		}
		item.Status = model.JobStatusCompleted
		_ = m.store.UpdateJobItem(ctx, jobID, item)
		_, _ = m.emit(ctx, model.TransferJobEvent{JobID: jobID, ItemIndex: idx, Level: "info", Step: "completed", Message: fmt.Sprintf("completed %s", item.SourceVolume), CreatedAt: time.Now().UTC()})
	}
	finished := time.Now().UTC()
	_ = m.store.UpdateJobStatus(ctx, jobID, model.JobStatusCompleted, "", &started, &finished)
	_, _ = m.emit(ctx, model.TransferJobEvent{JobID: jobID, ItemIndex: -1, Level: "info", Step: "job-completed", Message: "job completed", CreatedAt: finished})
}

func (m *Manager) finishCancelled(ctx context.Context, jobID string, currentIndex int) {
	job, err := m.store.GetJob(ctx, jobID)
	if err == nil {
		for _, item := range job.Items {
			if item.Index >= currentIndex && item.Status == model.JobStatusQueued {
				item.Status = model.JobStatusCancelled
				item.Error = "cancelled"
				_ = m.store.UpdateJobItem(ctx, jobID, item)
			}
		}
	}
	finished := time.Now().UTC()
	_ = m.store.UpdateJobStatus(ctx, jobID, model.JobStatusCancelled, "cancelled", nil, &finished)
	_, _ = m.emit(ctx, model.TransferJobEvent{JobID: jobID, ItemIndex: -1, Level: "warning", Step: "cancelled", Message: "job cancelled", CreatedAt: finished})
}

func (m *Manager) emit(ctx context.Context, event model.TransferJobEvent) (model.TransferJobEvent, error) {
	persisted, err := m.store.AppendEvent(ctx, event)
	if err != nil {
		return model.TransferJobEvent{}, err
	}
	m.mu.RLock()
	subs := make([]chan model.TransferJobEvent, 0, len(m.subs[persisted.JobID]))
	for ch := range m.subs[persisted.JobID] {
		subs = append(subs, ch)
	}
	m.mu.RUnlock()
	for _, ch := range subs {
		select {
		case ch <- persisted:
		default:
		}
	}
	return persisted, nil
}

func (m *Manager) cancelRequested(jobID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cancelled[jobID]
}

func validateJobRequest(req model.CreateTransferJobRequest) error {
	if req.SourceHost == "" {
		return errors.New("sourceHost is required")
	}
	if req.DestinationHost == "" {
		return errors.New("destinationHost is required")
	}
	if req.Operation != model.TransferClone && req.Operation != model.TransferCopy && req.Operation != model.TransferMove {
		return errors.New("operation must be clone, copy, or move")
	}
	if len(req.Items) == 0 {
		return errors.New("at least one transfer item is required")
	}
	seen := map[string]struct{}{}
	for _, item := range req.Items {
		if item.SourceVolume == "" || item.DestinationVolume == "" {
			return errors.New("sourceVolume and destinationVolume are required")
		}
		key := req.DestinationHost + ":" + item.DestinationVolume
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate destination volume %q", item.DestinationVolume)
		}
		seen[key] = struct{}{}
		if req.SourceHost == req.DestinationHost && item.SourceVolume == item.DestinationVolume {
			return fmt.Errorf("source and destination volume must differ for %q on same host", item.SourceVolume)
		}
	}
	return nil
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func SortEvents(events []model.TransferJobEvent) {
	sort.Slice(events, func(i, j int) bool { return events[i].ID < events[j].ID })
}
