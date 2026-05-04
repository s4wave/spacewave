package plugin_host_web

import (
	"context"
	"testing"

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
