package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hotreload/pkg/logger"
)

func TestBuild_CancelledContext(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	slowBuild := `package main
import "time"
func main() { time.Sleep(3 * time.Second) }
`
	script := filepath.Join(tmp, "slowbuild.go")
	if err := os.WriteFile(script, []byte(slowBuild), 0644); err != nil {
		t.Fatalf("failed writing slow build script: %v", err)
	}

	b := New(logger.NewWithWriter(os.Stderr))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	result := b.Build(ctx, "go run slowbuild.go", tmp)
	if result.Success {
		t.Fatalf("expected cancelled build to fail")
	}
	if !IsBuildCancelled(result.Error) {
		t.Fatalf("expected ErrBuildCancelled, got: %v", result.Error)
	}
}
