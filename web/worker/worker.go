package web_worker

import "context"

// WebWorker is the reference to a web worker.
type WebWorker interface {
	// GetId returns the web worker id.
	GetId() string

	// GetDocumentId returns the id of the parent WebDocument.
	// May be empty.
	GetDocumentId() string

	// GetShared indicates this is a shared worker.
	GetShared() bool

	// Remove shuts down the WebWorker and unregisters it from the host.
	// Returns context.Canceled if ctx is canceled
	// Returns if the worker was confirmed removed.
	// Returns false, nil if the worker was not found.
	Remove(ctx context.Context) (bool, error)
}
