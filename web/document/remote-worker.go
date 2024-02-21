package web_document

import web_worker "github.com/aperturerobotics/bldr/web/worker"

// remoteWebWorker contains remote web worker information.
type remoteWebWorker struct {
	id       string
	document string
	shared   bool
}

// buildRemoteWebWorker constructs a new remote WebWorker handle.
func (r *Remote) buildRemoteWebWorker(id, document string, shared bool) *remoteWebWorker {
	return &remoteWebWorker{
		id:       id,
		document: document,
		shared:   shared,
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

// _ is a type assertion
var _ web_worker.WebWorker = (*remoteWebWorker)(nil)
