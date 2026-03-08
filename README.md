# HotReload - Automatic Go Server Hot Reload Tool

A production-quality CLI tool that automatically rebuilds and restarts a Go server when source code changes are detected.

## Overview

HotReload solves the common development pain point of manually stopping, rebuilding, and restarting a Go server every time code changes. It monitors a directory for file changes and automatically triggers a rebuild and restart cycle.

## Architecture

### Components

```
hotreload/
├── cmd/hotreload/main.go      # Application entry point
├── internal/
│   ├── config/config.go       # CLI flag parsing and validation
│   ├── debounce/debounce.go   # Debouncing mechanism for file events
│   ├── builder/builder.go     # Build command execution
│   ├── runner/runner.go       # Server process management
│   └── watcher/watcher.go     # File system watching with fsnotify
├── pkg/logger/logger.go       # Structured logging with slog
├── testserver/                # Demo HTTP server
├── Makefile                   # Build automation
└── README.md
```

### Design Decisions

1. **fsnotify for File Events**: Uses the `fsnotify` library for cross-platform file system watching. This provides reliable file change detection across Linux/macOS.

2. **Debouncing (300-500ms)**: Rapid file events (common when editors save) are debounced to prevent unnecessary rebuilds. Only the final event after a quiet period triggers a build.

3. **Process Tree Management**: Uses OS-specific process handling so child processes are terminated properly during restarts on Unix and Windows.

4. **Context Cancellation**: Uses `exec.CommandContext` and Go contexts to properly cancel in-progress builds when new file changes occur.

5. **Restart Backoff**: Implements exponential backoff to prevent restart loops when the server crashes immediately after start.

6. **Ignored Directories**: Automatically ignores `.git`, `node_modules`, `vendor`, `bin`, and other build artifacts to reduce noise.

## Installation

```bash
# Clone and navigate to the project
cd hotreload

# Install dependencies for the project
go mod download
go get github.com/fsnotify/fsnotify

# Build the tool
make build

# Or manually:
go build -o bin/hotreload ./cmd/hotreload
```

## Usage

```bash
./bin/hotreload --root ./myproject --build "go build -o ./bin/server ./cmd/server" --exec "./bin/server"
```

### Parameters

| Flag | Description | Default |
|------|-------------|---------|
| `--root` | Directory to watch for changes | `.` |
| `--build` | Build command to execute | `go build -o ./bin/server ./cmd/server` |
| `--exec` | Run command to start the server | `./bin/server` |
| `--debounce` | Debounce delay before rebuild | `300ms` |
| `--ext` | Comma-separated file extensions to watch | `.go,.c,.cpp,...` |
| `--ignore` | Comma-separated directory names to ignore | `.git,node_modules,...` |
| `--config` | Path to YAML config file | `.hotreload.yaml` |
| `--ui` | Enable live terminal dashboard | `false` |
| `--status` | Print resolved config and exit | `false` |
| `--dry-run` | Validate configuration and exit | `false` |
| `--version` | Print build version and exit | `false` |
| `--summary-interval` | Emit periodic runtime summary logs (`0` disables) | `0` |
| `--shutdown-timeout` | Graceful shutdown timeout before force kill | `2s` |

### Example with custom watch rules

```bash
./bin/hotreload --root . --build "go build -o ./bin/app ./cmd/app" --exec "./bin/app" --debounce 500ms --ext ".go,.tmpl,.yaml,.env" --ignore ".git,node_modules,dist,tmp"
```

### YAML Configuration

Create a `.hotreload.yaml` in the project root:

```yaml
root: ./testserver
build: go build -o ./server ./testserver
exec: ./server
debounce: 400ms
summary_interval: 15s
shutdown_timeout: 2s
extensions: [.go, .yaml, .yml]
ignore_dirs: [.git, bin, dist]
ui: true
```

Then run:

```bash
./bin/hotreload
```

You can also pass another file:

```bash
./bin/hotreload --config ./.hotreload.example.yaml
```

CLI flags always override YAML values.

### Status, Dry-run, Version

```bash
./bin/hotreload --status
./bin/hotreload --dry-run --config ./.hotreload.example.yaml
./bin/hotreload --version
```

## Demo

### Running the Test Server

```bash
# Build hotreload and run the demo
make run-demo
```

On Windows PowerShell:

