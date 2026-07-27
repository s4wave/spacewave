//go:build !js

package releasewasm

import (
	"context"
	"encoding/base64"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

const releaseWasmStartupTraceEnv = "E2E_RELEASE_WASM_MANIFEST_STARTUP_TRACE"

const (
	startupTraceChunkSize = 256 * 1024
	startupTraceMissing   = "missing"
)

// releaseStartupTraceEnabled reports whether this run opted into capturing the
// startup trace.
func releaseStartupTraceEnabled() bool {
	return os.Getenv(releaseWasmStartupTraceEnv) == "1"
}

// applyReleaseStartupTraceEnv prepares the process for a trace run, before the
// harness builds anything.
//
// The capture reads the SharedWorker target through a browser-level CDP
// session, which Playwright provides for Chromium only, so firefox and webkit
// are rejected here rather than after a full build and boot. The instrumented
// build itself is selected by gocompiler.RuntimeStartupTraceEnv, so the capture
// opt-in sets it: an operator who asks for a trace gets a bundle that carries
// the callback instead of one that reports it missing.
func applyReleaseStartupTraceEnv() error {
	if !releaseStartupTraceEnabled() {
		return nil
	}
	name, err := releaseWasmBrowserName()
	if err != nil {
		return err
	}
	if name != "chromium" {
		return errors.Errorf(
			"%s=1 needs a browser CDP session, which only chromium provides; E2E_RELEASE_WASM_BROWSER is %s",
			releaseWasmStartupTraceEnv, name,
		)
	}
	if err := os.Setenv(gocompiler.RuntimeStartupTraceEnv, "1"); err != nil {
		return errors.Wrapf(err, "set %s from %s", gocompiler.RuntimeStartupTraceEnv, releaseWasmStartupTraceEnv)
	}
	return nil
}

// captureReleaseStartupTrace stops and reads the root distribution runtime
// trace from the SharedWorker target that owns the browser plugin host.
func captureReleaseStartupTrace(ctx context.Context, browser playwright.Browser) (_ []byte, retErr error) {
	cdp, err := browser.NewBrowserCDPSession()
	if err != nil {
		return nil, errors.Wrap(err, "create browser CDP session")
	}
	defer func() {
		if err := cdp.Detach(); err != nil {
			if retErr == nil {
				retErr = errors.Wrap(err, "detach browser CDP session")
			} else {
				retErr = errors.Wrapf(retErr, "detach browser CDP session: %v", err)
			}
		}
	}()

	raw, err := cdp.Send("Target.getTargets", map[string]any{})
	if err != nil {
		return nil, errors.Wrap(err, "list browser targets")
	}
	targets, err := startupTraceTargets(raw)
	if err != nil {
		return nil, err
	}

	var checked []string
	for _, target := range targets {
		data, found, err := captureStartupTraceTarget(ctx, cdp, target)
		if err != nil {
			return nil, errors.Wrapf(err, "capture startup trace from %s %s", target.kind, target.url)
		}
		checked = append(checked, target.kind+":"+target.url)
		if found {
			return data, nil
		}
	}
	return nil, errors.Errorf("startup trace callback not found; targets=%v", checked)
}

type startupTraceTarget struct {
	id   string
	kind string
	url  string
}

func startupTraceTargets(raw any) ([]startupTraceTarget, error) {
	result, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.Errorf("browser targets result has type %T", raw)
	}
	infos, ok := result["targetInfos"].([]any)
	if !ok {
		return nil, errors.Errorf("browser target infos has type %T", result["targetInfos"])
	}

	targets := make([]startupTraceTarget, 0, len(infos))
	for _, rawInfo := range infos {
		info, ok := rawInfo.(map[string]any)
		if !ok {
			return nil, errors.Errorf("browser target info has type %T", rawInfo)
		}
		kind, _ := info["type"].(string)
		if kind != "shared_worker" && kind != "worker" {
			continue
		}
		id, _ := info["targetId"].(string)
		url, _ := info["url"].(string)
		if id == "" {
			return nil, errors.Errorf("%s target has no targetId: %v", kind, info)
		}
		targets = append(targets, startupTraceTarget{id: id, kind: kind, url: url})
	}
	return targets, nil
}

