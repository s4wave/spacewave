package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// resolveLookupWorldEngine resolves a LookupWorldEngine directive.
func (c *Controller) resolveLookupWorldEngine(
	ctx context.Context,
	di directive.Instance,
	dir LookupWorldEngine,
) (directive.Resolver, error) {
	engineID := c.conf.GetEngineId()
	if engineID == "" {
		return nil, nil
	}
	if id := dir.LookupWorldEngineID(); id != "" && id != engineID {
		return nil, nil
	}

	return NewWorldEngineResolver(c)
}