```powershell
go build -o .\bin\hotreload.exe .\cmd\hotreload
.\bin\hotreload.exe --root .\testserver --build "go build -o .\server.exe .\testserver" --exec ".\server.exe" --ui
```

This will:
1. Build the hotreload tool
2. Start the test server on port 8080
3. Watch the `testserver/` directory for changes
4. Automatically rebuild and restart when you modify files

### Test the Server

```bash
# In another terminal, test the server
curl http://localhost:8080/
```

### Make Changes

1. Open `testserver/main.go` in your editor
2. Modify the message (e.g., change "Hello from test server!" to "Hot reloaded!")
3. Save the file
4. The server will automatically rebuild and restart within ~2 seconds
5. Refresh `http://localhost:8080/` to see your changes

## Running Tests

```bash
# Run all tests
make test

# Or manually:
go test -v ./...
```

### Test Coverage

- **Debounce Logic**: Tests for single event handling, multiple triggers, cancellation, and stop behavior
- **Change Event Handling**: Tests for directory add/remove, file filtering, and event processing

## Design Decisions

### Why fsnotify?

The `fsnotify` library provides:
- Cross-platform support (Linux, macOS, Windows)
- Reliable file system event delivery
- Low overhead for directory watching

### Why Debounce?

Editors often trigger multiple file events for a single save:
- IDEs may save multiple files
- Temporary files are created and deleted
- Auto-save features create many events

Debouncing ensures we only rebuild once after all related events settle.

### Process Cleanup

Simply killing a process doesn't kill its children. Hotreload uses process groups on Unix and `taskkill /T /F` on Windows to ensure:
- All spawned processes are terminated
- No orphaned processes remain
- Ports are freed properly

### Restart Backoff

If a server crashes immediately after starting, we could get into a restart loop. The backoff mechanism:
- Starts at 100ms
- Doubles on each immediate crash
- Caps at 5 seconds
- Resets after a successful start

## Assignment Mapping

### Core Requirements

- Watch for file changes recursively:
  - Implemented in `internal/watcher` with recursive startup scan and dynamic add/remove for directories.
- Trigger initial build immediately:
  - `cmd/hotreload/main.go` runs build before entering watch loop.
- Rebuild and restart automatically:
  - Debounced event channel triggers stop -> build -> restart sequence.
- Keep restart responsive under event bursts:
  - `internal/debounce` collapses event storms into a single rebuild trigger.
- Discard stale builds if new changes arrive:
  - Build context is cancelled and replaced when newer changes appear.
- Stream logs in real time:
  - Child process stdout/stderr are attached directly to terminal streams.

### Bonus Features Implemented

- Cross-platform process-tree shutdown:
  - Unix process groups + Windows `taskkill /T /F`.
- Stubborn process handling:
  - Graceful interrupt with timeout fallback to force kill (`--shutdown-timeout`).
- Crash-loop mitigation:
  - Exponential restart backoff in `internal/runner`.
- File filtering:
  - Ignore directories + extension filtering + temporary/editor file suppression.
- Runtime observability:
  - `--ui` live dashboard and optional periodic summary metrics (`--summary-interval`).
- Config ergonomics:
  - `.hotreload.yaml` support with CLI override precedence.

## Known Limitations & Future Work

- Command parsing currently uses simple whitespace splitting, so complex quoted arguments are limited.
- There is no external IPC endpoint for querying live status from another process yet.
- Future work:
  - switch command parsing to shell-aware semantics
  - add optional JSON status socket/HTTP endpoint
  - add more integration scenarios (rename storms, large tree stress)

## Development

### Project Structure

The code follows standard Go project layout:
- `cmd/` - Application entry points
- `internal/` - Private application code
- `pkg/` - Reusable packages
- `testserver/` - Demo server

### Code Style

- Uses Go's native `log/slog` for structured logging
- Follows standard Go naming conventions
- Modular, single-responsibility components
- Comprehensive error handling

## Troubleshooting

### Server won't start

Check the build command and ensure the binary path exists:
```bash
go build -o ./bin/server ./cmd/server
./bin/server
```

### Changes not detected

Ensure the root directory path is correct and you have permission to read it.

### Too many rebuilds

The debounce is set to 300ms. If you still see too many rebuilds, you can adjust it in the code.

## License

MIT License - Feel free to use and modify for your projects.

