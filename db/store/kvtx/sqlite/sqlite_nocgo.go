//go:build !cgo && !js && !wasip1 && !sqlite_purego_modernc

package store_kvtx_sqlite

import (
	"context"
	"database/sql"
	"os"

	purego "github.com/s4wave/spacewave/db/store/kvtx/sqlite/ncruces"
)

func open(ctx context.Context, path string, table string) (Store, error) {
	return purego.Open(ctx, path, table)
}

func openWithPragmas(ctx context.Context, path string, table string, pragmas Pragmas) (Store, error) {
	return purego.OpenWithPragmas(ctx, path, table, pragmas)
}

func openWithMode(ctx context.Context, path string, mode os.FileMode, table string) (Store, error) {
	return purego.OpenWithMode(ctx, path, mode, table)
}

func newStore(db *sql.DB, table string) (Store, error) {
	return purego.NewStore(db, table)
}
