package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"matilda-discovery-readiness/internal/workflow"
)

var ErrNotFound = errors.New("state not found")

type Store struct {
	root string
}

type Document struct {
	Version     int                    `json:"version"`
	Workspace   string                 `json:"workspace"`
	Inventory   string                 `json:"inventory"`
	LastAction  string                 `json:"last_action,omitempty"`
	LastStatus  workflow.Status        `json:"last_status,omitempty"`
	LastError   string                 `json:"last_error,omitempty"`
	CompletedAt string                 `json:"completed_at,omitempty"`
	Actions     map[string]ActionState `json:"actions,omitempty"`
	Readiness   ReadinessState         `json:"readiness"`
	Reports     ReportState            `json:"reports"`
}

type ActionState struct {
	Status      workflow.Status `json:"status"`
	CompletedAt string          `json:"completed_at,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type ReadinessState struct {
	Total    int `json:"total"`
	Ready    int `json:"ready"`
	NotReady int `json:"not_ready"`
}

type ReportState struct {
	LatestHTML     string `json:"latest_html,omitempty"`
	LatestJSON     string `json:"latest_json,omitempty"`
	LatestMarkdown string `json:"latest_markdown,omitempty"`
	LatestCSV      string `json:"latest_csv,omitempty"`
	ValidatedIPs   string `json:"validated_ips,omitempty"`
}

type RunRecord struct {
	ID                string          `json:"id"`
	Action            string          `json:"action"`
	Status            workflow.Status `json:"status"`
	StartedAt         string          `json:"started_at"`
	EndedAt           string          `json:"ended_at,omitempty"`
	Command           string          `json:"command,omitempty"`
	ReadinessTotal    int             `json:"readiness_total"`
	ReadinessReady    int             `json:"readiness_ready"`
	ReadinessNotReady int             `json:"readiness_not_ready"`
	ReportPaths       []string        `json:"report_paths,omitempty"`
	Summary           string          `json:"summary,omitempty"`
	Error             string          `json:"error,omitempty"`
}

type Update struct {
	Workspace string
	Inventory string
	Result    workflow.Result
	Readiness ReadinessState
	Reports   ReportState
}

func New(root string) Store {
	return Store{root: root}
}

func (s Store) Path() string {
	return filepath.Join(s.root, ".matilda", "state.json")
}

func (s Store) RunsDir() string {
	return filepath.Join(s.root, ".matilda", "runs")
}

func (s Store) Read() (Document, error) {
	path := s.Path()
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Document{Version: 1, Workspace: s.root}, ErrNotFound
		}
		return Document{}, err
	}
	var doc Document
	if err := json.Unmarshal(content, &doc); err != nil {
		return Document{}, err
	}
	if doc.Actions == nil {
		doc.Actions = map[string]ActionState{}
	}
	return doc, nil
}

func (s Store) WriteRun(record RunRecord) error {
	if record.ID == "" {
		return errors.New("run record id is required")
	}
	if err := os.MkdirAll(s.RunsDir(), 0700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(filepath.Join(s.RunsDir(), record.ID+".json"), content, 0600)
}

func (s Store) ListRuns(limit int) ([]RunRecord, error) {
	entries, err := os.ReadDir(s.RunsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})
	var records []RunRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(s.RunsDir(), entry.Name()))
		if err != nil {
			return nil, err
		}
		var record RunRecord
		if err := json.Unmarshal(content, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
		if limit > 0 && len(records) >= limit {
			break
		}
	}
	return records, nil
}

func (s Store) Update(update Update) (Document, error) {
	doc, err := s.Read()
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Document{}, err
	}
	if doc.Actions == nil {
		doc.Actions = map[string]ActionState{}
	}
	doc.Version = 1
	doc.Workspace = update.Workspace
	doc.Inventory = update.Inventory
	doc.LastAction = update.Result.Action
	doc.LastStatus = update.Result.Status
	doc.LastError = update.Result.Error
	doc.CompletedAt = update.Result.CompletedAt
	doc.Actions[update.Result.Action] = ActionState{
		Status:      update.Result.Status,
		CompletedAt: update.Result.CompletedAt,
		Error:       update.Result.Error,
	}
	doc.Readiness = update.Readiness
	doc.Reports = update.Reports

	if err := os.MkdirAll(filepath.Dir(s.Path()), 0700); err != nil {
		return Document{}, err
	}
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return Document{}, err
	}
	content = append(content, '\n')
	if err := os.WriteFile(s.Path(), content, 0600); err != nil {
		return Document{}, err
	}
	return doc, nil
}
