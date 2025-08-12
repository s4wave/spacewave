# SQLite Key-Value Store

This package provides a SQLite-based implementation of the `kvtx.Store`.

## Build Tags

The implementation uses conditional compilation with build tags:

### CGO Version (mattn/go-sqlite3)

- **Build tag**: `cgo && !js && !wasip1`
- **Driver**: `github.com/mattn/go-sqlite3`
- **Database driver name**: `sqlite3`
- **Features**: C-based SQLite implementation, faster

### Pure Go Version (modernc.org/sqlite)

- **Build tag**: `!cgo && !js && !wasip1`
- **Driver**: `modernc.org/sqlite`
- **Database driver name**: `sqlite`
- **Features**: Pure Go implementation, no CGO dependencies

## High-Performance Iterator

The iterator implementation provides efficient seeking and iteration:

- Supports forward and reverse iteration with proper boundary handling
- Uses optimized SQL queries with LIMIT 1 for O(log n) seeking
- Uses SQLite's BLOB comparison for prefix scanning

## Usage

```go
import sqlite "github.com/aperturerobotics/hydra/store/kvtx/sqlite"

// Open a SQLite store (automatically selects CGO vs pure Go)
store, err := sqlite.Open("/path/to/database.sqlite", "table_name")
if err != nil {
    // handle error
}
defer store.Close()

// Use the store for transactions
ctx := context.Background()
tx, err := store.NewTransaction(ctx, true) // true for write transaction
if err != nil {
    // handle error
}
defer tx.Discard()

// Perform operations
err = tx.Set(ctx, []byte("key"), []byte("value"))
if err != nil {
    // handle error
}

// Iterate with prefix
iter := tx.Iterate(ctx, []byte("prefix:"), true, false)
defer iter.Close()

for iter.Next() {
    key := iter.Key()
    value, _ := iter.Value()
    // process key/value
}

// Commit the transaction
err = tx.Commit(ctx)
if err != nil {
    // handle error
}
```

## Schema

The SQLite implementation creates a simple key-value table:

```sql
CREATE TABLE IF NOT EXISTS table_name (
    key BLOB PRIMARY KEY,
    value BLOB
)
```

## Performance

- **Reads**: O(log n) via primary key index
- **Writes**: O(log n) with transaction batching
- **Iteration**: O(log n) seek + O(k) for k results
- **Prefix scans**: O(log n + k) where k is results in range

## Testing

Run tests:

```bash
# Test with CGO
CGO_ENABLED=1 go test

# Test without CGO
CGO_ENABLED=0 go test
```
