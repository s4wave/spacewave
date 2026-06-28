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

func TestCrashReportIgnoresBenignNormalCloseTeardownAbort(t *testing.T) {
	// Stack shape captured from a real Config A/F shared-worker reload: the abort
	// is raised from the AbortSignal firing inside releaseAllResources, not from a
	// WebDocument.close frame, so the fixture must not depend on that frame.
	var report CrashReport
	report.AddMessage("page error: playwright: ERR_RPC_ABORT\n" +
		"Error: ERR_RPC_ABORT\n" +
		"    at ClientRPC.writePacket (usePromise.mjs)\n" +
		"    at ClientRPC.writeCallCancel (usePromise.mjs)\n" +
		"    at AbortSignal.onAbort (usePromise.mjs)\n" +
		"    at Client.clearPendingResourceReleases (CopyButton.mjs)\n" +
		"    at Client.releaseAllResources (CopyButton.mjs)\n" +
		"    at async Retry._execute (index.mjs)")

	if report.HasCrash() {
		t.Fatalf("normal-close teardown abort must not be a crash: %+v", report)
	}
	if got := len(report.PageErrors); got != 0 {
		t.Fatalf("unexpected page error count: got %d want 0", got)
	}
}

func TestCrashReportStillCatchesRealAbortPageError(t *testing.T) {
	var report CrashReport
	report.AddMessage("page error: playwright: ERR_RPC_ABORT during a live call")

	if !report.HasCrash() {
		t.Fatal("an ERR_RPC_ABORT outside resource-release teardown is still a crash")
	}
	if got := len(report.PageErrors); got != 1 {
		t.Fatalf("unexpected page error count: got %d want 1", got)
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
