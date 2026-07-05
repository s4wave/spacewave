package cli_plugin

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/sdk/cli/runner"
	s4wave_cli_terminal "github.com/s4wave/spacewave/sdk/cli/terminal"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

const prompt = "spacewave> "

// TerminalService runs the browser-safe Spacewave CLI command prompt over terminal frames.
type TerminalService struct {
	config runner.Config
}

// NewTerminalService constructs a CLI terminal service.
func NewTerminalService(factory runner.ClientFactory) *TerminalService {
	return &TerminalService{
		config: runner.Config{ClientFactory: factory},
	}
}

// RunCli serves one command-prompt terminal stream.
func (s *TerminalService) RunCli(strm s4wave_cli_terminal.SRPCCliTerminalService_RunCliStream) error {
	return newPromptSession(strm, s.config).run(strm.Context())
}

type promptSession struct {
	strm   s4wave_cli_terminal.SRPCCliTerminalService_RunCliStream
	config runner.Config
	line   []rune
}

func newPromptSession(strm s4wave_cli_terminal.SRPCCliTerminalService_RunCliStream, config runner.Config) *promptSession {
	return &promptSession{strm: strm, config: config}
}

func (s *promptSession) run(ctx context.Context) error {
	if err := s.strm.Send(&s4wave_terminal.TerminalFrame{Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY}); err != nil {
		return err
	}
	if err := s.writeOutput(prompt); err != nil {
		return err
	}

	for {
		frame, err := s.strm.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return err
		}
		switch frame.GetKind() {
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT:
			if err := s.handleInput(ctx, frame.GetData()); err != nil {
				return err
			}
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
			return nil
		case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE:
			continue
		}
	}
}

func (s *promptSession) handleInput(ctx context.Context, data []byte) error {
	for len(data) != 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			r = rune(data[0])
		}
		data = data[size:]

		switch r {
		case '\r', '\n':
			if err := s.writeOutput("\r\n"); err != nil {
				return err
			}
			line := strings.TrimSpace(string(s.line))
			s.line = s.line[:0]
			if line != "" {
				if err := s.runCommand(ctx, line); err != nil {
					return err
				}
			}
			if err := s.writeOutput(prompt); err != nil {
				return err
			}
		case '\b', '\x7f':
			if len(s.line) != 0 {
				s.line = s.line[:len(s.line)-1]
				if err := s.writeOutput("\b \b"); err != nil {
					return err
				}
			}
		default:
			s.line = append(s.line, r)
			if err := s.writeOutput(string(r)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *promptSession) runCommand(ctx context.Context, line string) error {
	args, err := splitCommandLine(line)
	if err != nil {
		return s.writeCommandError(err.Error())
	}
	if len(args) == 0 {
		return nil
	}
	if !isSupportedCommand(args[0]) {
		return s.writeCommandError("unsupported browser CLI command: " + args[0])
	}
	if msg := unsupportedBrowserCommandMode(args); msg != "" {
		return s.writeCommandError(msg)
	}

	opts, err := parseBrowserCommandOptions(args[1:], args[0] == "space" || args[0] == "spaces")
	if err != nil {
		return s.writeCommandError(err.Error())
	}

	var stdout bytes.Buffer
	config := s.config
	config.Stdout = &stdout
	cliCtx := &cli.Context{Context: ctx}

	switch args[0] {
	case "status":
		err = runner.RunStatus(config, cliCtx, opts.outputFormat, opts.sessionIdx)
	case "whoami":
		err = runner.RunWhoami(config, cliCtx, opts.outputFormat, opts.sessionIdx)
	case "space", "spaces":
		if len(opts.positional) != 1 || opts.positional[0] != "list" {
			return s.writeCommandError("unsupported browser CLI command: " + strings.Join(args, " "))
		}
		err = runner.RunSpaceList(config, cliCtx, opts.outputFormat, opts.sessionIdx, opts.watch)
	}
	if err != nil {
		if stdout.Len() != 0 {
			if writeErr := s.writeOutput(stdout.String()); writeErr != nil {
				return writeErr
			}
		}
		return s.writeCommandError(err.Error())
	}
	if stdout.Len() == 0 {
		return nil
	}
	return s.writeOutput(stdout.String())
}

func (s *promptSession) writeCommandError(msg string) error {
	return s.writeOutput("error: " + msg + "\r\n")
}

func (s *promptSession) writeOutput(output string) error {
	if output == "" {
		return nil
	}
	return s.strm.Send(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT,
		Data: []byte(output),
	})
}

