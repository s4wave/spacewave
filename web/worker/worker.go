package web_worker

// WebWorker is the reference to a web worker.
type WebWorker interface {
	// GetId returns the web worker id.
	GetId() string

	// GetDocumentId returns the id of the parent WebDocument.
	// May be empty.
	GetDocumentId() string

	// GetShared indicates this is a shared worker.
	GetShared() bool
}
