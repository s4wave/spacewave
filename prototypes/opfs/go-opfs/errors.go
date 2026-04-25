//go:build js && wasm

package opfs

// DOMError represents a JavaScript DOMException or error from OPFS operations.
type DOMError struct {
	Message string
}

// Error implements the error interface.
func (e *DOMError) Error() string {
	return e.Message
}
