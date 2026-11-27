package block_store_sql

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// The roots table holds the store's named roots. The store frees nothing by
// liveness, so a root only references a block for its holder: SetRoots moves
// the roots in the same SQL transaction as the blocks it inserts, comparing
// each root with its Previous, and needs no journal. The store keeps no
// leases: a root is held until a writer releases it, and the table's expires
// column, kept so tables created before leases moved to the Volume still
// accept inserts, always holds zero.

// GetRoot returns the block name holds, or nil when it is free.
func (s *Store) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	ref, err := s.readRoot(ctx, s.db, name, false)
	return ref, err
}

// ListRoots calls fn with each held root whose name starts with prefix, in
// name order.
func (s *Store) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	// Query the names in the prefix's range.
	rows, err := s.db.QueryContext(ctx,
		"SELECT name, ref FROM "+s.roots+" WHERE store_id = ? AND name >= ? ORDER BY name", // #nosec G202 -- NewStore quotes the table.
		s.storeID, []byte(prefix))
	if err != nil {
		return errors.Wrap(err, "list roots")
	}
	defer rows.Close()

	// Report each one until the prefix ends.
	for rows.Next() {
		var name, data []byte
		if err := rows.Scan(&name, &data); err != nil {
			return errors.Wrap(err, "list roots")
		}
		if len(name) < len(prefix) || string(name[:len(prefix)]) != prefix {
			break
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(data); err != nil {
			return errors.Wrapf(err, "decode root %s", name)
		}
		if err := fn(block.NamedRoot{Name: string(name), Ref: ref}); err != nil {
			return err
		}
	}
	return errors.Wrap(rows.Err(), "list roots")
}

// SetRoots inserts entries and moves the roots in one SQL transaction. Each
// root must hold its Previous, or the
// change is refused with ErrRootMoved and nothing is written. Tombstones are
// refused.
func (s *Store) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Open the transaction.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin roots transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// Insert the entries, so a root never references a block the store
	// lacks.
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

// readRoot returns the block name holds, nil for a free name. lock
// locks the row for the transaction's update.
func (s *Store) readRoot(ctx context.Context, db rowQuerier, name string, lock bool) (*block.BlockRef, error) {
	// Read the row.
	query := "SELECT ref FROM " + s.roots + " WHERE store_id = ? AND name = ?" // #nosec G202 -- NewStore quotes the table.
	if lock {
		query += " FOR UPDATE"
	}
	var data []byte
	err := db.QueryRowContext(ctx, query, s.storeID, []byte(name)).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "read root %s", name)
	}

	// Decode the stored ref.
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

	// Set the root's row.
	data, err := root.Ref.MarshalVT()
	if err != nil {
		return err
	}
	query := "INSERT INTO " + s.roots + " (store_id, name, ref, expires) VALUES (?, ?, ?, 0)" + // #nosec G202 -- NewStore quotes the table.
		" ON DUPLICATE KEY UPDATE ref = VALUES(ref), expires = 0"
	_, err = db.ExecContext(ctx, query, s.storeID, []byte(root.Name), data)
	return errors.Wrapf(err, "set root %s", root.Name)
}

// refsEqual reports whether a and b reference the same block, empty matching
// empty.
func refsEqual(a, b *block.BlockRef) bool {
	if a.GetEmpty() || b.GetEmpty() {
		return a.GetEmpty() == b.GetEmpty()
	}
	return a.EqualsRef(b)
}
