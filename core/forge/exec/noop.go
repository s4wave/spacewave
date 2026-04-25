package space_exec

import (
	"context"

	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// NoopConfigID is the config ID for the noop handler.
const NoopConfigID = "space-exec/noop"

// noopHandler writes a log line and returns nil.
type noopHandler struct {
	handle forge_target.ExecControllerHandle
}

// Execute runs the noop handler.
func (h *noopHandler) Execute(ctx context.Context) error {
	if h.handle != nil {
		if err := h.handle.WriteLog(ctx, "info", "noop execution complete"); err != nil {
			return err
		}
	}
	return nil
}

// NewNoopHandler constructs a noop handler.
func NewNoopHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configData []byte,
) (Handler, error) {
	return &noopHandler{handle: handle}, nil
}

// RegisterNoop registers the noop handler in the registry.
func RegisterNoop(r *Registry) {
	r.Register(NoopConfigID, NewNoopHandler)
}

// _ is a type assertion
var _ Handler = (*noopHandler)(nil)
