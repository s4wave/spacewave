//go:build js

package main

import (
	"encoding/binary"
	"os"

	"github.com/aperturerobotics/go-kvfile"
	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/pkg/errors"
	buffered_reader_at "github.com/s4wave/spacewave/db/util/buffered-reader-at"
	fetch_range "github.com/s4wave/spacewave/db/util/http-range/fetch"
	"github.com/sirupsen/logrus"
)

func main() {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	if err := run(le); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run(le *logrus.Entry) error {
	keepAliveHTTP := true
	verbose := true
	rangeReader := fetch_range.NewFetchRangeReader(le, fileUrl, &fetch.Opts{
		Method: "GET",
		CommonOpts: fetch.CommonOpts{
			KeepAlive: &keepAliveHTTP,
		},
	}, verbose)
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
