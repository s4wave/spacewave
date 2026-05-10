package plugin_host_web

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
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
