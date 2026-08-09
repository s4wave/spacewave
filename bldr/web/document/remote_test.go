package web_document

import (
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/sirupsen/logrus"
)

func TestRemoteWebWorkerStatusCarriesGenerationState(t *testing.T) {
	r := &Remote{
		documentID:  "document-1",
		le:          logrus.NewEntry(logrus.New()),
		ready:       true,
		snapshotCtr: ccontainer.NewCContainer[*WebDocumentStatus](nil),
	}

	dirty, err := r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:              "worker-1",
		Shared:          true,
		GenerationState: WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_STARTUP_RUNNING,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("expected worker generation status to dirty remote state")
	}

	r.updateStatusSnapshot()
	status := r.snapshotCtr.GetValue()
	if got := len(status.GetWebWorkers()); got != 1 {
		t.Fatalf("unexpected worker count: got %d want 1", got)
	}
	worker := status.GetWebWorkers()[0]
	if got, want := worker.GetGenerationState(), WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_STARTUP_RUNNING; got != want {
		t.Fatalf("unexpected generation state: got %s want %s", got, want)
	}

	dirty, err = r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:              "worker-1",
		Shared:          true,
		Ready:           true,
		GenerationState: WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_RUNNING,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("expected running generation status to dirty remote state")
	}

	r.updateStatusSnapshot()
	status = r.snapshotCtr.GetValue()
	worker = status.GetWebWorkers()[0]
	if !worker.GetReady() {
		t.Fatal("expected worker ready in remote snapshot")
	}
	if got, want := worker.GetGenerationState(), WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_RUNNING; got != want {
		t.Fatalf("unexpected generation state: got %s want %s", got, want)
	}
}

func TestRemoteWebWorkerStatusPreservesDeletedGenerationEventOnce(t *testing.T) {
	r := &Remote{
		documentID:  "document-1",
		le:          logrus.NewEntry(logrus.New()),
		ready:       true,
		snapshotCtr: ccontainer.NewCContainer[*WebDocumentStatus](nil),
	}

	if _, err := r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:              "worker-1",
		GenerationState: WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_STARTUP_RUNNING,
	}}); err != nil {
		t.Fatal(err)
	}

	dirty, err := r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:              "worker-1",
		Deleted:         true,
		FailureReason:   "StreamResetError: stream reset",
		GenerationState: WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_CONTROLLED_STREAM_RESET,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Fatal("expected deleted worker generation status to dirty remote state")
	}

	r.updateStatusSnapshot()
	status := r.snapshotCtr.GetValue()
	if got := len(status.GetWebWorkers()); got != 1 {
		t.Fatalf("unexpected deleted worker event count: got %d want 1", got)
	}
	worker := status.GetWebWorkers()[0]
	if !worker.GetDeleted() {
		t.Fatal("expected deleted worker event")
	}
	if worker.GetFailed() {
		t.Fatal("controlled stream reset should not be a terminal worker failure")
	}
	if got, want := worker.GetGenerationState(), WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_CONTROLLED_STREAM_RESET; got != want {
		t.Fatalf("unexpected generation state: got %s want %s", got, want)
	}
	if got, want := worker.GetFailureReason(), "StreamResetError: stream reset"; got != want {
		t.Fatalf("unexpected failure reason: got %q want %q", got, want)
	}

	r.updateStatusSnapshot()
	status = r.snapshotCtr.GetValue()
	if got := len(status.GetWebWorkers()); got != 0 {
		t.Fatalf("deleted worker event should be one-shot: got %d workers", got)
	}
}

func TestRemoteWebWorkerStaleGenerationDoesNotReplaceOrDeleteCurrentHandle(t *testing.T) {
	r := &Remote{
		documentID:  "document-1",
		le:          logrus.NewEntry(logrus.New()),
		ready:       true,
		snapshotCtr: ccontainer.NewCContainer[*WebDocumentStatus](nil),
	}

	if _, err := r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:         "worker-1",
		Generation: "generation-a",
	}}); err != nil {
		t.Fatal(err)
	}
	_, stale := r.lookupRemoteWebWorker("worker-1")

	if _, err := r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:         "worker-1",
		Generation: "generation-b",
	}}); err != nil {
		t.Fatal(err)
	}
	_, current := r.lookupRemoteWebWorker("worker-1")
	if current == stale {
		t.Fatal("replacement generation reused the stale worker handle")
	}
	if got, want := stale.GetGeneration(), "generation-a"; got != want {
		t.Fatalf("stale handle generation = %q, want %q", got, want)
	}
	if got, want := current.GetGeneration(), "generation-b"; got != want {
		t.Fatalf("current handle generation = %q, want %q", got, want)
	}

	if _, err := r.handleWebWorkerStatuses(false, []*WebWorkerStatus{{
		Id:         "worker-1",
		Generation: "generation-a",
		Deleted:    true,
	}}); err != nil {
		t.Fatal(err)
	}
	_, retained := r.lookupRemoteWebWorker("worker-1")
	if retained != current {
		t.Fatal("stale generation deletion removed the current worker handle")
	}
}
