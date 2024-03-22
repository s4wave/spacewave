//go:build js

package main

import (
	"context"
	"encoding/binary"
	"os"

	// buffered_reader_at "github.com/aperturerobotics/bldr/util/buffered-reader-at"
	buffered_reader_at "github.com/aperturerobotics/bldr/util/buffered-reader-at"
	fetch_range "github.com/aperturerobotics/bldr/util/fetch-range"
	fetch "github.com/aperturerobotics/bldr/util/wasm-fetch"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := run(ctx, le); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, le *logrus.Entry) error {
	keepAliveHTTP := true
	rangeReader := fetch_range.NewFetchRangeReader(fileUrl, &fetch.Opts{
		Method:    "GET",
		KeepAlive: &keepAliveHTTP,
	})
	size, err := rangeReader.Size()
	if err != nil {
		return err
	}

	// minimum http request is for 512Kb
	cacheReader := buffered_reader_at.NewBufferedReaderAt(rangeReader, 1024*512)
	kvReader, err := kvfile.BuildReader(cacheReader, uint64(size))
	if err != nil {
		return err
	}

	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, 0)
	val, found, err := kvReader.Get(key)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("key was not found")
	}
	le.Infof("successfully read %v bytes from %s", len(val), fileUrl)

	return nil
}
