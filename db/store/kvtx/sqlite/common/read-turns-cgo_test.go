//go:build cgo

package common_test

import (
	"context"

	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite/cgo"
)

// openReadTurnStore uses the build's concrete driver for generic test helpers.
func openReadTurnStore(ctx context.Context, path, table string) (*sqlite.Store, error) {
	return sqlite.Open(ctx, path, table)
}
