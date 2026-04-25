//go:build !js

package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"

	bdb "github.com/aperturerobotics/bbolt"
	kvfile "github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
)

// boltVolumeBucket is the bucket name used by hydra/volume/bolt.
const boltVolumeBucket = "hydra"

func main() {
	if err := run(); err != nil {
		_, _ = io.WriteString(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New(
			"usage: plugin-release export-kvfile --bolt-path /path/to/db --out /path/to/world.kvfile",
		)
	}

	switch os.Args[1] {
	case "export-kvfile":
		return runExportKVFile(os.Args[2:])
	default:
		return errors.Errorf("unknown command %q", os.Args[1])
	}
}

func runExportKVFile(args []string) error {
	fs := flag.NewFlagSet("export-kvfile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var boltPath string
	var outPath string
	if err := func() error {
		fs.StringVar(&boltPath, "bolt-path", "", "path to the bolt db")
		fs.StringVar(&outPath, "out", "", "path to the output kvfile")
		return fs.Parse(args)
	}(); err != nil {
		return errors.Wrap(err, "parse flags")
	}
	if boltPath == "" || outPath == "" {
		return errors.New(
			"usage: plugin-release export-kvfile --bolt-path /path/to/db --out /path/to/world.kvfile",
		)
	}

	if err := exportBoltToKVFile(context.Background(), boltPath, outPath); err != nil {
		return err
	}
	if _, err := verifyKVFile(outPath); err != nil {
		return err
	}
	return nil
}

func exportBoltToKVFile(ctx context.Context, boltPath, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return errors.Wrap(err, "mkdir output dir")
	}

	store, err := store_kvtx_bolt.Open(
		boltPath,
		0o644,
		&bdb.Options{ReadOnly: true},
		[]byte(boltVolumeBucket),
	)
	if err != nil {
		return errors.Wrap(err, "open bolt db")
	}
	defer store.GetDB().Close()

	f, err := os.Create(outPath)
	if err != nil {
		return errors.Wrap(err, "create kvfile")
	}
	if err := writeStoreKVFile(ctx, f, store); err != nil {
		_ = f.Close()
		_ = os.Remove(outPath)
		return errors.Wrap(err, "write kvfile")
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close kvfile")
	}
	return nil
}

func writeStoreKVFile(ctx context.Context, wr io.Writer, store kvtx.Store) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open read tx")
	}
	defer tx.Discard()

	kvwr := kvfile.NewWriter(wr)
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()
	for it.Next() {
		key := append([]byte(nil), it.Key()...)
		value, err := it.Value()
		if err != nil {
			return errors.Wrap(err, "read value")
		}
		if err := kvwr.WriteValue(key, bytes.NewReader(value)); err != nil {
			return errors.Wrap(err, "write value")
		}
	}
	if err := it.Err(); err != nil {
		return errors.Wrap(err, "iterate store")
	}
	if err := kvwr.Close(); err != nil {
		return errors.Wrap(err, "close kvfile writer")
	}
	return nil
}

func verifyKVFile(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, errors.Wrap(err, "open kvfile")
	}
	defer f.Close()

	rd, err := kvfile.BuildReaderWithFile(f)
	if err != nil {
		return 0, errors.Wrap(err, "build kvfile reader")
	}
	if rd.Size() == 0 {
		return 0, errors.New("kvfile is empty")
	}
	return rd.Size(), nil
}
