package portal

import "errors"

var (
	// ErrNotAuthorized asks the operator to provision unattended access.
	ErrNotAuthorized = errors.New("plasma capture not authorized. Run ./cmd/spacewave-gst-encoder/spacewave-authorize-display from the checkout as the Plasma session user")
	// ErrSessionClosed reports that the portal ended the capture and input session.
	ErrSessionClosed = errors.New("plasma remote desktop session closed")
	// errRequestDenied reports a rejected or cancelled portal response.
	errRequestDenied = errors.New("portal request was rejected or cancelled")
)
