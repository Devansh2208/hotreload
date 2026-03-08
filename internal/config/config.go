package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the CLI and file configuration for hotreload.
type Config struct {
	ConfigPath      string        // Optional path to .hotreload.yaml
	Root            string        // Directory to watch
	Build           string        // Build command
	Exec            string        // Run command
	Debounce        time.Duration // Debounce delay for file events
	Extensions      []string      // File extensions to watch (e.g. .go,.js)
	IgnoreDirs      []string      // Directory names to ignore
	UI              bool          // Enables live terminal dashboard mode
	Status          bool          // Prints resolved config and exits
	DryRun          bool          // Validates setup and exits without watching
	Version         bool          // Prints version and exits
	SummaryInterval time.Duration // Interval for periodic summary logs
	ShutdownTimeout time.Duration // Grace period before force-killing server
}

type fileConfig struct {
	Root            string   `yaml:"root"`
	Build           string   `yaml:"build"`
	Exec            string   `yaml:"exec"`
	Debounce        string   `yaml:"debounce"`
	Extensions      []string `yaml:"extensions"`
	IgnoreDirs      []string `yaml:"ignore_dirs"`
	UI              *bool    `yaml:"ui"`
	SummaryInterval string   `yaml:"summary_interval"`
	ShutdownTimeout string   `yaml:"shutdown_timeout"`
}

