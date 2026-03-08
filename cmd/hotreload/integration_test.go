package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHotreload_RebuildsOnFileChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	projectDir := createTempServerProject(t)
	port := freePort(t)

	binaryPath := filepath.Join(t.TempDir(), "hotreload-test-bin")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = "."
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build hotreload binary: %v\n%s", err, string(out))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	args := []string{
		"--root", projectDir,
		"--build", "go build -o ./server.exe .",
		"--exec", "./server.exe",
		"--debounce", "150ms",
	}
	hotreloadCmd := exec.CommandContext(ctx, binaryPath, args...)
	hotreloadCmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))

	sb := &safeBuffer{}
	hotreloadCmd.Stdout = sb
	hotreloadCmd.Stderr = sb

	if err := hotreloadCmd.Start(); err != nil {
		t.Fatalf("failed to start hotreload: %v", err)
	}
	defer func() {
		if hotreloadCmd.Process != nil {
			_ = hotreloadCmd.Process.Signal(os.Interrupt)
		}
		waitDone := make(chan struct{})
		go func() {
			_ = hotreloadCmd.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(3 * time.Second):
			if hotreloadCmd.Process != nil {
				_ = hotreloadCmd.Process.Kill()
			}
			<-waitDone
		}
		_, _ = io.Copy(io.Discard, bytes.NewBufferString(sb.String()))
	}()

	if err := waitForHTTP(port, "/health", 12*time.Second); err != nil {
		t.Fatalf("server did not become healthy: %v\nlogs:\n%s", err, sb.String())
	}
	if err := waitForLogOccurrences(sb, "Server started", 1, 6*time.Second); err != nil {
		t.Fatalf("did not observe initial server start: %v\nlogs:\n%s", err, sb.String())
	}

	mainFile := filepath.Join(projectDir, "main.go")
	content, err := os.ReadFile(mainFile)
	if err != nil {
		t.Fatalf("failed reading main.go: %v", err)
	}
	updated := string(content) + fmt.Sprintf("\n// change-marker-%d\n", time.Now().UnixNano())
	if err := os.WriteFile(mainFile, []byte(updated), 0644); err != nil {
		t.Fatalf("failed updating main.go: %v", err)
	}

	if err := waitForLogOccurrences(sb, "Build succeeded", 1, 15*time.Second); err != nil {
		t.Fatalf("did not observe rebuild: %v\nlogs:\n%s", err, sb.String())
	}
	if err := waitForLogOccurrences(sb, "Server started", 2, 15*time.Second); err != nil {
		t.Fatalf("did not observe server restart: %v\nlogs:\n%s", err, sb.String())
	}
}

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func createTempServerProject(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()

	goMod := "module e2eproject\n\ngo 1.21\n"
	mainGo := `package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "18080"
	}
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "OK")
	})
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "hello")
	})
	_ = http.ListenAndServe(":"+port, nil)
}
`
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed writing go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed writing main.go: %v", err)
	}
	return tmp
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForHTTP(port int, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) // #nosec G107 - local integration test endpoint
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", url)
}

func waitForLogOccurrences(sb *safeBuffer, marker string, atLeast int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		logs := sb.String()
		if strings.Count(logs, marker) >= atLeast {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for marker %q at least %d times", marker, atLeast)
}
