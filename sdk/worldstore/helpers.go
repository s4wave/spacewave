package worldstore

import (
	"fmt"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

var errNilWorldState = fmt.Errorf("worldstore: world state is required")

func errObjectNotFound(name string) error {
	return fmt.Errorf("worldstore: object %q does not exist; create it before opening", name)
}

// worldCursor aliases the bucket lookup cursor used by all world-backed
// object implementations.
type worldCursor = *bucket_lookup.Cursor
