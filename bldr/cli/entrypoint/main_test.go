//go:build !js

package cli_entrypoint

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestMainLogsTerminalCommandError(t *testing.T) {
	// Re-enter as the child process and run the requested failure mode.
	const childEnv = "SPACEWAVE_TEST_TERMINAL_COMMAND_ERROR"
	if mode := os.Getenv(childEnv); mode != "" {
		// Point the entrypoint at the child's state path and log file.
		os.Args = []string{
			"test-entrypoint",
			"--state-path", os.Getenv(childEnv + "_STATE"),
			"--log-file", "level=DEBUG;path=" + os.Getenv(childEnv+"_LOG"),
		}
		if mode == "nested" {
			os.Args = append(os.Args, "parent", "leaf")
		} else {
			os.Args = append(os.Args, "serve")
		}
		// Run the entrypoint with a serve command that fails per the mode.
		Main(
			"test-entrypoint",
			"test-entrypoint",
			nil,
			nil,
			[]BuildCommandsFunc{func(getBus func() CliBus) []*cli.Command {
				command := &cli.Command{Name: "serve"}
				switch mode {
				case "ordinary":
					command.Action = func(*cli.Context) error {
						return errors.New("serve failed deliberately")
					}
				case "exit-coder":
					command.Action = func(*cli.Context) error {
						return cli.Exit("serve requested custom exit", 23)
					}
				case "required-flag":
					command.Flags = []cli.Flag{&cli.StringFlag{
						Name:     "token",
						Required: true,
					}}
				case "nested":
					return []*cli.Command{{
						Name: "parent",
						Subcommands: []*cli.Command{{
							Name: "leaf",
							Action: func(*cli.Context) error {
								return errors.New("nested leaf failed deliberately")
							},
						}},
					}}
				case "panic":
					command.Action = func(*cli.Context) error {
						b := getBus()
						if b == nil {
							panic("bus initialization failed")
						}
						b.AddRelease(func() {
							b.GetLogger().Info("panic drain marker")
						})
						panic("serve panicked deliberately")
					}
				}
				return []*cli.Command{command}
			}},
		)
		return
	}

	// Enumerate the failure modes and their expected exit codes and logs.
	tests := []struct {
		name           string
		mode           string
		exitCode       int
		wantCommand    string
		wantError      string
		wantErrorCount int
	}{
		{
			name:           "ordinary error",
			mode:           "ordinary",
			exitCode:       1,
			wantCommand:    "test-entrypoint serve stopped",
			wantError:      "serve failed deliberately",
			wantErrorCount: 1,
		},
		{
			name:           "custom exit code",
			mode:           "exit-coder",
			exitCode:       23,
			wantCommand:    "test-entrypoint serve stopped",
			wantError:      "serve requested custom exit",
			wantErrorCount: 1,
		},
		{
			name:           "required flag error",
			mode:           "required-flag",
			exitCode:       1,
			wantCommand:    "test-entrypoint stopped",
			wantError:      `Required flag \"token\" not set`,
			wantErrorCount: 1,
		},
		{
			name:           "nested command error",
			mode:           "nested",
			exitCode:       1,
			wantCommand:    "test-entrypoint parent leaf stopped",
			wantError:      "nested leaf failed deliberately",
			wantErrorCount: 1,
		},
		{
			name:      "panic drains release log",
			mode:      "panic",
			exitCode:  2,
			wantError: "panic drain marker",
		},
	}
	// Run each failure mode in a child process and check its exit and log.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Launch the child process with the mode and log path set.
			tempDir := t.TempDir()
			logPath := filepath.Join(tempDir, "entrypoint.log")
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainLogsTerminalCommandError$")
			cmd.Env = append(
				os.Environ(),
				childEnv+"="+tt.mode,
				childEnv+"_LOG="+logPath,
				childEnv+"_STATE="+filepath.Join(tempDir, "state"),
			)

			// Run the child and expect the requested exit code.
			out, err := cmd.CombinedOutput()
			if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != tt.exitCode {
				t.Fatalf("child exit = %v, want status %d; output: %s", err, tt.exitCode, out)
			}

			// Read the log and check the error and terminal records.
			data, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			logOutput := string(data)
			if count := strings.Count(logOutput, "level=error"); count != tt.wantErrorCount {
				t.Fatalf("error record count = %d, want %d: %q", count, tt.wantErrorCount, logOutput)
			}
			if tt.wantCommand != "" {
				if count := strings.Count(logOutput, tt.wantCommand); count != 1 {
					t.Fatalf("terminal record count = %d, want 1: %q", count, logOutput)
				}
			}
			if !strings.Contains(logOutput, tt.wantError) {
				t.Fatalf("log missing terminal error: %q", logOutput)
			}
		})
	}
}
