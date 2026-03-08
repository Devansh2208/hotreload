package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"hotreload/pkg/logger"
)

func TestWatcher_IsIgnored(t *testing.T) {
	log := logger.New()
	w, err := New(log, Options{})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Stop()

	tests := []struct {
		path     string
		expected bool
	}{
		{".git", true},
		{filepath.Join("project", ".git", "config"), true},
		{"node_modules", true},
		{"vendor", true},
		{"bin", true},
		{"main.go", false},
		{"server.go", false},
	}

	for _, tt := range tests {
		result := w.isIgnored(tt.path)
		if result != tt.expected {
			t.Errorf("isIgnored(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestWatcher_IsTemporaryFile(t *testing.T) {
	log := logger.New()
	w, err := New(log, Options{})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Stop()

	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", false},
		{"main.go.tmp", true},
		{"main.go.swp", true},
		{"main.go~", true},
		{".DS_Store", true},
	}

	for _, tt := range tests {
		result := w.isTemporaryFile(tt.path)
		if result != tt.expected {
			t.Errorf("isTemporaryFile(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestWatcher_IsRelevantFile(t *testing.T) {
	log := logger.New()
	w, err := New(log, Options{})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Stop()

	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"server.go", true},
		{"handler.js", true},
		{"handler.ts", true},
		{"handler.jsx", true},
		{"handler.tsx", true},
		{"main.rs", true},
		{"main.py", true},
		{"README.md", false},
		{"config.json", false},
		{"style.css", false},
	}

	for _, tt := range tests {
		result := w.isRelevantFile(tt.path)
		if result != tt.expected {
			t.Errorf("isRelevantFile(%s) = %v, expected %v", tt.path, result, tt.expected)
		}
	}
}

func TestWatcher_AddRemoveDirectory(t *testing.T) {
	log := logger.New()
	w, err := New(log, Options{})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Stop()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "watcher-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Add directory
	w.addDir(tmpDir)

	// Check it's being watched
	dirs := w.WatchedDirs()
	if len(dirs) != 1 {
		t.Errorf("expected 1 watched dir, got %d", len(dirs))
	}

	// Remove directory
	w.removeDir(tmpDir)

	// Check it's not being watched anymore
	dirs = w.WatchedDirs()
	if len(dirs) != 0 {
		t.Errorf("expected 0 watched dirs, got %d", len(dirs))
	}
}

func TestWatcher_EventProcessing(t *testing.T) {
	log := logger.New()
	w, err := New(log, Options{})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Stop()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "watcher-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Start watcher
	err = w.Start(tmpDir)
	if err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// Create a Go file
	testFile := filepath.Join(tmpDir, "test.go")
	err = os.WriteFile(testFile, []byte("package main"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	defer os.Remove(testFile)

	// Wait for event
	select {
	case path := <-w.Events():
		if path != testFile {
			t.Errorf("expected event for %s, got %s", testFile, path)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for file event")
	}
}
