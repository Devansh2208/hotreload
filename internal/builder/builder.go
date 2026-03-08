package builder

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

// Builder handles building the project
type Builder struct {
	logger *logger.Logger
	mu     sync.Mutex
}

// New creates a new Builder
func New(log *logger.Logger) *Builder {
	return &Builder{
		logger: log,
	}
}

// BuildResult represents the result of a build operation
type BuildResult struct {
	Success bool
	Error   error
}

// Build executes the build command with the given context
// If ctx is cancelled, the build will be terminated
func (b *Builder) Build(ctx context.Context, buildCmd string, workDir string) *BuildResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.logger.Info("Starting build", "command", buildCmd, "dir", workDir)

	// Parse the command (simple shell-like parsing)
	parts := strings.Fields(buildCmd)
	if len(parts) == 0 {
		return &BuildResult{
			Success: false,
			Error:   ErrEmptyBuildCommand,
		}
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Configure the process execution to support cross-platform cancellation/cleanup.
	proc.ConfigureForGroup(cmd)

	startTime := time.Now()

	err := cmd.Run()

	elapsed := time.Since(startTime)

	if err != nil {
		if ctx.Err() != nil {
			b.logger.Warn("Build cancelled", "elapsed", elapsed)
			return &BuildResult{
				Success: false,
				Error:   ErrBuildCancelled,
			}
		}
		b.logger.Error("Build failed", "error", err, "elapsed", elapsed)
		return &BuildResult{
			Success: false,
			Error:   err,
		}
	}

	b.logger.Info("Build succeeded", "elapsed", elapsed)
	return &BuildResult{
		Success: true,
		Error:   nil,
	}
}

// BuildAsync executes the build command asynchronously
func (b *Builder) BuildAsync(ctx context.Context, buildCmd string, workDir string, onComplete func(*BuildResult)) {
	go func() {
		result := b.Build(ctx, buildCmd, workDir)
		onComplete(result)
	}()
}

// IsBuildCancelled checks if the error is due to cancellation
func IsBuildCancelled(err error) bool {
	return err == ErrBuildCancelled
}

// Common build errors
var (
	ErrEmptyBuildCommand = &buildError{"empty build command"}
	ErrBuildCancelled    = &buildError{"build was cancelled"}
)

type buildError struct {
	msg string
}

func (e *buildError) Error() string {
	return e.msg
}
