package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"hotreload/internal/builder"
	"hotreload/internal/config"
	"hotreload/internal/dashboard"
	"hotreload/internal/debounce"
	"hotreload/internal/metrics"
	"hotreload/internal/runner"
	"hotreload/internal/watcher"
	"hotreload/pkg/logger"
)

var version = "dev"

func main() {
	// Parse configuration
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse config: %v\n", err)
		os.Exit(1)
	}
	if cfg.Version {
		fmt.Printf("hotreload %s\n", version)
		return
	}
	if cfg.Status {
		printResolvedConfig(cfg)
		return
	}

	logOut := io.Writer(os.Stdout)
	if cfg.UI {
		logOut = io.Discard
	}
	log := logger.NewWithWriter(logOut)

	if err := cfg.Validate(true); err != nil {
		log.Error("Invalid config", "error", err)
		os.Exit(1)
	}
	if cfg.DryRun {
		log.Info("Dry-run mode: configuration validated", "root", cfg.Root)
		printResolvedConfig(cfg)
		return
	}

	log.Info("Hotreload starting", "root", cfg.Root, "build", cfg.Build, "exec", cfg.Exec)

	// Create components
	b := builder.New(log)
	r := runner.New(log)
	d := debounce.New(cfg.Debounce)

	// Create file watcher
	watcher, err := watcher.New(log, watcher.Options{
		IgnoreDirs: cfg.IgnoreDirs,
		Extensions: cfg.Extensions,
	})
	if err != nil {
		log.Error("Failed to create watcher", "error", err)
		os.Exit(1)
	}

	// Start watching
	if err := watcher.Start(cfg.Root); err != nil {
		log.Error("Failed to start watcher", "error", err)
		os.Exit(1)
	}

	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build context - will be cancelled on new file changes
	buildCtx, buildCancel := context.WithCancel(ctx)
	buildMu := sync.Mutex{}
	buildInProgress := false
	var lastChangedFile string
	rebuilds := 0
	successfulRebuilds := 0
	met := metrics.New()

	var ui *dashboard.Dashboard
	if cfg.UI {
		ui = dashboard.New(dashboard.Snapshot{
			StartedAt: time.Now(),
			State:     "initializing",
			Root:      cfg.Root,
			PID:       -1,
		})
		ui.Start()
		defer ui.Stop()
	}

	updateUI := func(mutator func(*dashboard.Snapshot)) {
		if ui != nil {
			ui.Update(mutator)
		}
	}

	if cfg.SummaryInterval > 0 {
		ticker := time.NewTicker(cfg.SummaryInterval)
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s := met.Snapshot()
					log.Info(
						"Runtime summary",
						"uptime", time.Since(s.StartedAt).Truncate(time.Second),
						"events", s.Events,
						"rebuilds", s.Rebuilds,
						"successful_rebuilds", s.SuccessfulRebuilds,
						"failed_builds", s.FailedBuilds,
						"last_change", s.LastChangedFile,
						"last_build_duration", s.LastBuildDuration,
						"pid", s.ServerPID,
					)
				}
			}
		}()
	}

	// Initial build
	log.Info("Performing initial build...")
	updateUI(func(s *dashboard.Snapshot) {
		s.State = "initial build"
	})
	initialResult := b.Build(buildCtx, cfg.Build, cfg.Root)
	if !initialResult.Success {
		log.Error("Initial build failed", "error", initialResult.Error)
		updateUI(func(s *dashboard.Snapshot) {
			s.State = "build failed"
			s.LastError = initialResult.Error.Error()
		})
		os.Exit(1)
	}

	// Start the server after successful build
	updateUI(func(s *dashboard.Snapshot) {
		s.State = "starting server"
		s.LastError = ""
	})
	runResult := r.Run(ctx, cfg.Exec, cfg.Root)
	if !runResult.Success {
		log.Error("Failed to start server", "error", runResult.Error)
		updateUI(func(s *dashboard.Snapshot) {
			s.State = "start failed"
			s.LastError = runResult.Error.Error()
		})
		os.Exit(1)
	}
	updateUI(func(s *dashboard.Snapshot) {
		s.State = "running"
		s.PID = r.GetPID()
	})
	met.SetPID(r.GetPID())

	// Main event loop
	go func() {
		for {
			select {
			case changedPath := <-watcher.Events():
				lastChangedFile = changedPath
				met.OnEvent(changedPath)
				updateUI(func(s *dashboard.Snapshot) {
					s.LastChange = changedPath
					s.State = "changes detected"
				})
				// Trigger debounce
				d.Trigger()
			case <-d.Channel():
				rebuilds++
				met.OnBuildStarted()
				// Debounce period elapsed, rebuild
				buildMu.Lock()
				if buildInProgress {
					// Cancel current build and rebuild
					log.Info("Cancelling current build due to new changes")
					buildCancel()
					buildCtx, buildCancel = context.WithCancel(ctx)
				}
				buildInProgress = true
				buildMu.Unlock()
				updateUI(func(s *dashboard.Snapshot) {
					s.State = "rebuilding"
					s.Rebuilds = rebuilds
				})

				// Stop current server
				if r.IsRunning() {
					log.Info("Stopping server for rebuild")
					if err := r.StopWithTimeout(cfg.ShutdownTimeout); err != nil {
						log.Warn("Error stopping server", "error", err)
					}
					r.Wait()
				}

				// Rebuild
				log.Info("Building project...", "trigger", lastChangedFile, "attempt", rebuilds)
				buildStart := time.Now()
				result := b.Build(buildCtx, cfg.Build, cfg.Root)
				buildDuration := time.Since(buildStart)

				buildMu.Lock()
				buildInProgress = false
				buildMu.Unlock()

				if !result.Success {
					if builder.IsBuildCancelled(result.Error) {
						log.Info("Build was cancelled, waiting for next trigger")
						met.OnBuildFailed(result.Error, buildDuration)
						updateUI(func(s *dashboard.Snapshot) {
							s.State = "build cancelled"
						})
					} else {
						log.Error("Build failed", "error", result.Error, "duration", buildDuration)
						met.OnBuildFailed(result.Error, buildDuration)
						updateUI(func(s *dashboard.Snapshot) {
							s.State = "build failed"
							s.LastError = result.Error.Error()
							s.LastBuildDuration = buildDuration
						})
					}
					continue
				}
				successfulRebuilds++
				met.OnBuildSuccess(buildDuration)

				// Restart server
				log.Info("Build succeeded", "duration", buildDuration, "successful_rebuilds", successfulRebuilds, "total_rebuilds", rebuilds)
				log.Info("Starting server...")
				updateUI(func(s *dashboard.Snapshot) {
					s.State = "starting server"
					s.LastError = ""
					s.LastBuildDuration = buildDuration
					s.SuccessfulRebuilds = successfulRebuilds
				})
				runResult := r.Run(ctx, cfg.Exec, cfg.Root)
				if !runResult.Success {
					log.Error("Failed to start server", "error", runResult.Error)
					updateUI(func(s *dashboard.Snapshot) {
						s.State = "start failed"
						s.LastError = runResult.Error.Error()
						s.PID = -1
					})
					continue
				}
				updateUI(func(s *dashboard.Snapshot) {
					s.State = "running"
					s.PID = r.GetPID()
				})
				met.SetPID(r.GetPID())

			case <-ctx.Done():
				updateUI(func(s *dashboard.Snapshot) {
					s.State = "stopping"
				})
				return
			}
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Info("Shutting down...")
	updateUI(func(s *dashboard.Snapshot) {
		s.State = "stopping"
	})

	// Cleanup
	cancel()
	watcher.Stop()
	r.StopWithTimeout(cfg.ShutdownTimeout)
	r.Wait()
	d.Stop()

	log.Info("Goodbye!")
	updateUI(func(s *dashboard.Snapshot) {
		s.State = "stopped"
		s.PID = -1
	})
}

func printResolvedConfig(cfg *config.Config) {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to format config: %v\n", err)
		return
	}
	fmt.Println(string(b))
}
