//go:build !js

package wasm

import "strings"

const exitedGoProgramMessage = "Go program has already exited"

// CrashReport classifies browser and worker messages captured during a WASM
// E2E scenario.
type CrashReport struct {
	GoFatalStackTrace []string
	PageErrors        []string
	RuntimeErrors     []string
	WorkerErrors      []string
	ExitedGoCount     int
}

// AddMessage records one browser or worker diagnostic line.
func (r *CrashReport) AddMessage(msg string) {
	lower := strings.ToLower(msg)
	if strings.Contains(msg, exitedGoProgramMessage) {
		r.ExitedGoCount++
	}
	if strings.Contains(lower, "fatal error:") || strings.Contains(lower, "runtime.throw") {
		r.GoFatalStackTrace = append(r.GoFatalStackTrace, msg)
	}
	if strings.Contains(lower, "page error:") && !isBenignTeardownAbort(lower) {
		r.PageErrors = append(r.PageErrors, msg)
	}
	if strings.Contains(lower, "uncaught rangeerror") ||
		strings.Contains(lower, "uncaught runtimeerror") ||
		strings.Contains(lower, "offset is outside the bounds of the dataview") ||
		strings.Contains(lower, "maximum call stack size exceeded") ||
		strings.Contains(lower, "memory access out of bounds") {
		r.RuntimeErrors = append(r.RuntimeErrors, msg)
	}
	if strings.Contains(lower, "worker ") && strings.Contains(lower, " error:") {
		r.WorkerErrors = append(r.WorkerErrors, msg)
	}
}

// isBenignTeardownAbort reports whether a page-error line is the document
// cancelling its own in-flight resource-release RPCs during a normal close.
// On a page reload the runtime client closes with normal-close and
// releaseAllResources aborts the pending release calls, which surfaces as an
// uncaught ERR_RPC_ABORT. This teardown abort appears only when the worker
// outlives the document (Config A/F shared worker), so it is benign and must
// not be classified as a crash.
//
// The two-token match is precise rather than coarse: releaseAllResources is the
// runtime client's teardown-only resource-release method, so an ERR_RPC_ABORT
// raised inside its frame is by construction the close race (a pending release
// call cancelled because the client is closing), not a product fault. A real
// crash surfaces with a different error code or outside the release path, which
// this guard leaves untouched. The argument is already lowercased.
func isBenignTeardownAbort(lower string) bool {
	return strings.Contains(lower, "err_rpc_abort") &&
		strings.Contains(lower, "releaseallresources")
}

// HasExitedGoLoop returns true once the same exited-Go symptom repeats enough
// times to be treated as recovery-loop evidence instead of a single close race.
func (r CrashReport) HasExitedGoLoop() bool {
	return r.ExitedGoCount >= 3
}

// HasCrash returns true when a primary crash signal was captured.
func (r CrashReport) HasCrash() bool {
	return len(r.GoFatalStackTrace) != 0 ||
		len(r.PageErrors) != 0 ||
		len(r.RuntimeErrors) != 0 ||
		len(r.WorkerErrors) != 0
}

// DrainCrashReport drains all currently buffered console messages into a
// report. It does not wait for future messages.
func DrainCrashReport(messages <-chan string) CrashReport {
	var report CrashReport
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				return report
			}
			report.AddMessage(msg)
		default:
			return report
		}
	}
}
