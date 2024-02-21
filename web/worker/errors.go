package web_worker

import "errors"

var (
	// ErrEmptyWebWorkerID is returned if the web worker id was empty.
	ErrEmptyWebWorkerID = errors.New("empty web worker id")
)
