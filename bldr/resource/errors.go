package resource

import "errors"

var (
	// ErrResourceNotFound is returned when a requested resource does not exist.
	ErrResourceNotFound = &ResourceFailure{Code: ResourceFailureCode_RESOURCE_FAILURE_CODE_RESOURCE_NOT_FOUND, Message: "resource not found"}
	// ErrClientReleased is returned when attempting to operate on a released client.
	ErrClientReleased = &ResourceFailure{Code: ResourceFailureCode_RESOURCE_FAILURE_CODE_CLIENT_RELEASED, Message: "client was released"}
	// ErrInvalidResourceID is returned when a resource ID is invalid or out of bounds.
	ErrInvalidResourceID = &ResourceFailure{Code: ResourceFailureCode_RESOURCE_FAILURE_CODE_INVALID_RESOURCE_ID, Message: "invalid resource id"}
	// ErrInvalidClientID is returned when a client ID is invalid or out of bounds.
	ErrInvalidClientID = &ResourceFailure{Code: ResourceFailureCode_RESOURCE_FAILURE_CODE_INVALID_CLIENT_ID, Message: "invalid client id"}
	// ErrResourceOrClientReleased is returned when either the resource or client has been released.
	ErrResourceOrClientReleased = &ResourceFailure{Code: ResourceFailureCode_RESOURCE_FAILURE_CODE_RESOURCE_OR_CLIENT_RELEASED, Message: "resource or client was released"}
	// ErrNoResourceClientContext is returned if there was no ResourceClientContext.
	ErrNoResourceClientContext = errors.New("no resource client context")
)

// Error returns the diagnostic without assigning it lifecycle meaning.
func (e *ResourceFailure) Error() string {
	return e.GetMessage()
}

// Is matches lifecycle failures by code, including failures decoded from a peer.
func (e *ResourceFailure) Is(target error) bool {
	other, ok := target.(*ResourceFailure)
	return ok && e.GetCode() != ResourceFailureCode_RESOURCE_FAILURE_CODE_UNKNOWN && e.GetCode() == other.GetCode()
}

// FailureFromError preserves known codes and treats other errors as diagnostics.
func FailureFromError(err error) *ResourceFailure {
	if err == nil {
		return nil
	}
	if failure, ok := errors.AsType[*ResourceFailure](err); ok {
		return &ResourceFailure{Code: failure.GetCode(), Message: err.Error()}
	}
	return &ResourceFailure{Message: err.Error()}
}
