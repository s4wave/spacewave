package gdrivetest

// Failure is one injected response before the normal endpoint handles a call.
type Failure struct {
	// Status is the HTTP status to return.
	Status int
	// Reason is the Drive error reason.
	Reason string
	// Message is the Drive error message.
	Message string
}
