package s4wave_mount

import (
	"context"

	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// Target addresses one typed World object.
type Target struct {
	// SessionIdx is the Session index.
	SessionIdx uint32
	// Space is a Space ID or display name. An empty value selects the first
	// Space.
	Space string
	// ObjectKey is the object key, used when no resolver is given.
	ObjectKey string
	// ResolveObjectKey resolves the key against the mounted engine when set.
	ResolveObjectKey func(ctx context.Context, engine *sdk_engine.SDKEngine) (string, error)
}
