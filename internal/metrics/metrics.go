package metrics

import (
	"sync"
	"time"
)

// Snapshot captures runtime metrics.
type Snapshot struct {
	StartedAt          time.Time
	Events             int
	Rebuilds           int
	SuccessfulRebuilds int
	FailedBuilds       int
	LastBuildDuration  time.Duration
	LastChangedFile    string
	LastError          string
	ServerPID          int
}

// RuntimeMetrics collects counters for observability.
type RuntimeMetrics struct {
	mu sync.RWMutex
	s  Snapshot
}

// New creates initialized metrics.
func New() *RuntimeMetrics {
	return &RuntimeMetrics{
		s: Snapshot{
			StartedAt: time.Now(),
			ServerPID: -1,
		},
	}
}

// OnEvent records a file change event.
func (m *RuntimeMetrics) OnEvent(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Events++
	m.s.LastChangedFile = path
}

// OnBuildStarted records a rebuild attempt.
func (m *RuntimeMetrics) OnBuildStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Rebuilds++
}

// OnBuildSuccess records a successful build.
func (m *RuntimeMetrics) OnBuildSuccess(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.SuccessfulRebuilds++
	m.s.LastBuildDuration = d
	m.s.LastError = ""
}

// OnBuildFailed records a failed build.
func (m *RuntimeMetrics) OnBuildFailed(err error, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.FailedBuilds++
	m.s.LastBuildDuration = d
	if err != nil {
		m.s.LastError = err.Error()
	}
}

// SetPID updates server process ID.
func (m *RuntimeMetrics) SetPID(pid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.ServerPID = pid
}

// Snapshot returns a copy of current metrics.
func (m *RuntimeMetrics) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}
