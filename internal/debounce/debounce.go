package debounce

import (
	"sync"
	"time"
)

// Debouncer implements debouncing for file change events
type Debouncer struct {
	channel chan struct{}
	delay   time.Duration
	muTimer sync.Mutex
	timer   *time.Timer
}

// New creates a new Debouncer with the specified delay
func New(delay time.Duration) *Debouncer {
	return &Debouncer{
		channel: make(chan struct{}, 1),
		delay:   delay,
	}
}

// Channel returns the channel that signals when debounce period has passed
func (d *Debouncer) Channel() <-chan struct{} {
	return d.channel
}

// Trigger triggers a new debounce cycle, canceling any pending one
func (d *Debouncer) Trigger() {
	d.muTimer.Lock()
	defer d.muTimer.Unlock()

	// Cancel existing timer if any
	if d.timer != nil {
		d.timer.Stop()
		// Drain the timer channel if it fired
		select {
		case <-d.timer.C:
		default:
		}
	}

	// Start new timer
	d.timer = time.AfterFunc(d.delay, func() {
		select {
		case d.channel <- struct{}{}:
		default:
		}
	})
}

// Stop stops the debouncer and cleans up resources
func (d *Debouncer) Stop() {
	d.muTimer.Lock()
	defer d.muTimer.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		select {
		case <-d.timer.C:
		default:
		}
	}
}
