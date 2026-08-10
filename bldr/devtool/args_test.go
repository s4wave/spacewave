//go:build !js

package devtool

import (
	"bytes"
	"strings"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestResolveWebStartModeDefaultsToGoScript(t *testing.T) {
	args := &DevtoolArgs{}

	mode, err := args.resolveWebStartMode()
	if err != nil {
		t.Fatal(err)
	}
	if mode != webStartModeGoScript {
		t.Fatalf("web start mode = %s, want %s", mode, webStartModeGoScript)
	}
}

func TestResolveWebStartModeWasmOverride(t *testing.T) {
	args := &DevtoolArgs{WebUseWasm: true}

	mode, err := args.resolveWebStartMode()
	if err != nil {
		t.Fatal(err)
	}
	if mode != webStartModeWasm {
		t.Fatalf("web start mode = %s, want %s", mode, webStartModeWasm)
	}
}

func TestResolveWebStartModeRejectsConflictingCompilerFlags(t *testing.T) {
	args := &DevtoolArgs{WebUseWasm: true, WebUseGoScript: true}

	if _, err := args.resolveWebStartMode(); err == nil {
		t.Fatal("resolveWebStartMode() error = nil, want conflict error")
	}
}

func TestDevtoolCommandShortFlagsParseAndAppearInHelp(t *testing.T) {
	tests := []struct {
		name       string
		command    *cli.Command
		commandArg []string
		check      func(*DevtoolArgs) bool
	}{
		{
			name:       "static",
			commandArg: []string{"static", "-l", "127.0.0.1:6000", "-p", "public"},
			check: func(args *DevtoolArgs) bool {
				return args.WebListenAddr == "127.0.0.1:6000" && args.ServeStaticPath == "public"
			},
		},
		{
			name:       "start web",
			commandArg: []string{"start", "web", "-l", "127.0.0.1:6001"},
			check: func(args *DevtoolArgs) bool {
				return args.WebListenAddr == "127.0.0.1:6001"
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := NewDevtoolArgs()
			var command *cli.Command
			switch tc.name {
			case "static":
				command = args.BuildStaticHttpCommand()
				command.Action = func(*cli.Context) error { return nil }
			case "start web":
				command = args.BuildStartCommand()
				command.Subcommands[2].Action = func(*cli.Context) error { return nil }
			}
			app := cli.NewApp()
			app.Name = "bldr"
			app.Flags = args.BuildFlags()
			app.Commands = []*cli.Command{command}
			if err := app.Run(append([]string{"bldr"}, tc.commandArg...)); err != nil {
				t.Fatalf("parse short flags: %v", err)
			}
			if !tc.check(args) {
				t.Fatalf("short flags did not update devtool args: %+v", args)
			}

			var output bytes.Buffer
			app.Writer = &output
			helpArgs := []string{"bldr", "start", "web"}
			if tc.name == "static" {
				helpArgs = []string{"bldr", "static"}
			}
			helpArgs = append(helpArgs, "--help")
			if err := app.Run(helpArgs); err != nil {
				t.Fatalf("render help: %v", err)
			}
			if !strings.Contains(output.String(), ", -l ") {
				t.Fatalf("help omitted -l alias:\n%s", output.String())
			}
			if tc.name == "static" && !strings.Contains(output.String(), ", -p ") {
				t.Fatalf("help omitted -p alias:\n%s", output.String())
			}
		})
	}
}

func TestDevtoolCommandValidatesParsedConfigurationBeforeAction(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		wantErr string
	}{
		{name: "empty output", flags: []string{"--output="}, wantErr: "output path"},
		{name: "empty state", flags: []string{"--state-path="}, wantErr: "state path"},
		{name: "invalid build policy", flags: []string{"--js-minification=sometimes"}, wantErr: "js-minification"},
		{name: "invalid log level", flags: []string{"--log-level=verbose"}, wantErr: "invalid log level"},
		{name: "relative source", flags: []string{"--bldr-src-path=bldr"}},
		{name: "absolute source", flags: []string{"--bldr-src-path=/tmp/bldr"}, wantErr: "relative"},
		{name: "escaping source", flags: []string{"--bldr-src-path=../bldr"}, wantErr: "escape"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := NewDevtoolArgs()
			called := false
			command := args.BuildTargetsCommand()
			command.Action = func(*cli.Context) error {
				called = true
				return nil
			}
			app := cli.NewApp()
			app.Name = "bldr"
			app.Flags = args.BuildFlags()
			app.Commands = []*cli.Command{command}
			runArgs := append([]string{"bldr"}, tc.flags...)
			err := app.Run(append(runArgs, "targets"))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("run valid command: %v", err)
				}
				if !called {
					t.Fatal("valid command action was not called")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantErr)
			}
			if called {
				t.Fatal("invalid configuration reached command action")
			}
		})
	}
}

func TestDevtoolStartCommandValidatesBeforeNestedAction(t *testing.T) {
	args := NewDevtoolArgs()
	called := false
	command := args.BuildStartCommand()
	command.Subcommands[2].Action = func(*cli.Context) error {
		called = true
		return nil
	}
	app := cli.NewApp()
	app.Name = "bldr"
	app.Flags = args.BuildFlags()
	app.Commands = []*cli.Command{command}

	err := app.Run([]string{"bldr", "--log-level=verbose", "start", "web"})
	if err == nil || !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("error = %v, want invalid log level", err)
	}
	if called {
		t.Fatal("invalid configuration reached nested command action")
	}
}
