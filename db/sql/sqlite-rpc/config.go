package sqlite_rpc

import (
	"context"
	"database/sql"
	"strings"

	sql_sqlite_wasm_rpc "github.com/s4wave/spacewave/db/sql/sqlite-wasm/rpc"
	"github.com/s4wave/spacewave/db/store/kvtx/sqlite/common"
)

// Config classifies SQLite bridge errors for the existing KV transaction owner.
type Config struct{}

// DriverName identifies the bridge; instance-bound callers use OpenStore.
func (Config) DriverName() string { return "sqlite-rpc" }

// OpenDSN preserves the database path supplied to its bridge.
func (Config) OpenDSN(path string) string { return path }

// Description describes the physical database reached through the bridge.
func (Config) Description() string { return "SQLite through an instance-bound RPC bridge" }

// IsBusyError identifies contention reported by a SQLite backend.
func (Config) IsBusyError(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked"))
}

// SyncWAL returns nil: the bridge flushes the database on its own terms.
func (Config) SyncWAL(string) error {
	return nil
}

// OpenStore opens the existing KVTX implementation over a supplied SQLite bridge.
// The bridge configures durability on every physical connection it creates.
func OpenStore(ctx context.Context, client sql_sqlite_wasm_rpc.SRPCSqliteBridgeClient, path, table string) (*common.Store[Config], error) {
	db := sql.OpenDB(NewConnector(client, path))
	return common.OpenDB(ctx, db, table, Config{})
}

// _ is a type assertion.
var _ common.SQLiteDriverConfig = Config{}
