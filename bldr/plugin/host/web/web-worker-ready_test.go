package plugin_host_web

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
)

func TestWaitForWebWorkerReadyReturnsWhenReady(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{
		WebWorkers: []*web_document.WebWorkerStatus{{
			Id:    "plugin/test",
			Ready: true,
		}},
	})

	if err := waitForWebWorkerReady(context.Background(), ctr, "plugin/test"); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForWebWorkerReadyReturnsWhenWorkerDeleted(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{
		WebWorkers: []*web_document.WebWorkerStatus{{
			Id:      "plugin/test",
			Deleted: true,
		}},
	})

	if err := waitForWebWorkerReady(context.Background(), ctr, "plugin/test"); err == nil {
		t.Fatal("expected deleted worker error")
	}
}

func TestWaitForWebWorkerReadyReturnsWhenWorkerFailed(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{
		WebWorkers: []*web_document.WebWorkerStatus{{
			Id:            "plugin/test",
			Deleted:       true,
			Failed:        true,
			FailureReason: "fatal wasm exit",
		}},
	})

	err := waitForWebWorkerReady(context.Background(), ctr, "plugin/test")
	if err == nil {
		t.Fatal("expected failed worker error")
	}
	if got, want := err.Error(), "web worker failed before becoming ready: fatal wasm exit"; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}
	if !isWebWorkerFailureError(errors.Wrap(err, "track web document")) {
		t.Fatal("expected wrapped failure to retain worker failure classification")
	}
}

func TestWaitForWebWorkerReadyConsumesGenerationState(t *testing.T) {
	for _, tc := range []struct {
		name          string
		state         web_document.WebWorkerGenerationState
		failureReason string
		wantErr       string
		wantFailure   bool
	}{
		{
			name:          "terminal failure",
			state:         web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_TERMINAL_FAILURE,
			failureReason: "fatal wasm exit",
			wantErr:       "web worker terminal failure before becoming ready: fatal wasm exit",
			wantFailure:   true,
		},
		{
			name:        "startup timeout",
			state:       web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_STARTUP_TIMEOUT,
			wantErr:     "web worker startup timed out before becoming ready",
			wantFailure: true,
		},
		{
			name:    "hidden document lifecycle",
			state:   web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_LIFECYCLE_HIDDEN,
			wantErr: "web document hidden before worker became ready",
		},
		{
			name:    "controlled stream reset",
			state:   web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_CONTROLLED_STREAM_RESET,
			wantErr: "web worker stream reset before becoming ready",
		},
		{
			name:    "normal stop",
			state:   web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_NORMAL_STOP,
			wantErr: "web worker stopped before becoming ready",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
			ctr.SetValue(&web_document.WebDocumentStatus{
				WebWorkers: []*web_document.WebWorkerStatus{{
					Id:              "plugin/test",
					Failed:          tc.wantFailure,
					GenerationState: tc.state,
					FailureReason:   tc.failureReason,
				}},
			})

			err := waitForWebWorkerReady(context.Background(), ctr, "plugin/test")
			if err == nil {
				t.Fatal("expected generation state error")
			}
			if got := err.Error(); got != tc.wantErr {
				t.Fatalf("unexpected error: got %q want %q", got, tc.wantErr)
			}
			if isWebWorkerFailureError(errors.Wrap(err, "track web document")) != tc.wantFailure {
				t.Fatalf("unexpected failure classification for %s", tc.name)
			}
		})
	}
}

func TestWaitForWebWorkerReadyReturnsForCapabilityReadyGeneration(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{
		WebWorkers: []*web_document.WebWorkerStatus{{
			Id:              "plugin/test",
			GenerationState: web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_CAPABILITY_READY,
		}},
	})

	if err := waitForWebWorkerReady(context.Background(), ctr, "plugin/test"); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForWebWorkerReadyReturnsWhenDocumentClosed(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{Closed: true})

	if err := waitForWebWorkerReady(context.Background(), ctr, "plugin/test"); err == nil {
		t.Fatal("expected closed document error")
	}
}

func TestWaitForCreatedWebWorkerReadyRemovesUnreadyWorker(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{
		WebWorkers: []*web_document.WebWorkerStatus{{
			Id: "plugin/test",
		}},
	})
	worker := &testWebWorker{id: "plugin/test"}

	ready, err := waitForCreatedWebWorkerReadyWithTimeout(context.Background(), ctr, worker, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("expected unready worker to be recreated")
	}
	if !worker.removed {
		t.Fatal("expected unready worker to be removed")
	}
}

func TestWaitForCreatedWebWorkerReadyDoesNotRemoveReadyWorker(t *testing.T) {
	ctr := ccontainer.NewCContainer[*web_document.WebDocumentStatus](nil)
	ctr.SetValue(&web_document.WebDocumentStatus{
		WebWorkers: []*web_document.WebWorkerStatus{{
			Id:    "plugin/test",
			Ready: true,
		}},
	})
	worker := &testWebWorker{id: "plugin/test"}

	ready, err := waitForCreatedWebWorkerReadyWithTimeout(context.Background(), ctr, worker, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("expected ready worker")
	}
	if worker.removed {
		t.Fatal("did not expect ready worker to be removed")
	}
}

type testWebWorker struct {
	id      string
	removed bool
}

func (w *testWebWorker) GetId() string {
	return w.id
}

func (w *testWebWorker) GetDocumentId() string {
	return ""
}

func (w *testWebWorker) GetShared() bool {
	return false
}

func (w *testWebWorker) Remove(ctx context.Context) (bool, error) {
	w.removed = true
	return true, nil
}
