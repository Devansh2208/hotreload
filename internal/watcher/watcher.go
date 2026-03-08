package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"

	"hotreload/pkg/logger"
)

// Watcher wraps fsnotify for file system watching
type Watcher struct {
	logger      *logger.Logger
	fsWatcher   *fsnotify.Watcher
	watchedDirs map[string]bool
	mu          sync.RWMutex
	eventChan   chan string
	stopChan    chan struct{}
	wg          sync.WaitGroup
	ignoredDirs map[string]bool
	extensions  map[string]bool
}

// Options controls watcher behavior.
type Options struct {
	IgnoreDirs []string
	Extensions []string
}

// New creates a new Watcher
func New(log *logger.Logger, opts Options) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		logger:      log,
		fsWatcher:   fsWatcher,
		watchedDirs: make(map[string]bool),
		eventChan:   make(chan string, 100),
		stopChan:    make(chan struct{}),
		ignoredDirs: make(map[string]bool),
		extensions:  make(map[string]bool),
	}

	defaultIgnored := []string{
		".git", "node_modules", "vendor", "bin", "dist", ".cache", ".idea", ".vscode",
	}
	if len(opts.IgnoreDirs) == 0 {
		opts.IgnoreDirs = defaultIgnored
	}

	defaultExts := []string{
		".go", ".c", ".cpp", ".h", ".hpp", ".rs", ".py", ".js", ".ts", ".jsx", ".tsx",
	}
	if len(opts.Extensions) == 0 {
		opts.Extensions = defaultExts
	}

	for _, dir := range opts.IgnoreDirs {
		w.ignoredDirs[dir] = true
	}
	for _, ext := range opts.Extensions {
		w.extensions[strings.ToLower(ext)] = true
	}

	return w, nil
}

// Start watching a root directory recursively
func (w *Watcher) Start(root string) error {
	w.logger.Info("Starting watcher", "root", root)

	// Walk directory tree and add all directories
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip ignored directories
		if info.IsDir() && w.isIgnored(path) {
			return filepath.SkipDir
		}

		if info.IsDir() {
			w.addDir(path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory tree: %w", err)
	}

	// Start event processor
	w.wg.Add(1)
	go w.processEvents()

	return nil
}

// addDir adds a directory to the watcher
func (w *Watcher) addDir(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.watchedDirs[path] {
		return
	}

	if err := w.fsWatcher.Add(path); err != nil {
		w.logger.Warn("Failed to watch directory", "path", path, "error", err)
		return
	}

	w.watchedDirs[path] = true
	w.logger.Debug("Watching directory", "path", path)
}

// removeDir removes a directory from the watcher
func (w *Watcher) removeDir(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.watchedDirs[path] {
		return
	}

	if err := w.fsWatcher.Remove(path); err != nil {
		w.logger.Warn("Failed to stop watching directory", "path", path, "error", err)
		return
	}

	delete(w.watchedDirs, path)
	w.logger.Debug("Stopped watching directory", "path", path)
}

// isIgnored checks if a path should be ignored
func (w *Watcher) isIgnored(path string) bool {
	clean := filepath.Clean(path)
	parts := strings.Split(clean, string(os.PathSeparator))
	for _, part := range parts {
		if w.ignoredDirs[part] {
			return true
		}
	}
	return false
}

// Events returns the channel of file change events
func (w *Watcher) Events() <-chan string {
	return w.eventChan
}

// processEvents processes fsnotify events
func (w *Watcher) processEvents() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stopChan:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			w.handleEvent(event)
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("Watcher error", "error", err)
		}
	}
}

// handleEvent handles a single fsnotify event
func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Only watch for create, write, remove, rename
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
		return
	}

	path := event.Name

	// Skip ignored directories
	if w.isIgnored(path) {
		return
	}

	// Handle directory creation
	if event.Op&fsnotify.Create != 0 {
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			w.addDir(path)
		}
	}

	// Handle directory removal
	if event.Op&fsnotify.Remove != 0 {
		w.removeDir(path)
	}

	// Check if it's a source file we care about
	if w.isTemporaryFile(path) || !w.isRelevantFile(path) {
		return
	}

	w.logger.Debug("File change detected", "path", path, "op", event.Op)

	// Send event (non-blocking)
	select {
	case w.eventChan <- path:
	default:
		w.logger.Warn("Event channel full, dropping event")
	}
}

// isRelevantFile checks if the file is a source file we should watch
func (w *Watcher) isRelevantFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return w.extensions[ext]
}

func (w *Watcher) isTemporaryFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(name, ".#") || strings.HasPrefix(name, "#") {
		return true
	}
	if strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".swo") || strings.HasSuffix(name, ".bak") || strings.HasSuffix(name, ".temp") {
		return true
	}
	switch name {
	case ".ds_store", "thumbs.db":
		return true
	}
	return false
}

// Stop stops the watcher
func (w *Watcher) Stop() error {
	close(w.stopChan)
	w.wg.Wait()

	if err := w.fsWatcher.Close(); err != nil {
		return fmt.Errorf("failed to close watcher: %w", err)
	}

	w.logger.Info("Watcher stopped")
	return nil
}

// WatchedDirs returns the list of watched directories
func (w *Watcher) WatchedDirs() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	dirs := make([]string, 0, len(w.watchedDirs))
	for dir := range w.watchedDirs {
		dirs = append(dirs, dir)
	}
	return dirs
}
