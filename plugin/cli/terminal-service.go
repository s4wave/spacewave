package cli_plugin

import (
	"context"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/sdk/cli/runner"
	s4wave_cli_terminal "github.com/s4wave/spacewave/sdk/cli/terminal"
	s4wave_terminal "github.com/s4wave/spacewave/sdk/terminal"
)

const (
	prompt               = "spacewave> "
	maxCommandOutput     = 4096
	fullNativeCLIPointer = "Open Command Line settings for the full native CLI."
)

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

	outMu sync.Mutex

	line         []rune
	cursor       int
	history      []string
	historyIndex int
	historyDraft []rune
}

type terminalRecv struct {
	frame *s4wave_terminal.TerminalFrame
	err   error
}

type commandState struct {
	done        chan commandResult
	cancel      context.CancelFunc
	interrupted bool
}

type commandResult struct {
	err error
}

type promptAction struct {
	command string
	exit    bool
}

type commandUsage struct {
	name  string
	usage string
}

type terminalOutputWriter struct {
	session *promptSession
}

func newPromptSession(strm s4wave_cli_terminal.SRPCCliTerminalService_RunCliStream, config runner.Config) *promptSession {
	return &promptSession{strm: strm, config: config, historyIndex: -1}
}

func (s *promptSession) run(ctx context.Context) error {
	if err := s.strm.Send(&s4wave_terminal.TerminalFrame{Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_READY}); err != nil {
		return err
	}
	if err := s.writeOutput(prompt); err != nil {
		return err
	}
	defer s.strm.Close()

	recvCh := s.recvFrames(ctx)
	var command *commandState
	for {
		if command != nil {
			select {
			case result := <-command.done:
				if err := s.finishCommand(command, result); err != nil {
					return err
				}
				command = nil
				continue
			default:
			}
		}
		select {
		case recv := <-recvCh:
			if recv.err != nil {
				if command != nil {
					command.cancel()
					<-command.done
				}
				if errors.Is(recv.err, io.EOF) || errors.Is(ctx.Err(), context.Canceled) {
					return nil
				}
				return recv.err
			}
			switch recv.frame.GetKind() {
			case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_INPUT:
				if command != nil {
					if hasInterrupt(recv.frame.GetData()) && !command.interrupted {
						command.interrupted = true
						command.cancel()
						if err := s.writeOutput("^C\r\n"); err != nil {
							return err
						}
					}
					continue
				}
				action, err := s.handleInput(ctx, recv.frame.GetData())
				if err != nil {
					return err
				}
				if action.exit {
					return s.writeExit(0)
				}
				if action.command != "" {
					command = s.startCommand(ctx, action.command)
				}
			case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_CLOSE:
				if command != nil {
					select {
					case result := <-command.done:
						if err := s.finishCommand(command, result); err != nil {
							return err
						}
					default:
						command.cancel()
						<-command.done
					}
				}
				return nil
			case s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_RESIZE:
				continue
			}
		case result := <-commandDone(command):
			if err := s.finishCommand(command, result); err != nil {
				return err
			}
			command = nil
		}
	}
}

