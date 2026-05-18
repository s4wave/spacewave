package web_document

import (
	"context"

	web_worker "github.com/s4wave/spacewave/bldr/web/worker"
)

// remoteWebWorker contains remote web worker information.
type remoteWebWorker struct {
	r               *Remote
	id              string
	document        string
	shared          bool
	ready           bool
	failed          bool
	failureReason   string
	generationState WebWorkerGenerationState
}

// buildRemoteWebWorker constructs a new remote WebWorker handle.
func (r *Remote) buildRemoteWebWorker(document string, status *WebWorkerStatus) *remoteWebWorker {
	return &remoteWebWorker{
		r:               r,
		id:              status.GetId(),
		document:        document,
		shared:          status.GetShared(),
		ready:           status.GetReady(),
		failed:          status.GetFailed(),
		failureReason:   status.GetFailureReason(),
		generationState: status.GetGenerationState(),
	}
}

// GetId returns the web worker id.
func (r *remoteWebWorker) GetId() string {
	return r.id
}

// GetDocumentId returns the id of the parent WebDocument.
// May be empty.
func (r *remoteWebWorker) GetDocumentId() string {
	return r.document
}

// GetShared indicates this is a shared worker.
func (r *remoteWebWorker) GetShared() bool {
	return r.shared
}

// Remove shuts down the WebWorker and unregisters it from the host.
// Returns context.Canceled if ctx is canceled
// Returns nil if the worker was not found.
func (r *remoteWebWorker) Remove(ctx context.Context) (bool, error) {
	resp, err := r.r.webDocument.RemoveWebWorker(ctx, &RemoveWebWorkerRequest{
		Id: r.id,
	})
	if err != nil {
		return false, err
	}
	return resp.GetRemoved(), nil
}

// _ is a type assertion
var _ web_worker.WebWorker = (*remoteWebWorker)(nil)