func isSupportedCommand(name string) bool {
	switch name {
	case "status", "whoami", "space", "spaces":
		return true
	default:
		return false
	}
}

type browserCommandOptions struct {
	outputFormat string
	sessionIdx   uint32
	watch        bool
	positional   []string
}

func parseBrowserCommandOptions(args []string, allowWatch bool) (browserCommandOptions, error) {
	opts := browserCommandOptions{outputFormat: "text", sessionIdx: 1}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--output" || arg == "-o":
			i++
			if i >= len(args) {
				return opts, errors.New(arg + " requires a value")
			}
			opts.outputFormat = args[i]
		case strings.HasPrefix(arg, "--output="):
			opts.outputFormat = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-o="):
			opts.outputFormat = strings.TrimPrefix(arg, "-o=")
		case arg == "--session-index":
			i++
			if i >= len(args) {
				return opts, errors.New(arg + " requires a value")
			}
			sessionIdx, err := parseSessionIndex(args[i])
			if err != nil {
				return opts, err
			}
			opts.sessionIdx = sessionIdx
		case strings.HasPrefix(arg, "--session-index="):
			sessionIdx, err := parseSessionIndex(strings.TrimPrefix(arg, "--session-index="))
			if err != nil {
				return opts, err
			}
			opts.sessionIdx = sessionIdx
		case arg == "--watch" || arg == "-w":
			if !allowWatch {
				return opts, errors.New("unsupported flag: " + arg)
			}
			opts.watch = true
		case strings.HasPrefix(arg, "--watch="):
			if !allowWatch {
				return opts, errors.New("unsupported flag: --watch")
			}
			watch, err := strconv.ParseBool(strings.TrimPrefix(arg, "--watch="))
			if err != nil {
				return opts, err
			}
			opts.watch = watch
		case strings.HasPrefix(arg, "-w="):
			if !allowWatch {
				return opts, errors.New("unsupported flag: -w")
			}
			watch, err := strconv.ParseBool(strings.TrimPrefix(arg, "-w="))
			if err != nil {
				return opts, err
			}
			opts.watch = watch
		case strings.HasPrefix(arg, "-"):
			return opts, errors.New("unsupported flag: " + arg)
		default:
			opts.positional = append(opts.positional, arg)
		}
	}
	return opts, nil
}

func parseSessionIndex(raw string) (uint32, error) {
	idx, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "parse session index")
	}
	return uint32(idx), nil
}

func unsupportedBrowserCommandMode(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "space", "spaces":
		for _, arg := range args[1:] {
			if arg == "--watch" || arg == "-w" {
				return "browser CLI terminal does not support watch mode"
			}
			if raw, ok := strings.CutPrefix(arg, "--watch="); ok {
				enabled, err := strconv.ParseBool(raw)
				if err != nil || enabled {
					return "browser CLI terminal does not support watch mode"
				}
			}
			if raw, ok := strings.CutPrefix(arg, "-w="); ok {
				enabled, err := strconv.ParseBool(raw)
				if err != nil || enabled {
					return "browser CLI terminal does not support watch mode"
				}
			}
		}
	}
	return ""
}

func splitCommandLine(line string) ([]string, error) {
	var args []string
	var b strings.Builder
	var quote rune
	escaped := false
	inArg := false

	for _, r := range line {
		if escaped {
			b.WriteRune(r)
			inArg = true
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			inArg = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			inArg = true
		case ' ', '\t':
			if inArg {
				args = append(args, b.String())
				b.Reset()
				inArg = false
			}
		default:
			b.WriteRune(r)
			inArg = true
		}
	}
	if escaped {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inArg {
		args = append(args, b.String())
	}
	return args, nil
}

// _ is a type assertion.
var _ s4wave_cli_terminal.SRPCCliTerminalServiceServer = ((*TerminalService)(nil))
