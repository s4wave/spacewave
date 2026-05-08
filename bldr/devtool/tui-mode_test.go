//go:build !js

package devtool

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestDevtoolArgsResolveUIModeUsesTUIForTerminal(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return true }

	if mode := args.ResolveUIMode(); mode != DevtoolUIModeTUI {
		t.Fatalf("expected tui mode, got %s", mode.String())
	}
	if !args.ShouldUseTUI() {
		t.Fatal("expected ShouldUseTUI to be true")
	}
}

func TestDevtoolArgsResolveUIModeFallsBackToPlainWithoutTerminal(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return false }

	if mode := args.ResolveUIMode(); mode != DevtoolUIModePlain {
		t.Fatalf("expected plain mode, got %s", mode.String())
	}
	if args.ShouldUseTUI() {
		t.Fatal("expected ShouldUseTUI to be false")
	}
}

func TestDevtoolArgsNoTUIDisablesTerminalMode(t *testing.T) {
	args := NewDevtoolArgs()
	args.NoTUI = true
	args.terminalDetector = func() bool { return true }

	if mode := args.ResolveUIMode(); mode != DevtoolUIModePlain {
		t.Fatalf("expected no-tui to force plain mode, got %s", mode.String())
	}
}

func TestDevtoolFileIsTerminalRejectsPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()

	if devtoolFileIsTerminal(reader) {
		t.Fatal("expected pipe reader to be non-terminal")
	}
	if devtoolFileIsTerminal(writer) {
		t.Fatal("expected pipe writer to be non-terminal")
	}
}

func TestDevtoolArgsRunStatusCommandStoresResolvedMode(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return true }

	called := false
	err := args.runStatusCommand(context.Background(), func(ctx context.Context) error {
		called = true
		if mode := args.CurrentUIMode(); mode != DevtoolUIModeTUI {
			t.Fatalf("expected active tui mode, got %s", mode.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected command callback")
	}
	if !args.ShouldUseTUI() {
		t.Fatal("expected ShouldUseTUI to use stored command mode")
	}
}

func TestDevtoolArgsBuildFlagsIncludesNoTUI(t *testing.T) {
	args := NewDevtoolArgs()

	for _, flag := range args.BuildFlags() {
		if slices.Contains(flag.Names(), "no-tui") {
			return
		}
	}
	t.Fatal("expected no-tui flag")
}

func TestDevtoolArgsWriteBannerSkipsTUI(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return true }

	err := args.runStatusCommand(context.Background(), func(ctx context.Context) error {
		var out bytes.Buffer
		args.writeBannerTo(&out)
		if out.Len() != 0 {
			t.Fatalf("expected tui mode to suppress banner output, got %q", out.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevtoolArgsWriteBannerUsesPlainMode(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return false }

	err := args.runStatusCommand(context.Background(), func(ctx context.Context) error {
		var out bytes.Buffer
		args.writeBannerTo(&out)
		if !strings.Contains(out.String(), "Welcome, user") {
			t.Fatalf("expected plain mode banner output, got %q", out.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDevtoolArgsInitRepoRootRoutesConsoleLogsToFileInTUIMode(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "logs", "devtool.log")
	var console bytes.Buffer
	log := logrus.New()
	log.SetOutput(&console)
	log.SetLevel(logrus.InfoLevel)
	log.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	args := NewDevtoolArgs()
	args.Logger = logrus.NewEntry(log)
	args.UseGitRoot = false
	args.StatePath = filepath.Join(dir, "state")
	args.BuildType = "release"
	args.terminalDetector = func() bool { return true }
	if err := args.LogFiles.Set(logPath); err != nil {
		t.Fatal(err)
	}

	err := args.runStatusCommand(context.Background(), func(ctx context.Context) error {
		if _, _, err := args.InitRepoRoot(); err != nil {
			return err
		}
		if got := args.commandLogFile(); got != logPath {
			t.Fatalf("commandLogFile = %q, want %q", got, logPath)
		}
		args.Logger.Info("routed tui log")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	args.CloseLogFiles()

	if strings.Contains(console.String(), "routed tui log") {
		t.Fatalf("expected tui mode to suppress console log output, got %q", console.String())
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "routed tui log") {
		t.Fatalf("expected log file to contain routed message, got %q", string(data))
	}
}

func TestDevtoolArgsInitRepoRootResolvesDefaultDevLogUnderGitRoot(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, string(out))
	}
	subdir := filepath.Join(dir, "nested")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}

	args := NewDevtoolArgs()
	args.BuildType = "dev"
	args.StatePath = ".bldr"
	args.terminalDetector = func() bool { return true }
	if _, _, err := args.InitRepoRoot(); err != nil {
		t.Fatal(err)
	}
	defer args.CloseLogFiles()

	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(realDir, ".bldr", "logs")
	got := filepath.Dir(args.commandLogFile())
	if got != want {
		t.Fatalf("default dev log dir = %q, want %q", got, want)
	}
}