// Parse parses CLI flags, optional YAML file config, and merges them.
// CLI flags override file values.
func Parse() (*Config, error) {
	root := flag.String("root", "", "Directory to watch for changes")
	build := flag.String("build", "", "Build command")
	execCmd := flag.String("exec", "", "Run command")
	debounce := flag.String("debounce", "", "Debounce delay (e.g. 300ms, 1s)")
	extensions := flag.String("ext", "", "Comma-separated file extensions to watch")
	ignoreDirs := flag.String("ignore", "", "Comma-separated directory names to ignore")
	configPath := flag.String("config", ".hotreload.yaml", "Path to YAML config file")
	ui := flag.Bool("ui", false, "Enable live terminal dashboard")
	status := flag.Bool("status", false, "Print resolved configuration and exit")
	dryRun := flag.Bool("dry-run", false, "Validate config/build command and exit")
	version := flag.Bool("version", false, "Print version and exit")
	summaryInterval := flag.String("summary-interval", "", "Periodic summary interval (e.g. 10s, 1m); 0 disables")
	shutdownTimeout := flag.String("shutdown-timeout", "", "Graceful shutdown timeout before force kill (e.g. 2s)")

	flag.Parse()
	setFlags := visitedFlags()

	cfg := defaultConfig()
	cfg.ConfigPath = *configPath

	_, configExplicit := setFlags["config"]
	if err := cfg.loadYAML(*configPath, configExplicit); err != nil {
		return nil, err
	}

	if _, ok := setFlags["root"]; ok {
		cfg.Root = *root
	}
	if _, ok := setFlags["build"]; ok {
		cfg.Build = *build
	}
	if _, ok := setFlags["exec"]; ok {
		cfg.Exec = *execCmd
	}
	if _, ok := setFlags["debounce"]; ok {
		d, err := time.ParseDuration(*debounce)
		if err != nil {
			return nil, fmt.Errorf("invalid --debounce value: %w", err)
		}
		cfg.Debounce = d
	}
	if _, ok := setFlags["ext"]; ok {
		cfg.Extensions = parseExtensionsCSV(*extensions)
	}
	if _, ok := setFlags["ignore"]; ok {
		cfg.IgnoreDirs = parseNamesCSV(*ignoreDirs)
	}
	if _, ok := setFlags["ui"]; ok {
		cfg.UI = *ui
	}
	if _, ok := setFlags["status"]; ok {
		cfg.Status = *status
	}
	if _, ok := setFlags["dry-run"]; ok {
		cfg.DryRun = *dryRun
	}
	if _, ok := setFlags["version"]; ok {
		cfg.Version = *version
	}
	if _, ok := setFlags["summary-interval"]; ok {
		v, err := time.ParseDuration(*summaryInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid --summary-interval value: %w", err)
		}
		cfg.SummaryInterval = v
	}
	if _, ok := setFlags["shutdown-timeout"]; ok {
		v, err := time.ParseDuration(*shutdownTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid --shutdown-timeout value: %w", err)
		}
		cfg.ShutdownTimeout = v
	}

	if err := cfg.Validate(!cfg.Version && !cfg.Status); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate validates the configuration.
func (c *Config) Validate(requirePaths bool) error {
	if requirePaths && c.Root == "" {
		return fmt.Errorf("root directory is required")
	}
	if requirePaths && c.Build == "" {
		return fmt.Errorf("build command is required")
	}
	if requirePaths && c.Exec == "" {
		return fmt.Errorf("exec command is required")
	}
	if c.Debounce <= 0 {
		return fmt.Errorf("debounce must be > 0")
	}
	if len(c.Extensions) == 0 {
		return fmt.Errorf("at least one extension must be provided")
	}

	if requirePaths {
		info, err := os.Stat(c.Root)
		if err != nil {
			return fmt.Errorf("failed to stat root directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("root is not a directory: %s", c.Root)
		}
	}
	if c.SummaryInterval < 0 {
		return fmt.Errorf("summary interval must be >= 0")
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be > 0")
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		ConfigPath: ".hotreload.yaml",
		Root:       ".",
		Build:      "go build -o ./bin/server ./cmd/server",
		Exec:       "./bin/server",
		Debounce:   300 * time.Millisecond,
		Extensions: parseExtensionsCSV(".go,.c,.cpp,.h,.hpp,.rs,.py,.js,.ts,.jsx,.tsx,.mod,.sum,.yaml,.yml,.json,.toml"),
		IgnoreDirs: parseNamesCSV(".git,node_modules,vendor,bin,dist,.cache,.idea,.vscode"),
		UI:         false,
		Status:     false,
		DryRun:     false,
		Version:    false,
		// 0 disables periodic summaries.
		SummaryInterval: 0,
		ShutdownTimeout: 2 * time.Second,
	}
}

func (c *Config) loadYAML(path string, required bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		return fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return fmt.Errorf("failed to parse config file %q: %w", path, err)
	}

	if fc.Root != "" {
		c.Root = fc.Root
	}
	if fc.Build != "" {
		c.Build = fc.Build
	}
	if fc.Exec != "" {
		c.Exec = fc.Exec
	}
	if fc.Debounce != "" {
		d, err := time.ParseDuration(fc.Debounce)
		if err != nil {
			return fmt.Errorf("invalid debounce in config file %q: %w", path, err)
		}
		c.Debounce = d
	}
	if len(fc.Extensions) > 0 {
		c.Extensions = normalizeExtensions(fc.Extensions)
	}
	if len(fc.IgnoreDirs) > 0 {
		c.IgnoreDirs = normalizeNames(fc.IgnoreDirs)
	}
	if fc.UI != nil {
		c.UI = *fc.UI
	}
	if fc.SummaryInterval != "" {
		d, err := time.ParseDuration(fc.SummaryInterval)
		if err != nil {
			return fmt.Errorf("invalid summary_interval in config file %q: %w", path, err)
		}
		c.SummaryInterval = d
	}
	if fc.ShutdownTimeout != "" {
		d, err := time.ParseDuration(fc.ShutdownTimeout)
		if err != nil {
			return fmt.Errorf("invalid shutdown_timeout in config file %q: %w", path, err)
		}
		c.ShutdownTimeout = d
	}
	return nil
}

func visitedFlags() map[string]struct{} {
	m := map[string]struct{}{}
	flag.Visit(func(f *flag.Flag) {
		m[f.Name] = struct{}{}
	})
	return m
}

func parseExtensionsCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeExtensions(strings.Split(raw, ","))
}

func parseNamesCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeNames(strings.Split(raw, ","))
}

func normalizeExtensions(values []string) []string {
	out := make([]string, 0, len(values))
	for _, p := range values {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if !strings.HasPrefix(v, ".") {
			v = "." + v
		}
		out = append(out, strings.ToLower(v))
	}
	return out
}

func normalizeNames(values []string) []string {
	out := make([]string, 0, len(values))
	for _, p := range values {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
