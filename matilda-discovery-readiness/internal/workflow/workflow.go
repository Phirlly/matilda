package workflow

import "time"

type Status string

const (
	StatusStarted   Status = "started"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Event struct {
	Time    string `json:"time"`
	Action  string `json:"action"`
	Stage   string `json:"stage"`
	Check   string `json:"check,omitempty"`
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

type Result struct {
	Action      string  `json:"action"`
	Status      Status  `json:"status"`
	StartedAt   string  `json:"started_at"`
	CompletedAt string  `json:"completed_at,omitempty"`
	Error       string  `json:"error,omitempty"`
	Events      []Event `json:"events"`
}

func Start(action string) Result {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return Result{
		Action:    action,
		Status:    StatusStarted,
		StartedAt: now,
		Events: []Event{
			{Time: now, Action: action, Stage: action, Status: StatusStarted, Message: "started"},
		},
	}
}

func (r *Result) Finish(err error, cancelled bool) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r.CompletedAt = now
	switch {
	case cancelled:
		r.Status = StatusCancelled
		r.Error = err.Error()
	case err != nil:
		r.Status = StatusFailed
		r.Error = err.Error()
	default:
		r.Status = StatusCompleted
	}
	message := "completed"
	if r.Error != "" {
		message = r.Error
	}
	r.Events = append(r.Events, Event{
		Time:    now,
		Action:  r.Action,
		Stage:   r.Action,
		Status:  r.Status,
		Message: message,
	})
}
