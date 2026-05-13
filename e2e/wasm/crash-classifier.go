//go:build !js

package wasm

import "strings"

const exitedGoProgramMessage = "Go program has already exited"

// CrashReport classifies browser and worker messages captured during a WASM
// E2E scenario.
type CrashReport struct {
	GoFatalStackTrace []string
	PageErrors        []string
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
	if strings.Contains(lower, "page error:") {
		r.PageErrors = append(r.PageErrors, msg)
	}
	if strings.Contains(lower, "worker ") && strings.Contains(lower, " error:") {
		r.WorkerErrors = append(r.WorkerErrors, msg)
	}
}

// HasExitedGoLoop returns true once the same exited-Go symptom repeats enough
// times to be treated as recovery-loop evidence instead of a single close race.
func (r CrashReport) HasExitedGoLoop() bool {
	return r.ExitedGoCount >= 3
}

// HasCrash returns true when a primary crash signal was captured.
func (r CrashReport) HasCrash() bool {
	return len(r.GoFatalStackTrace) != 0 || len(r.PageErrors) != 0 || len(r.WorkerErrors) != 0
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
