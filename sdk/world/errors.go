package s4wave_world

import (
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

var (
	// ErrTypedObjectGrantRetired identifies acquisition prevented by retirement of
	// its exact granting World mount, independent of local Resource controls.
	ErrTypedObjectGrantRetired = errors.New("typed object granting World mount retired")
	// ErrRemotePluginUnavailable is the cause a PluginUnavailableError restored
	// from a World RPC code carries, since the wire does not carry the cause.
	ErrRemotePluginUnavailable = errors.New("remote plugin unavailable")
)

// GetError restores the typed rejection or sentinel from a World operation response.
func (r *ApplyWorldOpResponse) GetError() error {
	return operationError(r.GetErrorCode(), r.GetRejectionCode(), r.GetRejectionMessage())
}

// GetError restores the typed rejection or sentinel from an object operation response.
func (r *ApplyObjectOpResponse) GetError() error {
	return operationError(r.GetErrorCode(), r.GetRejectionCode(), r.GetRejectionMessage())
}

// operationError preserves application codes across the Resource RPC boundary.
func operationError(code WorldErrorCode, rejectionCode, message string) error {
	if rejectionCode != "" {
		return &world.OperationRejection{Code: rejectionCode, Message: message}
	}
	return ErrorFromCode(code)
}

// ErrorFromCode restores the typed error a World RPC code stands for. It
// returns nil only for WORLD_ERROR_CODE_UNSPECIFIED, so a refused or failed
// operation never reads as success: every other code, including one this
// build does not know, returns an error.
//
// The wire carries only the code, so the plugin errors come back with their
// identifying fields empty; callers classify them by type.
func ErrorFromCode(code WorldErrorCode) error {
	switch code {
	case WorldErrorCode_WORLD_ERROR_CODE_UNSPECIFIED:
		return nil
	case WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP:
		return errors.Wrap(world.ErrUnhandledOp, "remote world operation unhandled")
	case WorldErrorCode_WORLD_ERROR_CODE_TYPED_OBJECT_GRANT_RETIRED:
		return ErrTypedObjectGrantRetired
	case WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND:
		return world.ErrObjectNotFound
	case WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT:
		return &world.PluginOpTimeoutError{}
	case WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE:
		// The code means nothing was written, not that the plugin was never
		// sent the operation, so the restored error says it was sent: a
		// server passing it on then reports it as writing nothing only when
		// its own transaction is atomic.
		return &world.PluginUnavailableError{Sent: true, Err: ErrRemotePluginUnavailable}
	case WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING:
		return &world.PluginExecutableMissingError{}
	case WorldErrorCode_WORLD_ERROR_CODE_REPLAY_WAITS:
		return sobject.ErrReplayWaits
	}
	for _, c := range commitRefusals {
		if c.code == code {
			return &CommitRefusal{Code: code, sentinel: c.sentinel}
		}
	}
	return &UnknownErrorCodeError{Code: code}
}

// ObjectBodiesBatchRevisionError reports that paginated reads crossed World
// sequence numbers too many times to form one consistent result.
type ObjectBodiesBatchRevisionError struct {
	// Expected is the sequence number recorded from the first page.
	Expected uint64
	// Got is the sequence number observed on the mismatching page.
	Got uint64
	// Retries is the number of pagination restarts attempted.
	Retries int
}

// Error returns the inconsistent World sequence details.
func (e *ObjectBodiesBatchRevisionError) Error() string {
	return "object bodies batch World sequence changed from " +
		strconv.FormatUint(e.Expected, 10) + " to " +
		strconv.FormatUint(e.Got, 10) + " after " +
		strconv.Itoa(e.Retries) + " retries"
}
