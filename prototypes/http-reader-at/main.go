//go:build !js

package main

import (
	"context"
	"encoding/binary"
	"net/http"
	"os"

	httplog "github.com/aperturerobotics/bifrost/http/log"
	buffered_reader_at "github.com/aperturerobotics/bldr/util/buffered-reader-at"
	http_range "github.com/aperturerobotics/bldr/util/http-range"
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
	fileUrl := "https://b2-alpha-dist.aperture.app/demo.kvfile"
	req, err := http.NewRequest("GET", fileUrl, nil)
	if err != nil {
		return err
	}

	seekingHTTP := http_range.NewHTTPRangeReader(req, httplog.ClientWithLogger(http.DefaultClient, le, true))

	size, err := seekingHTTP.Size()
	if err != nil {
		return err
	}
	if size < 1000 {
		return errors.Errorf("unexpected file size: %v", size)
	}

	// optimization: cache the ending of the file up front
	// kvfile does a lot of reads of the end of the file when querying
	cacheReader := buffered_reader_at.NewBufferedReaderAt(seekingHTTP, 4096)
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
