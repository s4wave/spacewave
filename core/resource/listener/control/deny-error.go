//go:build !js

package control

import "strings"

// DenyError indicates that the peer explicitly denied the takeover.
// CLI callers use errors.As to distinguish this from generic RPC
// transport errors and print a clearer message.
type DenyError struct {
	// Reason is the peer-supplied denial reason.
	Reason string
}

// Error implements error.
func (e *DenyError) Error() string {
	if e.Reason == "" {
		return "takeover denied by peer"
	}
	return e.Reason
}

// extractDenyReason extracts the deny reason from an error string if
// it contains the DenyErrorMarker embedded by InvokeMethod.
func extractDenyReason(err error) (string, bool) {
	// Extract the peer denial marker and trim its reason for the caller.
	msg := err.Error()
	_, after, ok := strings.Cut(msg, DenyErrorMarker)
	if !ok {
		return "", false
	}
	reason := strings.TrimSpace(after)
	return reason, true
}
