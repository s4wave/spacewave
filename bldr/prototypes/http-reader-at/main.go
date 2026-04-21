//go:build !js

package main

import (
	"context"
	"encoding/binary"
	"os"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	buffered_reader_at "github.com/s4wave/spacewave/db/util/buffered-reader-at"
	http_range "github.com/s4wave/spacewave/db/util/http-range"
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
	rangeHTTP, err := http_range.NewHTTPRangeReader(ctx, le, fileUrl, nil, false, true)
	if err != nil {
		return err
	}

	size, err := rangeHTTP.Size()
	if err != nil {
		return err
	}
	if size < 1000 {
		return errors.Errorf("unexpected file size: %v", size)
	}

	// optimization: cache the ending of the file up front
	// kvfile does a lot of reads of the end of the file when querying
	cacheReader := buffered_reader_at.NewBufferedReaderAt(rangeHTTP, 4096)
	kvReader, err := kvfile.BuildReader(cacheReader, size)
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
