package packcopy

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// Volume types of a pack index file.
const (
	// IndexSQLite is a SQLite file, the keys in one table.
	IndexSQLite = "sqlite"
	// IndexBolt is a bbolt file, the keys in one bucket.
	IndexBolt = "bolt"
	// IndexS4DB is an s4db file.
	IndexS4DB = "s4db"
)

// DefaultIndexBucket is the bucket a Bolt volume keeps its keys in.
const DefaultIndexBucket = "hydra"

// IndexFileOptions configures OpenIndexFile.
type IndexFileOptions struct {
	// Kind is the file's volume type; empty detects it from the file.
	Kind string
	// Table is the table of a SQLite file.
	Table string
	// Bucket is the bucket of a Bolt file; empty uses DefaultIndexBucket.
	Bucket string
	// SQLite configures a SQLite file.
	SQLite store_kvtx_sqlite.Pragmas
	// ReadOnly opens the file without writing it.
	ReadOnly bool
}

// OpenIndexFile opens the key space a pack index file holds: a copy of a
// volume, or a pack index store, of any volume type a backend keeps its pack
// indexes in. Each type keeps the keys unchanged, so the store's prefix finds
// its index in any of them.
func OpenIndexFile(ctx context.Context, path string, opts *IndexFileOptions) (kvtx.Store, func() error, error) {
	// Name the type, from the file when not given.
	kind := opts.Kind
	if kind == "" {
		var err error
		if kind, err = DetectIndexFile(path); err != nil {
			return nil, nil, err
		}
	}

	// Open it.
	switch kind {
	case IndexSQLite:
		pragmas := opts.SQLite
		pragmas.ReadOnly = pragmas.ReadOnly || opts.ReadOnly
		store, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, opts.Table, pragmas)
		if err != nil {
			return nil, nil, err
		}
		return store, store.Close, nil
	case IndexBolt:
		bucket := opts.Bucket
		if bucket == "" {
			bucket = DefaultIndexBucket
		}
		store, err := store_kvtx_bolt.Open(path, 0o644, &bdb.Options{ReadOnly: opts.ReadOnly, Timeout: time.Second, FreelistType: bdb.FreelistMapType}, []byte(bucket))
		if err != nil {
			return nil, nil, errors.Wrap(err, "open the bolt file, which its writer may hold")
		}
		return store, store.Close, nil
	case IndexS4DB:
		db, err := s4db.Open(path, s4db.Options{ReadOnly: opts.ReadOnly})
		if err != nil {
			return nil, nil, err
		}
		return db, db.Close, nil
	default:
		return nil, nil, errors.Errorf("unknown index file type %q: want %s, %s or %s", kind, IndexSQLite, IndexBolt, IndexS4DB)
	}
}

// Headers of the volume files.
var (
	// sqliteHeader opens a SQLite file.
	sqliteHeader = []byte("SQLite format 3\x00")
	// s4dbHeader opens an s4db file.
	s4dbHeader = []byte("S4WAVEDB")
)

// Bolt's first meta page: a page header of id, flags, count and overflow,
// then the meta's magic.
const (
	// boltMetaFlag marks a meta page.
	boltMetaFlag = 0x04
	// boltMagic is the meta's magic.
	boltMagic = 0xED0CDAED
)

// DetectIndexFile names the volume type of the file at path from its header.
func DetectIndexFile(path string) (string, error) {
	// Read the header.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b := make([]byte, 20)
	n, err := io.ReadFull(f, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", errors.Wrap(err, "read the index file's header")
	}
	b = b[:n]

	// Match each type's.
	switch {
	case bytes.HasPrefix(b, sqliteHeader):
		return IndexSQLite, nil
	case bytes.HasPrefix(b, s4dbHeader):
		return IndexS4DB, nil
	case n == 20 && binary.LittleEndian.Uint16(b[8:]) == boltMetaFlag && binary.LittleEndian.Uint32(b[16:]) == boltMagic:
		return IndexBolt, nil
	default:
		return "", errors.Errorf("%s is no SQLite, bbolt or s4db file", path)
	}
}
