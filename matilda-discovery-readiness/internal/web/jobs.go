package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"matilda-discovery-readiness/internal/app"
)

const maxJobOutputBytes = 1 << 20

var (
	errJobRunning          = errors.New("another browser action is already running")
	errJobNotFound         = errors.New("browser action job not found")
	errUnknownAction       = errors.New("unknown workflow action")
	errConfirmationMissing = errors.New("action requires confirmation before changing targets")
)

type jobStatus string

const (
	jobRunning   jobStatus = "running"
	jobCompleted jobStatus = "completed"
	jobFailed    jobStatus = "failed"
	jobCancelled jobStatus = "cancelled"
)

type jobSnapshot struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Status    jobStatus `json:"status"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

type jobEvent struct {
	JobID  string    `json:"job_id"`
	Action string    `json:"action"`
	Status jobStatus `json:"status"`
	Text   string    `json:"text,omitempty"`
	Error  string    `json:"error,omitempty"`
}

type browserJob struct {
	id          string
	action      app.ActionSpec
	status      jobStatus
	output      string
	errText     string
	startedAt   time.Time
	endedAt     time.Time
	cancel      context.CancelFunc
	subscribers map[chan jobEvent]struct{}
}

type jobManager struct {
	mu     sync.Mutex
	rt     *app.Runtime
	jobs   map[string]*browserJob
	active string
	nextID int64
}

func newJobManager(rt *app.Runtime) *jobManager {
	return &jobManager{
		rt:   rt,
		jobs: map[string]*browserJob{},
	}
}

func (m *jobManager) Start(actionID string, confirmed bool) (jobSnapshot, error) {
	action, ok := app.WorkflowActionByID(actionID)
	if !ok {
		return jobSnapshot{}, errUnknownAction
	}
	if action.Mutating && !confirmed {
		return jobSnapshot{}, errConfirmationMissing
	}

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now().UTC()

	m.mu.Lock()
	if m.active != "" {
		if active, ok := m.jobs[m.active]; ok && active.status == jobRunning {
			m.mu.Unlock()
			cancel()
			return jobSnapshot{}, errJobRunning
		}
		m.active = ""
	}
	m.nextID++
	id := fmt.Sprintf("job-%d-%d", now.UnixNano(), m.nextID)
	job := &browserJob{
		id:          id,
		action:      action,
		status:      jobRunning,
		startedAt:   now,
		cancel:      cancel,
		subscribers: map[chan jobEvent]struct{}{},
	}
	m.jobs[id] = job
	m.active = id
	snapshot := job.snapshot()
	m.mu.Unlock()

	go m.run(ctx, job, confirmed)
	return snapshot, nil
}

func (m *jobManager) Snapshot(id string) (jobSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return jobSnapshot{}, false
	}
	return job.snapshot(), true
}

func (m *jobManager) Subscribe(id string) (jobSnapshot, <-chan jobEvent, func(), bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return jobSnapshot{}, nil, func() {}, false
	}
	snapshot := job.snapshot()
	if job.status != jobRunning {
		return snapshot, nil, func() {}, true
	}
	ch := make(chan jobEvent, 64)
	job.subscribers[ch] = struct{}{}
	unsubscribe := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if current, ok := m.jobs[id]; ok {
			delete(current.subscribers, ch)
			close(ch)
		}
	}
	return snapshot, ch, unsubscribe, true
}

func (m *jobManager) Cancel(id string) (jobSnapshot, error) {
	m.mu.Lock()
	job, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return jobSnapshot{}, errJobNotFound
	}
	if job.status != jobRunning {
		snapshot := job.snapshot()
		m.mu.Unlock()
		return snapshot, nil
	}
	cancel := job.cancel
	snapshot := job.snapshot()
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	m.appendOutput(id, "\nCancellation requested. Waiting for the current command to stop.\n")
	return snapshot, nil
}

func (m *jobManager) run(ctx context.Context, job *browserJob, confirmed bool) {
	writer := jobStreamWriter{manager: m, jobID: job.id}
	result := m.rt.WithContext(ctx).RunWorkflowActionTo(job.action.ID, confirmed, writer, writer)

	status := jobCompleted
	if !result.OK {
		status = jobFailed
	}
	if result.Error == app.ErrCancelled.Error() {
		status = jobCancelled
	}
	m.finish(job.id, status, result.Error)
}

func (m *jobManager) appendOutput(id string, text string) {
	if text == "" {
		return
	}
	var subscribers []chan jobEvent
	var event jobEvent

	m.mu.Lock()
	job, ok := m.jobs[id]
	if ok {
		job.output = appendBoundedOutput(job.output, text)
		event = jobEvent{JobID: job.id, Action: job.action.ID, Status: job.status, Text: text}
		for ch := range job.subscribers {
			subscribers = append(subscribers, ch)
		}
	}
	m.mu.Unlock()

	if !ok {
		return
	}
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (m *jobManager) finish(id string, status jobStatus, errText string) {
	var subscribers []chan jobEvent
	var event jobEvent

	m.mu.Lock()
	job, ok := m.jobs[id]
	if ok {
		job.status = status
		job.errText = errText
		job.endedAt = time.Now().UTC()
		if m.active == id {
			m.active = ""
		}
		event = jobEvent{JobID: job.id, Action: job.action.ID, Status: job.status, Error: errText}
		for ch := range job.subscribers {
			subscribers = append(subscribers, ch)
		}
	}
	m.mu.Unlock()

	if !ok {
		return
	}
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (j *browserJob) snapshot() jobSnapshot {
	return jobSnapshot{
		ID:        j.id,
		Action:    j.action.ID,
		Status:    j.status,
		Output:    j.output,
		Error:     j.errText,
		StartedAt: j.startedAt,
		EndedAt:   j.endedAt,
	}
}

type jobStreamWriter struct {
	manager *jobManager
	jobID   string
}

func (w jobStreamWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.manager.appendOutput(w.jobID, string(append([]byte(nil), p...)))
	return len(p), nil
}

func appendBoundedOutput(current string, text string) string {
	if len(current)+len(text) <= maxJobOutputBytes {
		return current + text
	}
	marker := "\n[earlier output truncated]\n"
	combined := current + text
	keep := maxJobOutputBytes - len(marker)
	if keep < 0 {
		keep = maxJobOutputBytes
		marker = ""
	}
	if len(combined) <= keep {
		return marker + combined
	}
	return marker + combined[len(combined)-keep:]
}

func finalEventName(status jobStatus) string {
	switch status {
	case jobCompleted:
		return "completed"
	case jobCancelled:
		return "cancelled"
	default:
		return "failed"
	}
}

var _ io.Writer = jobStreamWriter{}
