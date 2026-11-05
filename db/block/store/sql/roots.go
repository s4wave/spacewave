package block_store_sql

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// The roots table holds the store's named roots. The store frees nothing by
// liveness, so a root only names a block for its holder: SetRoots moves the
// roots in the same SQL transaction as the blocks it inserts, comparing each
// root with its Previous, and needs no journal.

// GetRoot returns the block name holds, or nil when it is free or expired.
func (s *Store) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	ref, err := s.readRoot(ctx, s.db, name, false)
	return ref, err
}

// ListRoots calls fn with each held root whose name starts with prefix, in
// name order.
func (s *Store) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	// Query the names in the prefix's range.
	rows, err := s.db.QueryContext(ctx,
		"SELECT name, ref, expires FROM "+s.roots+" WHERE store_id = ? AND name >= ? ORDER BY name", // #nosec G202 -- NewStore quotes the table.
		s.storeID, []byte(prefix))
	if err != nil {
		return errors.Wrap(err, "list roots")
	}
	defer rows.Close()

	// Report each held one until the prefix ends.
	now := time.Now().UnixNano()
	for rows.Next() {
		var name, data []byte
		var expires int64
		if err := rows.Scan(&name, &data, &expires); err != nil {
			return errors.Wrap(err, "list roots")
		}
		if len(name) < len(prefix) || string(name[:len(prefix)]) != prefix {
			break
		}
		if expires != 0 && expires <= now {
			continue
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(data); err != nil {
			return errors.Wrapf(err, "decode root %s", name)
		}
		if err := fn(block.NamedRoot{Name: string(name), Ref: ref, Expires: expiresTime(expires)}); err != nil {
			return err
		}
	}
	return errors.Wrap(rows.Err(), "list roots")
}

// SetRoots inserts entries and moves the roots in one SQL transaction. Each
// root must hold its Previous, an expired root counting as free, or the
// change is refused with ErrRootMoved and nothing is written. Tombstones are
// refused.
func (s *Store) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Open the transaction.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin roots transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// Insert the entries, so a root never names a block the store lacks.
	if err := s.insert(ctx, tx, entries); err != nil {
		return err
	}

	// Compare and move each root.
	for _, root := range roots {
		held, err := s.readRoot(ctx, tx, root.Name, true)
		if err != nil {
			return err
		}
		if !refsEqual(held, root.Previous) {
			return block.RootMoved(root.Name)
		}
		if err := s.writeRoot(ctx, tx, root); err != nil {
			return err
		}
	}
	return errors.Wrap(tx.Commit(), "commit roots transaction")
}

// rowQuerier reads a row on the pool or in a transaction.
type rowQuerier interface {
	// QueryRowContext runs a query that returns at most one row.
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readRoot returns the block name holds, nil for a free or expired name. lock
// locks the row for the transaction's update.
func (s *Store) readRoot(ctx context.Context, db rowQuerier, name string, lock bool) (*block.BlockRef, error) {
	// Read the row.
	query := "SELECT ref, expires FROM " + s.roots + " WHERE store_id = ? AND name = ?" // #nosec G202 -- NewStore quotes the table.
	if lock {
		query += " FOR UPDATE"
	}
	var data []byte
	var expires int64
	err := db.QueryRowContext(ctx, query, s.storeID, []byte(name)).Scan(&data, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "read root %s", name)
	}

	// Treat an expired root as free.
	if expires != 0 && expires <= time.Now().UnixNano() {
		return nil, nil
	}
	ref := &block.BlockRef{}
	if err := ref.UnmarshalVT(data); err != nil {
		return nil, errors.Wrapf(err, "decode root %s", name)
	}
	return ref, nil
}

// writeRoot sets root's row, or deletes it when root releases the name.
func (s *Store) writeRoot(ctx context.Context, db execer, root block.NamedRoot) error {
	// Release the name.
	if root.Ref.GetEmpty() {
		query := "DELETE FROM " + s.roots + " WHERE store_id = ? AND name = ?" // #nosec G202 -- NewStore quotes the table.
		_, err := db.ExecContext(ctx, query, s.storeID, []byte(root.Name))
		return errors.Wrapf(err, "release root %s", root.Name)
	}

	// Set it.
	data, err := root.Ref.MarshalVT()
	if err != nil {
		return err
	}
	var expires int64
	if !root.Expires.IsZero() {
		expires = root.Expires.UnixNano()
	}
	query := "INSERT INTO " + s.roots + " (store_id, name, ref, expires) VALUES (?, ?, ?, ?)" + // #nosec G202 -- NewStore quotes the table.
		" ON DUPLICATE KEY UPDATE ref = VALUES(ref), expires = VALUES(expires)"
	_, err = db.ExecContext(ctx, query, s.storeID, []byte(root.Name), data, expires)
	return errors.Wrapf(err, "set root %s", root.Name)
}

// refsEqual reports whether a and b name the same block, empty matching empty.
func refsEqual(a, b *block.BlockRef) bool {
	if a.GetEmpty() || b.GetEmpty() {
		return a.GetEmpty() == b.GetEmpty()
	}
	return a.EqualsRef(b)
}

// expiresTime returns the time of expires, the zero time for none.
func expiresTime(expires int64) time.Time {
	if expires == 0 {
		return time.Time{}
	}
	return time.Unix(0, expires)
}
