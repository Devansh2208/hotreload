package dashboard

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Snapshot is the current dashboard state.
type Snapshot struct {
	StartedAt          time.Time
	State              string
	Root               string
	LastChange         string
	LastBuildDuration  time.Duration
	Rebuilds           int
	SuccessfulRebuilds int
	PID                int
	LastError          string
}

// Dashboard renders a live status view in the terminal.
type Dashboard struct {
	mu     sync.RWMutex
	state  Snapshot
	ticker *time.Ticker
	done   chan struct{}
}

// New creates a dashboard with an initial snapshot.
func New(initial Snapshot) *Dashboard {
	return &Dashboard{
		state: initial,
		done:  make(chan struct{}),
	}
}

// Start begins periodic rendering.
func (d *Dashboard) Start() {
	d.ticker = time.NewTicker(250 * time.Millisecond)
	d.render()
	go func() {
		for {
			select {
			case <-d.done:
				d.render()
				return
			case <-d.ticker.C:
				d.render()
			}
		}
	}()
}

// Stop ends rendering.
func (d *Dashboard) Stop() {
	if d.ticker != nil {
		d.ticker.Stop()
	}
	close(d.done)
}

// Update applies a mutation to the current snapshot.
func (d *Dashboard) Update(mutator func(*Snapshot)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	mutator(&d.state)
}

func (d *Dashboard) render() {
	d.mu.RLock()
	s := d.state
	d.mu.RUnlock()

	uptime := time.Since(s.StartedAt).Truncate(time.Second)
	if s.StartedAt.IsZero() {
		uptime = 0
	}

	fmt.Fprint(os.Stdout, "\033[2J\033[H")
	fmt.Fprintln(os.Stdout, "hotreload --ui")
	fmt.Fprintln(os.Stdout, "----------------")
	fmt.Fprintf(os.Stdout, "State:           %s\n", s.State)
	fmt.Fprintf(os.Stdout, "Root:            %s\n", s.Root)
	fmt.Fprintf(os.Stdout, "PID:             %d\n", s.PID)
	fmt.Fprintf(os.Stdout, "Uptime:          %s\n", uptime)
	fmt.Fprintf(os.Stdout, "Rebuilds:        %d (success: %d)\n", s.Rebuilds, s.SuccessfulRebuilds)
	fmt.Fprintf(os.Stdout, "Last Build:      %s\n", s.LastBuildDuration.Truncate(time.Millisecond))
	fmt.Fprintf(os.Stdout, "Last Change:     %s\n", valueOrDash(s.LastChange))
	fmt.Fprintf(os.Stdout, "Last Error:      %s\n", valueOrDash(s.LastError))
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Press Ctrl+C to stop.")
}

func valueOrDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
