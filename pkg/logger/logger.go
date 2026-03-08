package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

type loggerContextKey struct{}

// Logger wraps slog.Logger with additional utilities
type Logger struct {
	logger  *slog.Logger
	outputs map[string]io.Writer
	mu      sync.RWMutex
}

// New creates a new Logger
func New() *Logger {
	return NewWithWriter(os.Stdout)
}

// NewWithWriter creates a new Logger that writes to the provided writer.
func NewWithWriter(w io.Writer) *Logger {
	l := &Logger{
		outputs: make(map[string]io.Writer),
	}
	l.logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	return l
}

// With returns a new Logger with the given attributes
func (l *Logger) With(attrs ...any) *Logger {
	newLogger := &Logger{
		logger:  l.logger.With(attrs...),
		outputs: l.outputs,
	}
	return newLogger
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, args ...any) {
	l.logger.Error(msg, args...)
	os.Exit(1)
}

// StreamProcessOutput streams stdout and stderr from a process in real-time
func StreamProcessOutput(name string, stdout io.Reader, stderr io.Reader, logger *Logger) {
	var wg sync.WaitGroup

	// Stream stdout
	if stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 1024)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					logger.Info(name+" stdout", "output", string(buf[:n]))
				}
				if err != nil {
					break
				}
			}
		}()
	}

	// Stream stderr
	if stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 1024)
			for {
				n, err := stderr.Read(buf)
				if n > 0 {
					logger.Warn(name+" stderr", "output", string(buf[:n]))
				}
				if err != nil {
					break
				}
			}
		}()
	}

	wg.Wait()
}

// StreamWriter creates a writer that logs to the logger
func StreamWriter(name string, isErr bool) io.Writer {
	pr, pw := io.Pipe()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := pr.Read(buf)
			if n > 0 {
				output := string(buf[:n])
				if isErr {
					slog.Warn(name, "output", output)
				} else {
					slog.Info(name, "output", output)
				}
			}
			if err != nil {
				break
			}
		}
	}()

	return pw
}

// Context returns a context with the logger
func (l *Logger) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, l)
}

// FromContext returns the logger from context
func FromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(loggerContextKey{}).(*Logger); ok {
		return l
	}
	return New()
}
