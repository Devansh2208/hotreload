package debounce

import (
	"sync"
	"testing"
	"time"
)

func TestDebouncer_Trigger(t *testing.T) {
	d := New(100 * time.Millisecond)
	defer d.Stop()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		<-d.Channel()
	}()

	// Trigger multiple times rapidly
	for i := 0; i < 5; i++ {
		d.Trigger()
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()
}

func TestDebouncer_SingleEvent(t *testing.T) {
	d := New(50 * time.Millisecond)
	defer d.Stop()

	d.Trigger()

	select {
	case <-d.Channel():
		// Expected
	case <-time.After(200 * time.Millisecond):
		t.Error("expected debounce to fire within timeout")
	}
}

func TestDebouncer_CancelPrevious(t *testing.T) {
	d := New(200 * time.Millisecond)
	defer d.Stop()

	// Trigger first
	d.Trigger()

	// Immediately trigger again (should cancel first)
	d.Trigger()

	start := time.Now()

	select {
	case <-d.Channel():
		elapsed := time.Since(start)
		// Should fire around 200ms from last trigger, not 100ms
		if elapsed < 150*time.Millisecond {
			t.Errorf("debounce fired too early: %v", elapsed)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("expected debounce to fire within timeout")
	}
}

func TestDebouncer_MultipleTriggers(t *testing.T) {
	d := New(100 * time.Millisecond)
	defer d.Stop()

	triggerCount := 0
	var mu sync.Mutex

	// Start listener
	go func() {
		for range d.Channel() {
			mu.Lock()
			triggerCount++
			mu.Unlock()
		}
	}()

	// Trigger 10 times rapidly
	for i := 0; i < 10; i++ {
		d.Trigger()
	}

	// Wait for debounce to fire
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if triggerCount != 1 {
		t.Errorf("expected exactly 1 trigger, got %d", triggerCount)
	}
	mu.Unlock()
}

func TestDebouncer_Stop(t *testing.T) {
	d := New(1 * time.Second)
	defer d.Stop()

	d.Trigger()

	// Stop immediately
	d.Stop()

	select {
	case <-d.Channel():
		t.Error("debounce should not fire after Stop")
	case <-time.After(100 * time.Millisecond):
		// Expected - should not fire
	}
}
