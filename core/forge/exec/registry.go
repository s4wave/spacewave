package space_exec

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// ErrUnknownConfigID is returned when the config ID has no registered handler.
var ErrUnknownConfigID = errors.New("unknown exec handler config ID")

// Registry maps exec config IDs to handler factories.
type Registry struct {
	factories map[string]HandlerFactory
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]HandlerFactory)}
}

// Register adds a handler factory for the given config ID.
func (r *Registry) Register(configID string, factory HandlerFactory) {
	r.factories[configID] = factory
}

// Lookup returns the factory for the given config ID, or nil if not found.
func (r *Registry) Lookup(configID string) HandlerFactory {
	return r.factories[configID]
}

// ConfigIDs returns all registered config IDs.
func (r *Registry) ConfigIDs() []string {
	ids := make([]string, 0, len(r.factories))
	for id := range r.factories {
		ids = append(ids, id)
	}
	return ids
}

// CreateHandler looks up and constructs a handler for the given config ID.
func (r *Registry) CreateHandler(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	handle forge_target.ExecControllerHandle,
	inputs forge_target.InputMap,
	configID string,
	configData []byte,
) (Handler, error) {
	factory := r.factories[configID]
	if factory == nil {
		return nil, errors.Wrap(ErrUnknownConfigID, configID)
	}
	return factory(ctx, le, ws, handle, inputs, configData)
}
