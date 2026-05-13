package plugin_host_web

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
	web_worker "github.com/s4wave/spacewave/bldr/web/worker"
)

const webWorkerReadyTimeout = time.Second * 30

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
			if worker.GetFailed() {
				return webWorkerFailureError(worker, "web worker failed before becoming ready")
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

func waitForCreatedWebWorkerReady(ctx context.Context, docStatusCtr *ccontainer.CContainer[*web_document.WebDocumentStatus], worker web_worker.WebWorker) (bool, error) {
	return waitForCreatedWebWorkerReadyWithTimeout(ctx, docStatusCtr, worker, webWorkerReadyTimeout)
}

func waitForCreatedWebWorkerReadyWithTimeout(ctx context.Context, docStatusCtr *ccontainer.CContainer[*web_document.WebDocumentStatus], worker web_worker.WebWorker, timeout time.Duration) (bool, error) {
	readyCtx, readyCtxCancel := context.WithTimeout(ctx, timeout)
	defer readyCtxCancel()
	if err := waitForWebWorkerReady(readyCtx, docStatusCtr, worker.GetId()); err != nil {
		if ctx.Err() != nil {
			return false, context.Canceled
		}
		if err != context.DeadlineExceeded {
			return false, err
		}

		removeCtx, removeCtxCancel := context.WithTimeout(ctx, time.Second*3)
		defer removeCtxCancel()
		if _, err := worker.Remove(removeCtx); err != nil {
			if ctx.Err() != nil {
				return false, context.Canceled
			}
			return false, errors.Wrap(err, "remove unready web worker")
		}
		return false, nil
	}

	return true, nil
}

func webWorkerFailureError(worker *web_document.WebWorkerStatus, message string) error {
	if failureReason := worker.GetFailureReason(); failureReason != "" {
		return &webWorkerFailureErr{message: message + ": " + failureReason}
	}
	return &webWorkerFailureErr{message: message}
}

func isWebWorkerFailureError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := errors.Cause(err).(*webWorkerFailureErr)
	return ok
}

type webWorkerFailureErr struct {
	message string
}

func (e *webWorkerFailureErr) Error() string {
	return e.message
}
