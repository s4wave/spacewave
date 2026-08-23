package plugin_host_web

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
	web_worker "github.com/s4wave/spacewave/bldr/web/worker"
)

// webWorkerReadyTimeout must cover the browser's silent download, compile, and
// startup window for large Go wasm plugin workers. WebDocument currently
// advances to STARTUP_RUNNING before that phase and does not publish finer
// progress, so a short host timeout recreates a worker that may still be
// starting correctly.
const webWorkerReadyTimeout = time.Minute * 5

// waitForWebWorkerReady waits until the web worker becomes ready or closes,
// retrying transient startup failures.
func waitForWebWorkerReady(ctx context.Context, docStatusCtr *ccontainer.CContainer[*web_document.WebDocumentStatus], webWorkerID string) error {
	var docStatus *web_document.WebDocumentStatus
	for {
		nextDocStatus, err := docStatusCtr.WaitValueChange(ctx, docStatus, nil)
		if err != nil {
			return err
		}
		docStatus = nextDocStatus
		if docStatus.GetClosed() {
			return webWorkerStartupRetryError("web document closed before worker became ready")
		}

		for _, worker := range docStatus.GetWebWorkers() {
			if worker.GetId() != webWorkerID {
				continue
			}
			switch worker.GetGenerationState() {
			case web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_TERMINAL_FAILURE:
				return webWorkerFailureError(worker, "web worker terminal failure before becoming ready")
			case web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_STARTUP_TIMEOUT:
				return webWorkerFailureError(worker, "web worker startup timed out before becoming ready")
			case web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_LIFECYCLE_HIDDEN:
				return webWorkerStartupRetryError("web document hidden before worker became ready")
			case web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_CONTROLLED_STREAM_RESET:
				return webWorkerStartupRetryError("web worker stream reset before becoming ready")
			case web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_NORMAL_STOP:
				return webWorkerStartupRetryError("web worker stopped before becoming ready")
			case web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_RUNNING,
				web_document.WebWorkerGenerationState_WEB_WORKER_GENERATION_STATE_CAPABILITY_READY:
				return nil
			}
			if worker.GetFailed() {
				return webWorkerFailureError(worker, "web worker failed before becoming ready")
			}
			if worker.GetDeleted() {
				return webWorkerStartupRetryError("web worker was removed before becoming ready")
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

// waitForCreatedWebWorkerReadyWithTimeout waits for a created web worker to
// become ready, removing and reporting the worker on retryable failure.
func waitForCreatedWebWorkerReadyWithTimeout(ctx context.Context, docStatusCtr *ccontainer.CContainer[*web_document.WebDocumentStatus], worker web_worker.WebWorker, timeout time.Duration) (bool, error) {
	readyCtx, readyCtxCancel := context.WithTimeout(ctx, timeout)
	defer readyCtxCancel()
	if err := waitForWebWorkerReady(readyCtx, docStatusCtr, worker.GetId()); err != nil {
		if ctx.Err() != nil {
			return false, context.Canceled
		}
		if err != context.DeadlineExceeded {
			if !isWebWorkerStartupRetryError(err) {
				return false, err
			}
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

// webWorkerFailureError builds an error carrying the worker's failure
// reason.
func webWorkerFailureError(worker *web_document.WebWorkerStatus, message string) error {
	if failureReason := worker.GetFailureReason(); failureReason != "" {
		return &webWorkerFailureErr{message: message + ": " + failureReason}
	}
	return &webWorkerFailureErr{message: message}
}

// isWebWorkerFailureError reports whether err is a worker failure.
func isWebWorkerFailureError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := errors.Cause(err).(*webWorkerFailureErr)
	return ok
}

// webWorkerStartupRetryError builds a retryable startup failure error.
func webWorkerStartupRetryError(message string) error {
	return &webWorkerStartupRetryErr{message: message}
}

// isWebWorkerStartupRetryError reports whether err is a retryable startup
// failure.
func isWebWorkerStartupRetryError(err error) bool {
	if err == nil {
		return false
	}
	_, ok := errors.Cause(err).(*webWorkerStartupRetryErr)
	return ok
}

// webWorkerFailureErr is a non-retryable worker failure.
type webWorkerFailureErr struct {
	message string
}

func (e *webWorkerFailureErr) Error() string {
	return e.message
}

// webWorkerStartupRetryErr is a retryable worker startup failure.
type webWorkerStartupRetryErr struct {
	message string
}

func (e *webWorkerStartupRetryErr) Error() string {
	return e.message
}
