package runner

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"hotreload/internal/proc"
	"hotreload/pkg/logger"
)

// Runner handles running the built server process
type Runner struct {
	logger         *logger.Logger
	mu             sync.Mutex
	process        *exec.Cmd
	stopChan       chan struct{}
	wg             sync.WaitGroup
	restartBackoff time.Duration
	maxBackoff     time.Duration
	isRunning      bool
	startTime      time.Time
}

// New creates a new Runner
func New(log *logger.Logger) *Runner {
	return &Runner{
		logger:         log,
		stopChan:       make(chan struct{}),
		restartBackoff: 100 * time.Millisecond,
		maxBackoff:     5 * time.Second,
		isRunning:      false,
	}
}

// RunResult represents the result of a run operation
type RunResult struct {
	Success bool
	Error   error
}

// Run starts the server process
func (r *Runner) Run(ctx context.Context, execCmd string, workDir string) *RunResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if we need to apply backoff due to rapid restarts
	if r.isRunning && r.startTime.Add(r.restartBackoff).After(time.Now()) {
		r.logger.Info("Applying restart backoff", "backoff", r.restartBackoff)
		select {
		case <-time.After(r.restartBackoff):
		case <-ctx.Done():
			return &RunResult{Success: false, Error: ctx.Err()}
		}

		// Exponential backoff
		r.restartBackoff = min(r.restartBackoff*2, r.maxBackoff)
	} else {
		r.restartBackoff = 100 * time.Millisecond
	}

	// Kill existing process if any
	if r.process != nil && r.process.Process != nil {
		r.killProcessTree(r.process.Process.Pid)
	}

	r.logger.Info("Starting server", "command", execCmd, "dir", workDir)

	// Parse command
	parts := strings.Fields(execCmd)
	if len(parts) == 0 {
		return &RunResult{
			Success: false,
			Error:   ErrEmptyExecCommand,
		}
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Configure process execution to support cross-platform restarts.
	proc.ConfigureForGroup(cmd)

	if err := cmd.Start(); err != nil {
		r.logger.Error("Failed to start server", "error", err)
		return &RunResult{
			Success: false,
			Error:   err,
		}
	}

	r.process = cmd
	r.isRunning = true
	r.startTime = time.Now()

	// Start goroutine to wait for process
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		err := cmd.Wait()
		r.mu.Lock()
		r.isRunning = false
		if r.process == cmd {
			r.process = nil
		}
		r.mu.Unlock()

		if err != nil {
			r.logger.Warn("Server process exited", "error", err)
		} else {
			r.logger.Info("Server process exited normally")
		}
	}()

	r.logger.Info("Server started", "pid", cmd.Process.Pid)
	return &RunResult{
		Success: true,
		Error:   nil,
	}
}

// Stop stops the server process
func (r *Runner) Stop() error {
	return r.StopWithTimeout(2 * time.Second)
}

// StopWithTimeout attempts graceful shutdown first, then force-kills on timeout.
func (r *Runner) StopWithTimeout(timeout time.Duration) error {
	r.mu.Lock()
	if r.process == nil || r.process.Process == nil {
		r.mu.Unlock()
		return nil
	}
	cmd := r.process
	pid := cmd.Process.Pid
	r.mu.Unlock()

	r.logger.Info("Stopping server", "pid", pid, "graceful_timeout", timeout)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		r.logger.Warn("Graceful signal failed, forcing kill", "pid", pid, "error", err)
		return r.killProcessTree(pid)
	}

	deadline := time.Now().Add(timeout)
	select {
	default:
	}

	for time.Now().Before(deadline) {
		r.mu.Lock()
		stillRunning := r.isRunning
		r.mu.Unlock()
		if !stillRunning {
			r.logger.Info("Server stopped gracefully", "pid", pid)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	r.logger.Warn("Graceful shutdown timed out, force-killing server", "pid", pid)
	if err := r.killProcessTree(pid); err != nil {
		return err
	}
	r.mu.Lock()
	r.isRunning = false
	r.process = nil
	r.mu.Unlock()
	return nil
}

// KillProcessTree kills the process and all its children
func (r *Runner) killProcessTree(pid int) error {
	if err := proc.KillTree(pid); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

// IsRunning returns whether the server is currently running
func (r *Runner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRunning
}

// Wait waits for the server process to exit
func (r *Runner) Wait() {
	r.wg.Wait()
}

// GetPID returns the PID of the running process, or -1 if not running
func (r *Runner) GetPID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.process != nil && r.process.Process != nil {
		return r.process.Process.Pid
	}
	return -1
}

// Common errors
var (
	ErrEmptyExecCommand = &runnerError{"empty exec command"}
)

type runnerError struct {
	msg string
}

func (e *runnerError) Error() string {
	return e.msg
}

// min returns the minimum of two durations
func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
