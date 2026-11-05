package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"time"

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/block/packcopy"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// sqliteHeader starts every SQLite database file.
const sqliteHeader = "SQLite format 3\x00"

// volumeArgs configures reading blocks from Volume store files, which a copy
// asks before the packs: a Volume holds the blocks it has not uploaded, and
// often copies of uploaded ones, with their refs in its reference graph.
type volumeArgs struct {
	// files are the store files, in the order a copy asks them.
	files cli.StringSlice
	// table is the key-value table of a SQLite store file.
	table string
	// bucket is the bucket of a bolt store file.
	bucket string
}

// flags binds the Volume files' flags.
func (a *volumeArgs) flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{Name: "volume", Usage: "Volume store file, bolt or SQLite, read before the packs; repeat for each", Destination: &a.files},
		&cli.StringFlag{Name: "volume-table", Usage: "key-value table of a SQLite Volume file", Destination: &a.table},
		&cli.StringFlag{Name: "volume-bucket", Usage: "bucket of a bolt Volume file", Value: "hydra", Destination: &a.bucket},
	}
}

// open opens each file read-only and returns sources that serve its blocks
// with the refs its reference graph records, with the function that closes
// them. A block the graph does not track reads without refs, so the copy asks
// the next source for it.
//
// A SQLite file may be open in its writer at the same time: each read is a
// short transaction of its own, so the reader never holds the writer's log.
// A bolt file must be stopped, since its writer holds it exclusively.
func (a *volumeArgs) open(ctx context.Context) ([]packcopy.Source, func(), error) {
	// Collect the sources and what closes each.
	var sources []packcopy.Source
	var closers []func() error
	closeAll := func() {
		for _, c := range closers {
			_ = c()
		}
	}

	// Open each file and serve it through its reference graph.
	for _, path := range a.files.Value() {
		store, closeStore, err := a.openFile(ctx, path)
		if err != nil {
			closeAll()
			return nil, nil, errors.Wrapf(err, "--volume %s", path)
		}
		closers = append(closers, closeStore)
		blocks := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
		sources = append(sources, block_gc.NewGCStoreOps(blocks, volume_kvtx.OpenRefGraph(store)))
	}
	return sources, closeAll, nil
}

// openFile opens one store file read-only, telling its kind by its header.
func (a *volumeArgs) openFile(ctx context.Context, path string) (store_kvtx.Store, func() error, error) {
	// Read the header.
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	header := make([]byte, len(sqliteHeader))
	_, err = io.ReadFull(f, header)
	_ = f.Close()
	if err != nil {
		return nil, nil, err
	}

	// Open a SQLite file beside its writer.
	if bytes.Equal(header, []byte(sqliteHeader)) {
		if a.table == "" {
			return nil, nil, errors.New("set --volume-table for a SQLite file")
		}
		store, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, a.table, store_kvtx_sqlite.Pragmas{ReadOnly: true})
		if err != nil {
			return nil, nil, err
		}
		return store, store.Close, nil
	}

	// Open a stopped bolt file, failing fast while a writer holds it.
	db, err := bdb.Open(path, 0o444, &bdb.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return nil, nil, errors.Wrap(err, "open read-only; stop its writer first")
	}
	return store_kvtx_bolt.NewStore(db, []byte(a.bucket)), db.Close, nil
}
