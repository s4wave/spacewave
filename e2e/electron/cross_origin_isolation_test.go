//go:build !skip_e2e && !js

package electron

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

const electronCOIWaitTimeout = 30 * time.Second

type electronCOISnapshot struct {
	Href                string
	CrossOriginIsolated bool
	SabAvailable        bool
	OpfsAvailable       bool
	WebLocksAvailable   bool
	WorkerComms         *electronWorkerCommsResult
	DetectionLine       string
	StartupLabels       []string
}

type electronWorkerCommsResult struct {
	Config              string
	CrossOriginIsolated bool
	SabAvailable        bool
	OpfsAvailable       bool
	WebLocksAvailable   bool
}

// TIER: nightly
func TestElectronRendererReportsCrossOriginIsolatedWorkerComms(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), electronCOIWaitTimeout)
	defer cancel()

	page, err := h.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	consoleMessages := make(chan string, 32)
	page.On("console", func(msg playwright.ConsoleMessage) {
		text := msg.Text()
		if !strings.Contains(text, "worker-comms: detected config") {
			return
		}
		select {
		case consoleMessages <- text:
		default:
		}
	})
	if _, err := page.Reload(playwright.PageReloadOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(float64(electronCOIWaitTimeout / time.Millisecond)),
	}); err != nil {
		t.Fatal(err)
	}

	snapshot := readElectronCOISnapshot(t, page, electronCOIWaitTimeout)
	consoleLine := waitForElectronCOIConsoleLine(t, consoleMessages)
	t.Logf("electron COI console: %s", consoleLine)
	t.Logf("electron COI detection: %s", snapshot.DetectionLine)

	if !snapshot.CrossOriginIsolated {
		t.Fatalf("expected renderer window.crossOriginIsolated=true; snapshot=%+v", snapshot)
	}
	if !snapshot.SabAvailable {
		t.Fatalf("expected renderer SharedArrayBuffer availability; snapshot=%+v", snapshot)
	}
	if snapshot.WorkerComms == nil {
		t.Fatalf("worker-comms detection mark missing; startup labels=%v; snapshot=%+v", snapshot.StartupLabels, snapshot)
	}
	if !snapshot.WorkerComms.CrossOriginIsolated {
		t.Fatalf("worker-comms detected crossOriginIsolated=false; snapshot=%+v", snapshot)
	}
	if !snapshot.WorkerComms.SabAvailable {
		t.Fatalf("worker-comms detected SharedArrayBuffer unavailable; snapshot=%+v", snapshot)
	}
	if snapshot.WorkerComms.Config != "B" && snapshot.WorkerComms.Config != "C" {
		t.Fatalf("worker-comms config = %q, want B or C for SAB-capable Electron; snapshot=%+v", snapshot.WorkerComms.Config, snapshot)
	}
	if snapshot.WorkerComms.OpfsAvailable && snapshot.WorkerComms.WebLocksAvailable && snapshot.WorkerComms.Config != "C" {
		t.Fatalf("worker-comms config = %q, want C when OPFS and Web Locks are available; snapshot=%+v", snapshot.WorkerComms.Config, snapshot)
	}
}

func readElectronCOISnapshot(t testing.TB, page interface {
	Evaluate(expression string, arg ...any) (any, error)
}, timeout time.Duration) electronCOISnapshot {
	t.Helper()

	raw, err := page.Evaluate(`async (arg) => {
		const timeoutMS = Array.isArray(arg) ? arg[0] : arg
		const deadline = Date.now() + timeoutMS
		const startupLabels = () => (globalThis.__swStartupMarks ?? []).map((mark) => mark.label)
		const findWorkerCommsMark = () => {
			const marks = globalThis.__swStartupMarks ?? []
			for (let i = marks.length - 1; i >= 0; i--) {
				const mark = marks[i]
				if (mark?.label === 'worker-comms.detected') {
					return mark
				}
			}
			return null

		}
		const detectSab = () => {
			try {
				if (typeof SharedArrayBuffer !== 'function') {
					return false
				}
				return new SharedArrayBuffer(8).byteLength === 8
			} catch {
				return false
			}
		}

		let mark = findWorkerCommsMark()
		while (!mark && Date.now() < deadline) {
			await new Promise((resolve) => setTimeout(resolve, 50))
			mark = findWorkerCommsMark()
		}

		const detail = mark?.detail ?? null
		return JSON.stringify({
			href: window.location.href,
			crossOriginIsolated: window.crossOriginIsolated === true,
			sabAvailable: detectSab(),
			opfsAvailable: typeof navigator.storage?.getDirectory === 'function',
			webLocksAvailable: !!navigator.locks && typeof navigator.locks.request === 'function',
			workerComms: detail ? {
				config: String(detail.config ?? ''),
				crossOriginIsolated: detail.crossOriginIsolated === true,
				sabAvailable: detail.sabAvailable === true,
				opfsAvailable: detail.opfsAvailable === true,
				webLocksAvailable: detail.webLocksAvailable === true,
			} : null,
			detectionLine: detail
				? `+"`worker-comms.detected config ${detail.config} crossOriginIsolated=${detail.crossOriginIsolated} sabAvailable=${detail.sabAvailable} opfsAvailable=${detail.opfsAvailable} webLocksAvailable=${detail.webLocksAvailable}`"+`
				: '',
			startupLabels: startupLabels(),
		})
	}`, int(timeout/time.Millisecond))
	if err != nil {
		t.Fatalf("read Electron COI snapshot: %v", err)
	}

	rawJSON, ok := raw.(string)
	if !ok {
		t.Fatalf("Electron COI snapshot returned %T, want JSON string", raw)
	}
	return parseElectronCOISnapshot(t, rawJSON)
}

func parseElectronCOISnapshot(t testing.TB, data string) electronCOISnapshot {
	t.Helper()

	var parser fastjson.Parser
	value, err := parser.Parse(data)
	if err != nil {
		t.Fatalf("parse Electron COI snapshot: %v; raw=%s", err, data)
	}
	startupLabelValues := value.GetArray("startupLabels")
	startupLabels := make([]string, 0, len(startupLabelValues))
	for _, startupLabel := range startupLabelValues {
		startupLabels = append(startupLabels, string(startupLabel.GetStringBytes()))
	}
	snapshot := electronCOISnapshot{
		Href:                string(value.GetStringBytes("href")),
		CrossOriginIsolated: value.GetBool("crossOriginIsolated"),
		SabAvailable:        value.GetBool("sabAvailable"),
		OpfsAvailable:       value.GetBool("opfsAvailable"),
		WebLocksAvailable:   value.GetBool("webLocksAvailable"),
		DetectionLine:       string(value.GetStringBytes("detectionLine")),
		StartupLabels:       startupLabels,
	}
	if workerComms := value.Get("workerComms"); workerComms != nil {
		snapshot.WorkerComms = &electronWorkerCommsResult{
			Config:              string(workerComms.GetStringBytes("config")),
			CrossOriginIsolated: workerComms.GetBool("crossOriginIsolated"),
			SabAvailable:        workerComms.GetBool("sabAvailable"),
			OpfsAvailable:       workerComms.GetBool("opfsAvailable"),
			WebLocksAvailable:   workerComms.GetBool("webLocksAvailable"),
		}
	}
	return snapshot
}

func waitForElectronCOIConsoleLine(t testing.TB, messages <-chan string) string {
	t.Helper()

	timer := time.NewTimer(electronCOIWaitTimeout)
	defer timer.Stop()
	for {
		select {
		case msg := <-messages:
			return msg
		case <-timer.C:
			t.Fatal("timed out waiting for worker-comms console line")
		}
	}
}
