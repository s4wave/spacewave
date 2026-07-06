//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	trace_service "github.com/s4wave/spacewave/core/trace/service"
	"github.com/sirupsen/logrus"
)

const (
	cliTerminalGoScriptWaitMS = 240000

	cliTerminalTextExpression = `(() => {
		const terminal = document.querySelector('.xterm')
		if (!terminal) return ''
		const parts = []
		const pushText = (node) => {
			const text = node?.textContent ?? ''
			if (text) parts.push(text)
		}
		pushText(terminal.querySelector('.xterm-accessibility-tree'))
		pushText(terminal.querySelector('.live-region'))
		pushText(terminal.querySelector('.xterm-rows'))
		return parts.join('\n').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim()
	})()`
)

var cliTerminalSessionHashRE = regexp.MustCompile(`#/u/(\d+)(?:$|/)`)

func TestGoScriptCliTerminalCommands(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if compiler != E2EWasmCompilerGoScript {
		t.Skipf("requires %s", E2EWasmCompilerGoScript)
	}

	h := cliTerminalGoScriptHarness(t)

	sess := h.NewCleanPageSession(t)
	crashConsole, stopCrashConsole := sess.WatchConsole()
	defer stopCrashConsole()
	defer func() {
		report := DrainCrashReport(crashConsole)
		if report.HasCrash() {
			t.Errorf("unexpected browser/WASM crash report during CLI terminal gate: %+v", report)
		}
		if report.HasExitedGoLoop() {
			t.Errorf("unexpected exited-Go loop during CLI terminal gate: %+v", report)
		}
	}()
	debugConsole, stopDebugConsole := sess.WatchConsole()
	consoleLog := newCliTerminalConsoleLog(debugConsole)
	defer func() {
		stopDebugConsole()
		consoleLog.Wait()
	}()

	page := sess.Page()
	AssertRootImportMap(t, h, page)
	AssertBrowserStartupDone(t, h, page)

	NavigateHash(t, h, page, "#/quickstart/local")
	sessionIndex := waitForCliTerminalLocalSession(t, page, consoleLog)

	NavigateHash(t, h, page, fmt.Sprintf("#/u/%s/settings/cli", sessionIndex))
	openCLIButton := page.Locator("button:has-text('Open CLI terminal')").First()
	if err := openCLIButton.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	}); err != nil {
		t.Fatalf("wait for Open CLI terminal button: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
	if err := openCLIButton.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	}); err != nil {
		t.Fatalf("click Open CLI terminal: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}

	waitForCliTerminalRoute(t, page, consoleLog)
	focusCliTerminal(t, page, consoleLog)
	waitForCliTerminalText(t, page, consoleLog, []string{"spacewave>"})

	type commandCase struct {
		command  string
		expects  []string
		oneOfAny []string
	}
	tests := []commandCase{
		{
			command: "status",
			expects: []string{
				"Status",
				"running",
				"Session Index",
				"Spaces",
			},
		},
		{
			command: "whoami",
			expects: []string{
				"Session",
				"Provider",
				"Account",
				"Lock",
			},
		},
		{
			command:  "space list",
			expects:  []string{"space list"},
			oneOfAny: []string{"no spaces", "ID", "NAME"},
		},
	}
	for _, tt := range tests {
		runCliTerminalCommand(t, page, consoleLog, tt.command)
		waitForCliTerminalCommandOutput(t, page, consoleLog, tt.command, tt.expects, tt.oneOfAny)
	}
}

func cliTerminalGoScriptHarness(t testing.TB) *Harness {
	t.Helper()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	manifestBuildTimeout, err := ResolveE2EWasmManifestBuildTimeout(20 * time.Minute)
	if err != nil {
		t.Fatalf("configure CLI terminal GoScript manifest build timeout: %v", err)
	}
	opts := []Option{
		WithSessionHarness(),
		WithManifestBuildTimeout(manifestBuildTimeout),
		WithGoScriptBrowserStartup(),
		WithConfigMutator(configureCliTerminalGoScriptStartup),
	}
	if E2EWasmTraceServiceEnabled(E2EWasmCompilerGoScript) {
		opts = append(opts, WithConfigMutator(trace_service.InjectTraceConfig))
	}

	h, err := Boot(context.Background(), le, opts...)
	if err != nil {
		t.Fatalf("boot CLI terminal GoScript harness: %v", err)
	}
	t.Cleanup(h.Release)

	if err := h.LaunchBrowser(); err != nil {
		t.Fatalf("launch CLI terminal GoScript browser: %v", err)
	}
	if err := h.CompileScripts("."); err != nil {
		t.Fatalf("compile CLI terminal e2e scripts: %v", err)
	}
	return h
}

func configureCliTerminalGoScriptStartup(conf *bldr_project.ProjectConfig) error {
	return ConfigureGoScriptForManifest("spacewave-cli-plugin")(conf)
}

func waitForCliTerminalLocalSession(t testing.TB, page playwright.Page, consoleLog *cliTerminalConsoleLog) string {
	t.Helper()

	_, err := page.WaitForFunction(`() => {
		const hash = window.location.hash
		return /^#\/u\/\d+\/?$/.test(hash)
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for local quickstart session route: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
	match := cliTerminalSessionHashRE.FindStringSubmatch(page.URL())
	if len(match) != 2 {
		t.Fatalf("local quickstart URL %q did not contain a session index; debug: %v", page.URL(), collectCliTerminalDebug(page, consoleLog))
	}
	return match[1]
}

func waitForCliTerminalRoute(t testing.TB, page playwright.Page, consoleLog *cliTerminalConsoleLog) {
	t.Helper()

	_, err := page.WaitForFunction(`() => window.location.hash.includes('/settings/cli/terminal')`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for CLI terminal route: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
	terminalHost := page.Locator(".xterm:visible").First()
	if err := terminalHost.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	}); err != nil {
		t.Fatalf("wait for CLI terminal host: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
}

func focusCliTerminal(t testing.TB, page playwright.Page, consoleLog *cliTerminalConsoleLog) {
	t.Helper()

	screen := page.Locator(".xterm:visible .xterm-screen").First()
	if err := screen.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	}); err != nil {
		t.Fatalf("wait for xterm screen: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
	if err := screen.Click(playwright.LocatorClickOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	}); err != nil {
		t.Fatalf("focus xterm screen: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
	_, err := page.WaitForFunction(`() => document.activeElement === document.querySelector('.xterm textarea')`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for xterm helper textarea focus: %v\ndebug: %v", err, collectCliTerminalDebug(page, consoleLog))
	}
}

func runCliTerminalCommand(t testing.TB, page playwright.Page, consoleLog *cliTerminalConsoleLog, command string) {
	t.Helper()

	if err := page.Keyboard().Type(command); err != nil {
		t.Fatalf("type CLI terminal command %q: %v\ndebug: %v", command, err, collectCliTerminalDebug(page, consoleLog))
	}
	if err := page.Keyboard().Press("Enter"); err != nil {
		t.Fatalf("submit CLI terminal command %q: %v\ndebug: %v", command, err, collectCliTerminalDebug(page, consoleLog))
	}
}

func waitForCliTerminalText(t testing.TB, page playwright.Page, consoleLog *cliTerminalConsoleLog, expects []string) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const expects = Array.isArray(arg) ? arg : []
		const terminalText = `+cliTerminalTextExpression+`
		return expects.every((expected) => terminalText.includes(expected))
	}`, expects, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for CLI terminal text %v: %v\ndebug: %v", expects, err, collectCliTerminalDebug(page, consoleLog))
	}
}

func waitForCliTerminalCommandOutput(
	t testing.TB,
	page playwright.Page,
	consoleLog *cliTerminalConsoleLog,
	command string,
	expects []string,
	oneOfAny []string,
) {
	t.Helper()

	_, err := page.WaitForFunction(`(arg) => {
		const [command, expects, oneOfAny] = Array.isArray(arg) ? arg : ['', [], []]
		const terminalText = `+cliTerminalTextExpression+`
		const commandAt = terminalText.lastIndexOf(command)
		if (commandAt < 0) return false
		const tail = terminalText.slice(commandAt)
		if (!expects.every((expected) => tail.includes(expected))) return false
		if (oneOfAny.length === 0) return true
		return oneOfAny.some((expected) => tail.includes(expected))
	}`, []any{command, expects, oneOfAny}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(cliTerminalGoScriptWaitMS),
	})
	if err != nil {
		t.Fatalf("wait for CLI terminal command %q output: %v\ndebug: %v", command, err, collectCliTerminalDebug(page, consoleLog))
	}
}