func (s *promptSession) recvFrames(ctx context.Context) <-chan terminalRecv {
	ch := make(chan terminalRecv, 1)
	go func() {
		for {
			frame, err := s.strm.Recv()
			select {
			case ch <- terminalRecv{frame: frame, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

func commandDone(command *commandState) <-chan commandResult {
	if command == nil {
		return nil
	}
	return command.done
}

func (s *promptSession) startCommand(ctx context.Context, line string) *commandState {
	cmdCtx, cancel := context.WithCancel(ctx)
	command := &commandState{done: make(chan commandResult, 1), cancel: cancel}
	go func() {
		command.done <- commandResult{err: s.executeCommand(cmdCtx, line)}
	}()
	return command
}

func (s *promptSession) finishCommand(command *commandState, result commandResult) error {
	if result.err != nil && (!command.interrupted || !errors.Is(result.err, context.Canceled)) {
		if err := s.writeCommandError(result.err.Error()); err != nil {
			return err
		}
	}
	return s.writeOutput(prompt)
}

func (s *promptSession) handleInput(ctx context.Context, data []byte) (promptAction, error) {
	for len(data) != 0 {
		if handled, rest, err := s.handleEscape(data); handled || err != nil {
			if err != nil {
				return promptAction{}, err
			}
			data = rest
			continue
		}
		if data[0] == 0x03 {
			s.line = s.line[:0]
			s.cursor = 0
			s.resetHistoryRecall()
			if err := s.writeOutput("^C\r\n" + prompt); err != nil {
				return promptAction{}, err
			}
			data = data[1:]
			continue
		}

		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			r = rune(data[0])
		}
		data = data[size:]

		switch r {
		case '\r', '\n':
			if err := s.writeOutput("\r\n"); err != nil {
				return promptAction{}, err
			}
			line := strings.TrimSpace(string(s.line))
			s.line = s.line[:0]
			s.cursor = 0
			s.resetHistoryRecall()
			if line == "" {
				if err := s.writeOutput(prompt); err != nil {
					return promptAction{}, err
				}
				continue
			}
			s.pushHistory(line)
			switch line {
			case "help", "?":
				if err := s.writeHelp(); err != nil {
					return promptAction{}, err
				}
				if err := s.writeOutput(prompt); err != nil {
					return promptAction{}, err
				}
			case "clear":
				if err := s.writeOutput("\x1b[2J\x1b[H" + prompt); err != nil {
					return promptAction{}, err
				}
			case "exit":
				return promptAction{exit: true}, nil
			default:
				return promptAction{command: line}, nil
			}
		case '\b', '\x7f':
			if s.cursor != 0 {
				s.line = append(s.line[:s.cursor-1], s.line[s.cursor:]...)
				s.cursor--
				s.resetHistoryRecall()
				if err := s.redrawLine(); err != nil {
					return promptAction{}, err
				}
			}
		default:
			s.line = append(s.line, 0)
			copy(s.line[s.cursor+1:], s.line[s.cursor:])
			s.line[s.cursor] = r
			s.cursor++
			s.resetHistoryRecall()
			if s.cursor == len(s.line) {
				if err := s.writeOutput(string(r)); err != nil {
					return promptAction{}, err
				}
				continue
			}
			if err := s.redrawLine(); err != nil {
				return promptAction{}, err
			}
		}
	}
	return promptAction{}, nil
}

func (s *promptSession) handleEscape(data []byte) (bool, []byte, error) {
	if len(data) < 3 || data[0] != '\x1b' || data[1] != '[' {
		return false, data, nil
	}
	switch data[2] {
	case 'A':
		return true, data[3:], s.recallHistory(-1)
	case 'B':
		return true, data[3:], s.recallHistory(1)
	case 'C':
		if s.cursor < len(s.line) {
			s.cursor++
			return true, data[3:], s.writeOutput("\x1b[C")
		}
	case 'D':
		if s.cursor != 0 {
			s.cursor--
			return true, data[3:], s.writeOutput("\x1b[D")
		}
	}
	return true, data[3:], nil
}

func (s *promptSession) recallHistory(direction int) error {
	if len(s.history) == 0 {
		return nil
	}
	if direction < 0 {
		if s.historyIndex == -1 {
			s.historyDraft = append(s.historyDraft[:0], s.line...)
			s.historyIndex = len(s.history) - 1
		} else if s.historyIndex > 0 {
			s.historyIndex--
		}
	} else {
		if s.historyIndex == -1 {
			return nil
		}
		if s.historyIndex < len(s.history)-1 {
			s.historyIndex++
		} else {
			s.line = append(s.line[:0], s.historyDraft...)
			s.cursor = len(s.line)
			s.historyIndex = -1
			return s.redrawLine()
		}
	}
	s.line = []rune(s.history[s.historyIndex])
	s.cursor = len(s.line)
	return s.redrawLine()
}

func (s *promptSession) pushHistory(line string) {
	if len(s.history) == 0 || s.history[len(s.history)-1] != line {
		s.history = append(s.history, line)
	}
}

func (s *promptSession) resetHistoryRecall() {
	s.historyIndex = -1
	s.historyDraft = s.historyDraft[:0]
}

func (s *promptSession) redrawLine() error {
	out := "\r\x1b[2K" + prompt + string(s.line)
	if right := len(s.line) - s.cursor; right != 0 {
		out += "\x1b[" + strconv.Itoa(right) + "D"
	}
	return s.writeOutput(out)
}

func (s *promptSession) executeCommand(ctx context.Context, line string) error {
	args, err := splitCommandLine(line)
	if err != nil {
		return s.writeCommandError(err.Error())
	}
	if len(args) == 0 {
		return nil
	}
	if !isSupportedCommand(args[0]) {
		return s.writeUnsupportedCommandError(args[0])
	}

	opts, err := parseBrowserCommandOptions(args[1:], args[0] == "space" || args[0] == "spaces")
	if err != nil {
		return s.writeCommandError(err.Error())
	}

	config := s.config
	config.Stdout = &terminalOutputWriter{session: s}
	cliCtx := &cli.Context{Context: ctx}

	switch args[0] {
	case "status":
		err = runner.RunStatus(config, cliCtx, opts.outputFormat, opts.sessionIdx)
	case "whoami":
		err = runner.RunWhoami(config, cliCtx, opts.outputFormat, opts.sessionIdx)
	case "space", "spaces":
		if len(opts.positional) != 1 || opts.positional[0] != "list" {
			return s.writeUnsupportedCommandError(strings.Join(args, " "))
		}
		err = runner.RunSpaceList(config, cliCtx, opts.outputFormat, opts.sessionIdx, opts.watch)
	}
	return err
}

func (s *promptSession) writeHelp() error {
	var out strings.Builder
	out.WriteString("Supported browser CLI commands:\r\n")
	for _, usage := range supportedCommandUsages(s.config) {
		out.WriteString("  ")
		out.WriteString(usage.name)
		if pad := 14 - len(usage.name); pad > 0 {
			out.WriteString(strings.Repeat(" ", pad))
		} else {
			out.WriteString("  ")
		}
		out.WriteString(usage.usage)
		out.WriteString("\r\n")
	}
	out.WriteString(fullNativeCLIPointer)
	out.WriteString("\r\n")
	return s.writeOutput(out.String())
}

func supportedCommandUsages(config runner.Config) []commandUsage {
	commands := runner.NewCommands(config)
	usages := make([]commandUsage, 0, 8)
	for _, cmd := range commands {
		usages = append(usages, commandUsage{name: cmd.Name, usage: cmd.Usage})
		for _, sub := range cmd.Subcommands {
			usages = append(usages, commandUsage{name: cmd.Name + " " + sub.Name, usage: sub.Usage})
			for _, alias := range cmd.Aliases {
				usages = append(usages, commandUsage{name: alias + " " + sub.Name, usage: sub.Usage})
			}
		}
	}
	usages = append(usages,
		commandUsage{name: "help, ?", usage: "show browser CLI help"},
		commandUsage{name: "clear", usage: "clear the terminal"},
		commandUsage{name: "exit", usage: "close this browser CLI prompt"},
	)
	return usages
}

func (s *promptSession) writeCommandError(msg string) error {
	return s.writeOutput("error: " + msg + "\r\n")
}

func (s *promptSession) writeUnsupportedCommandError(command string) error {
	return s.writeOutput("error: unsupported browser CLI command: " + command + "\r\n" + supportedCommandSetLine() + "\r\n" + fullNativeCLIPointer + "\r\n")
}

func (s *promptSession) writeOutput(output string) error {
	if output == "" {
		return nil
	}
	return s.writeOutputBytes([]byte(output))
}

func (s *promptSession) writeOutputBytes(output []byte) error {
	if len(output) == 0 {
		return nil
	}
	data := normalizeTerminalOutput(output)
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return s.strm.Send(&s4wave_terminal.TerminalFrame{
		Kind: s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_OUTPUT,
		Data: data,
	})
}

func normalizeTerminalOutput(output []byte) []byte {
	data := make([]byte, 0, len(output))
	for i, b := range output {
		if b == '\n' && (i == 0 || output[i-1] != '\r') {
			data = append(data, '\r')
		}
		data = append(data, b)
	}
	return data
}

func (s *promptSession) writeExit(code int32) error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return s.strm.Send(&s4wave_terminal.TerminalFrame{
		Kind:     s4wave_terminal.TerminalFrameKind_TERMINAL_FRAME_KIND_EXIT,
		ExitCode: code,
	})
}

func (w *terminalOutputWriter) Write(data []byte) (int, error) {
	total := 0
	for len(data) != 0 {
		n := min(len(data), maxCommandOutput)
		if err := w.session.writeOutputBytes(data[:n]); err != nil {
			return total, err
		}
		total += n
		data = data[n:]
	}
	return total, nil
}

func isSupportedCommand(name string) bool {
	switch name {
	case "status", "whoami", "space", "spaces":
		return true
	default:
		return false
	}
}

func supportedCommandSetLine() string {
	return "supported browser CLI commands: status, whoami, space list, spaces list, help, ?, clear, exit"
}

func allowedFlagSet(allowWatch bool) string {
	if allowWatch {
		return "--output, -o, --session-index, --watch, -w"
	}
	return "--output, -o, --session-index"
}

type browserCommandOptions struct {
	outputFormat string
	sessionIdx   uint32
	watch        bool
	positional   []string
}

func parseBrowserCommandOptions(args []string, allowWatch bool) (browserCommandOptions, error) {
	// Initialize defaults shared by all browser CLI commands.
	opts := browserCommandOptions{outputFormat: "text", sessionIdx: 1}

	// Parse flags and retain positional command arguments.
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
				return opts, errors.New("unsupported flag: " + arg + " (allowed flags: " + allowedFlagSet(false) + ")")
			}
			opts.watch = true
		case strings.HasPrefix(arg, "--watch="):
			if !allowWatch {
				return opts, errors.New("unsupported flag: --watch (allowed flags: " + allowedFlagSet(false) + ")")
			}
			watch, err := strconv.ParseBool(strings.TrimPrefix(arg, "--watch="))
			if err != nil {
				return opts, err
			}
			opts.watch = watch
		case strings.HasPrefix(arg, "-w="):
			if !allowWatch {
				return opts, errors.New("unsupported flag: -w (allowed flags: " + allowedFlagSet(false) + ")")
			}
			watch, err := strconv.ParseBool(strings.TrimPrefix(arg, "-w="))
			if err != nil {
				return opts, err
			}
			opts.watch = watch
		case strings.HasPrefix(arg, "-"):
			return opts, errors.New("unsupported flag: " + arg + " (allowed flags: " + allowedFlagSet(allowWatch) + ")")
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

func hasInterrupt(data []byte) bool {
	return slices.Contains(data, 0x03)
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
var _ s4wave_cli_terminal.SRPCCliTerminalServiceServer = (*TerminalService)(nil)
