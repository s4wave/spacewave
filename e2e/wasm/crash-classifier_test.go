//go:build !js

package wasm

import "testing"

func TestCrashReportClassifiesGoFatalAndExitedGoLoop(t *testing.T) {
	var report CrashReport
	report.AddMessage("fatal error: found bad pointer in Go heap")
	report.AddMessage("runtime.throw({0x123, 0x456})")
	report.AddMessage("Go program has already exited")
	report.AddMessage("Go program has already exited")
	report.AddMessage("Go program has already exited")

	if !report.HasCrash() {
		t.Fatal("expected crash signal")
	}
	if !report.HasExitedGoLoop() {
		t.Fatal("expected exited-Go loop signal")
	}
	if got := len(report.GoFatalStackTrace); got != 2 {
		t.Fatalf("unexpected Go fatal line count: got %d want 2", got)
	}
}

func TestDrainCrashReportDoesNotWaitForFutureMessages(t *testing.T) {
	messages := make(chan string, 2)
	messages <- "page error: boom"
	messages <- "worker plugin/core error: failed"

	report := DrainCrashReport(messages)
	if !report.HasCrash() {
		t.Fatal("expected crash signal")
	}
	if got := len(report.PageErrors); got != 1 {
		t.Fatalf("unexpected page error count: got %d want 1", got)
	}
	if got := len(report.WorkerErrors); got != 1 {
		t.Fatalf("unexpected worker error count: got %d want 1", got)
	}
}
