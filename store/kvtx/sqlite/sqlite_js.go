//go:build js || wasip1

package store_kvtx_sqlite

import (
	"context"
	"database/sql"
	"os"

	"github.com/aperturerobotics/hydra/store/kvtx/sqlite/wasm"
)

func open(ctx context.Context, path string, table string) (Store, error) {
	return wasm.Open(ctx, path, table)
}

func openWithMode(ctx context.Context, path string, mode os.FileMode, table string) (Store, error) {
	return wasm.OpenWithMode(ctx, path, mode, table)
}

func newStore(db *sql.DB, table string) (Store, error) {
	return wasm.NewStore(db, table)
}