func captureStartupTraceTarget(
	ctx context.Context,
	cdp playwright.CDPSession,
	target startupTraceTarget,
) (_ []byte, _ bool, retErr error) {
	raw, err := cdp.Send("Target.attachToTarget", map[string]any{
		"targetId": target.id,
		"flatten":  false,
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "attach to browser target")
	}
	result, ok := raw.(map[string]any)
	if !ok {
		return nil, false, errors.Errorf("attach target result has type %T", raw)
	}
	sessionID, _ := result["sessionId"].(string)
	if sessionID == "" {
		return nil, false, errors.Errorf("attach target result has no sessionId: %v", result)
	}
	defer func() {
		_, err := cdp.Send("Target.detachFromTarget", map[string]any{"sessionId": sessionID})
		if err == nil {
			return
		}
		if retErr == nil {
			retErr = errors.Wrap(err, "detach from browser target")
		} else {
			retErr = errors.Wrapf(retErr, "detach from browser target: %v", err)
		}
	}()

	traceStopped := true
	traceCompleted := false
	defer func() {
		if traceStopped && !traceCompleted {
			abortStartupTraceTarget(cdp, sessionID)
		}
	}()

	encodedLength, err := evaluateStartupTraceTarget(ctx, cdp, sessionID, 1, `(async () => {
		const stop = globalThis.BLDR_STOP_STARTUP_TRACE
		if (typeof stop !== 'function') return 'missing'
		const value = await stop()
		return value == null ? 'missing' : String(value)
	})()`)
	if err != nil {
		return nil, false, err
	}
	if encodedLength == startupTraceMissing {
		return nil, false, nil
	}
	length, err := strconv.Atoi(encodedLength)
	if err != nil || length <= 0 {
		return nil, false, errors.Errorf("startup trace encoded length %q is invalid", encodedLength)
	}

	var encoded strings.Builder
	encoded.Grow(length)
	requestID := 2
	for offset := 0; offset < length; offset += startupTraceChunkSize {
		size := min(startupTraceChunkSize, length-offset)
		expression := "globalThis.BLDR_READ_STARTUP_TRACE(" + strconv.Itoa(offset) + "," + strconv.Itoa(size) + ")"
		chunk, err := evaluateStartupTraceTarget(ctx, cdp, sessionID, requestID, expression)
		if err != nil {
			return nil, false, errors.Wrapf(err, "read startup trace at %d", offset)
		}
		if len(chunk) != size {
			return nil, false, errors.Errorf("startup trace chunk at %d has %d bytes, want %d", offset, len(chunk), size)
		}
		encoded.WriteString(chunk)
		requestID++
	}
	traceCompleted = true
	data, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil {
		return nil, false, errors.Wrap(err, "decode startup trace")
	}
	if len(data) == 0 {
		return nil, false, errors.New("startup trace is empty")
	}
	return data, true, nil
}

func abortStartupTraceTarget(cdp playwright.CDPSession, sessionID string) {
	const expression = `(() => {
		const read = globalThis.BLDR_READ_STARTUP_TRACE
		if (typeof read === 'function') read(-1, 0)
	})()`
	_, _ = cdp.Send("Target.sendMessageToTarget", map[string]any{
		"sessionId": sessionID,
		"message":   startupTraceEvaluateMessage(0, expression),
	})
}

func evaluateStartupTraceTarget(
	ctx context.Context,
	cdp playwright.CDPSession,
	sessionID string,
	requestID int,
	expression string,
) (string, error) {
	responses := make(chan string, 1)
	handler := func(params map[string]any) {
		if got, _ := params["sessionId"].(string); got != sessionID {
			return
		}
		message, _ := params["message"].(string)
		if startupTraceResponseID(message) == requestID {
			responses <- message
		}
	}
	cdp.On("Target.receivedMessageFromTarget", handler)
	defer cdp.RemoveListener("Target.receivedMessageFromTarget", handler)

	message := startupTraceEvaluateMessage(requestID, expression)
	if _, err := cdp.Send("Target.sendMessageToTarget", map[string]any{
		"sessionId": sessionID,
		"message":   message,
	}); err != nil {
		return "", errors.Wrap(err, "send Runtime.evaluate to browser target")
	}

	select {
	case response := <-responses:
		return parseStartupTraceEvaluation(response)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func startupTraceEvaluateMessage(requestID int, expression string) string {
	var arena fastjson.Arena
	root := arena.NewObject()
	root.Set("id", arena.NewNumberInt(requestID))
	root.Set("method", arena.NewString("Runtime.evaluate"))
	params := arena.NewObject()
	params.Set("expression", arena.NewString(expression))
	params.Set("returnByValue", arena.NewTrue())
	params.Set("awaitPromise", arena.NewTrue())
	root.Set("params", params)
	return string(root.MarshalTo(nil))
}

func startupTraceResponseID(message string) int {
	var parser fastjson.Parser
	value, err := parser.Parse(message)
	if err != nil {
		return 0
	}
	return value.GetInt("id")
}

func parseStartupTraceEvaluation(message string) (string, error) {
	var parser fastjson.Parser
	value, err := parser.Parse(message)
	if err != nil {
		return "", errors.Wrap(err, "parse Runtime.evaluate response")
	}
	if remoteErr := value.Get("error"); remoteErr != nil {
		return "", errors.Errorf("Runtime.evaluate failed: %s", remoteErr.String())
	}
	if exception := value.Get("result", "exceptionDetails"); exception != nil {
		return "", errors.Errorf("Runtime.evaluate exception: %s", exception.String())
	}
	result := value.Get("result", "result")
	if result == nil {
		return "", errors.Errorf("Runtime.evaluate response has no result: %s", message)
	}
	if resultType := string(result.GetStringBytes("type")); resultType != "string" {
		return "", errors.Errorf("Runtime.evaluate returned %q: %s", resultType, result.String())
	}
	return string(result.GetStringBytes("value")), nil
}
