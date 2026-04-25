package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/world"
)

// resolveLookupWorldEngine resolves a LookupWorldEngine directive.
func (c *Controller) resolveLookupWorldEngine(
	ctx context.Context,
	di directive.Instance,
	dir world.LookupWorldEngine,
) (directive.Resolver, error) {
	engineID := c.engineID
	if engineID == "" {
		return nil, nil
	}
	if id := dir.LookupWorldEngineID(); id != "" && id != engineID {
		return nil, nil
	}

	return world.NewWorldEngineResolver(c)
}