func collectCliTerminalDebug(page playwright.Page, consoleLog *cliTerminalConsoleLog) any {
	raw, err := page.Evaluate(`() => JSON.stringify({
		url: window.location.href,
		hash: window.location.hash,
		bootStatus: globalThis.__swBootStatus ?? null,
		startupMarks: (globalThis.__swStartupMarks ?? []).slice(-12).map((mark) => ({
			name: mark.name,
			label: mark.label,
			sequence: mark.sequence,
			detail: mark.detail?.label ?? mark.detail?.phase ?? mark.detail?.state ?? null,
		})),
		appText: document.querySelector('#bldr-root')?.textContent?.replace(/\s+/g, ' ').slice(0, 800) ?? '',
		terminalText: ` + cliTerminalTextExpression + `,
		terminalClass: document.querySelector('.xterm')?.className ?? null,
		terminalFocused: !!document.querySelector('.xterm.focus'),
		terminalTextareaFocused: document.activeElement === document.querySelector('.xterm textarea'),
		terminalTextareaValue: document.querySelector('.xterm textarea')?.value ?? null,
		terminalHTML: document.querySelector('.xterm')?.innerHTML?.slice(0, 1600) ?? null,
		buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
			text: button.textContent?.replace(/\s+/g, ' ').trim().slice(0, 120) ?? '',
			disabled: button.disabled,
		})),
	}, null, 2)`)
	if err != nil {
		return map[string]any{
			"error":                 err.Error(),
			"url":                   page.URL(),
			"recentConsoleMessages": consoleLog.Snapshot(),
		}
	}
	debug, ok := raw.(string)
	if !ok {
		return map[string]any{
			"debug":                 raw,
			"recentConsoleMessages": consoleLog.Snapshot(),
		}
	}
	return map[string]any{
		"page":                  strings.TrimSpace(debug),
		"recentConsoleMessages": consoleLog.Snapshot(),
	}
}

type cliTerminalConsoleLog struct {
	mu     sync.Mutex
	recent []string
	done   chan struct{}
}

func newCliTerminalConsoleLog(console <-chan string) *cliTerminalConsoleLog {
	log := &cliTerminalConsoleLog{done: make(chan struct{})}
	go func() {
		defer close(log.done)
		for msg := range console {
			log.mu.Lock()
			log.recent = append(log.recent, msg)
			if len(log.recent) > 20 {
				copy(log.recent, log.recent[len(log.recent)-20:])
				log.recent = log.recent[:20]
			}
			log.mu.Unlock()
		}
	}()
	return log
}

func (l *cliTerminalConsoleLog) Snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.recent...)
}

func (l *cliTerminalConsoleLog) Wait() {
	<-l.done
}
