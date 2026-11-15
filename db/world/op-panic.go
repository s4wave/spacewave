package world

import "fmt"

// OpPanicError is a panic recovered while decoding or applying an operation.
// A panic says nothing certain about the operation, as the handler's own
// code or the device's state may cause it, so it is never a refusal.
type OpPanicError struct {
	// Value is the value the panic carried.
	Value any
}

// Error returns the error message.
func (e *OpPanicError) Error() string {
	return fmt.Sprintf("operation panicked: %v", e.Value)
}
