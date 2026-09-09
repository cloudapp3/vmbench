package vmbench

import (
	"strings"
	"time"
)

// EventKind is a typed event name emitted by the bench runner.
type EventKind string

const (
	EventCheckupStart    EventKind = "checkup_start"
	EventCheckupProgress EventKind = "checkup_progress"
	EventCheckupDone     EventKind = "checkup_done"
	EventCheckupSkip     EventKind = "checkup_skip"
	EventCheckupFail     EventKind = "checkup_fail"
	EventBenchDone       EventKind = "bench_done"
	EventBenchLog        EventKind = "bench_log"
)

// Event is one state/progress update emitted during RunCore.
type Event struct {
	Kind      EventKind
	Checkup   string
	Workload  string
	Category  string
	Iteration int
	Current   int
	Total     int
	Progress  float64
	Metric    string
	Duration  time.Duration
	Err       error
	Message   string
	Status    string
}

// EventHandler receives run-time events from RunCore.
type EventHandler func(Event)

type stringError string

func (e stringError) Error() string { return string(e) }

func errString(text string) error {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	return stringError(trimmed)
}
