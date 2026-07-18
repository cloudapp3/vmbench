package vmbench

import (
	"strings"
	"time"
)

// EventKind is a typed event name emitted by the bench runner.
type EventKind string

const (
	EventSuiteStart    EventKind = "suite_start"
	EventSuiteProgress EventKind = "suite_progress"
	EventSuiteDone     EventKind = "suite_done"
	EventSuiteSkip     EventKind = "suite_skip"
	EventSuiteFail     EventKind = "suite_fail"
	EventBenchDone     EventKind = "bench_done"
	EventBenchLog      EventKind = "bench_log"
)

// Event is one state/progress update emitted during RunCore.
type Event struct {
	Kind      EventKind
	Suite     string
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
