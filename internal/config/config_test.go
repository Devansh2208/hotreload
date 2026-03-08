package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParse_YAMLAndCLIOverride(t *testing.T) {
	tmp := t.TempDir()
	rootFromFile := filepath.Join(tmp, "from-file")
	if err := os.MkdirAll(rootFromFile, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	cfgFile := filepath.Join(tmp, ".hotreload.yaml")
	yml := `root: ` + rootFromFile + `
build: go build -o ./filebin .
exec: ./filebin
debounce: 900ms
extensions: [.go, .yaml]
ignore_dirs: [.git, bin]
ui: true
summary_interval: 5s
shutdown_timeout: 3s
`
	if err := os.WriteFile(cfgFile, []byte(yml), 0644); err != nil {
		t.Fatalf("write yaml failed: %v", err)
	}

	cfg, err := parseWithArgs([]string{
		"hotreload",
		"--config", cfgFile,
		"--build", "go build -o ./override .",
		"--debounce", "200ms",
		"--ui=false",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if cfg.Build != "go build -o ./override ." {
		t.Fatalf("expected CLI build override, got %q", cfg.Build)
	}
	if cfg.Debounce != 200*time.Millisecond {
		t.Fatalf("expected debounce override, got %v", cfg.Debounce)
	}
	if cfg.UI {
		t.Fatalf("expected ui=false from CLI override")
	}
	if cfg.SummaryInterval != 5*time.Second {
		t.Fatalf("expected summary interval from yaml, got %v", cfg.SummaryInterval)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("expected shutdown timeout from yaml, got %v", cfg.ShutdownTimeout)
	}
}

func TestParse_StatusSkipsPathValidation(t *testing.T) {
	cfg, err := parseWithArgs([]string{
		"hotreload",
		"--status",
		"--root", filepath.Join("Z:", "definitely-not-real-path-for-test"),
	})
	if err != nil {
		t.Fatalf("parse should not fail for status mode: %v", err)
	}
	if !cfg.Status {
		t.Fatalf("expected status=true")
	}
}

func parseWithArgs(args []string) (*Config, error) {
	origArgs := os.Args
	origFlagSet := flag.CommandLine
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origFlagSet
	}()

	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	os.Args = args
	return Parse()
}
