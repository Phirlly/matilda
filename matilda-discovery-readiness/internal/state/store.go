package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

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
