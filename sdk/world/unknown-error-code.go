package s4wave_world

import "strconv"

// UnknownErrorCodeError is a World RPC code this build does not know. The
// server refused or failed the request for a reason a newer build defines.
type UnknownErrorCodeError struct {
	// Code is the code the response carried.
	Code WorldErrorCode
}

// Error returns the explanation.
func (e *UnknownErrorCodeError) Error() string {
	return "remote world request failed with unknown error code " + strconv.Itoa(int(e.Code))
}

// _ is a type assertion
var _ error = (*UnknownErrorCodeError)(nil)
