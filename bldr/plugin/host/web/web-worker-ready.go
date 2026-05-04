package plugin_host_web

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
)

func waitForWebWorkerReady(ctx context.Context, docStatusCtr *ccontainer.CContainer[*web_document.WebDocumentStatus], webWorkerID string) error {
	var docStatus *web_document.WebDocumentStatus
	for {
		nextDocStatus, err := docStatusCtr.WaitValueChange(ctx, docStatus, nil)
		if err != nil {
			return err
		}
		docStatus = nextDocStatus
		if docStatus.GetClosed() {
			return errors.New("web document closed before worker became ready")
		}

		for _, worker := range docStatus.GetWebWorkers() {
			if worker.GetId() != webWorkerID {
				continue
			}
			if worker.GetDeleted() {
				return errors.New("web worker was removed before becoming ready")
			}
			if worker.GetReady() {
				return nil
			}
		}
	}
}
