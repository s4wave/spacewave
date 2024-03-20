//go:build js
// +build js

package dist_entrypoint

import (
	"context"
	"errors"
	"io/fs"
	"os"

	bldr_dist "github.com/aperturerobotics/bldr/dist"
	buffered_reader_at "github.com/aperturerobotics/bldr/util/buffered-reader-at"
	fetch_range "github.com/aperturerobotics/bldr/util/fetch-range"
	fetch "github.com/aperturerobotics/bldr/util/wasm-fetch"
	kvfile_compress "github.com/aperturerobotics/go-kvfile/compress"
	"github.com/aperturerobotics/util/ioseek"
	"github.com/sirupsen/logrus"
)

// Main runs the default main entrypoint the web.
func Main(distMetaB58 string, logLevel logrus.Level, assetsFS fs.FS) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    false,
		DisableTimestamp: false,
	})
	log.SetLevel(logLevel)
	le := logrus.NewEntry(log)

	// There is no os.Interrupt on js.
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	if err := func() error {
		distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
		if err != nil {
			return err
		}

		err = Run(ctx, le, distMeta, assetsFS)
		if err != context.Canceled {
			return err
		}
		return nil
	}(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// openStaticVolume opens the static volume kvfile.
func openStaticVolume(assetsFS fs.FS) (kvfile_compress.ReadSeekerAt, error) {
	// read the URL to fetch from the assets fs
	fetchUrlDat, err := fs.ReadFile(assetsFS, "assets.url")
	if err != nil {
		return nil, err
	}
	fetchUrl := string(fetchUrlDat)
	if len(fetchUrl) == 0 {
		return nil, errors.New("empty assets url")
	}

	// send http requests for at minimum 100Kb
	fetchReader := fetch_range.NewFetchRangeReader(fetchUrl, &fetch.Opts{Method: "GET"})
	totalSize, err := fetchReader.Size()
	if err != nil {
		return nil, err
	}

	bufferReader := buffered_reader_at.NewBufferedReaderAt(fetchReader, 102400)
	seekerReader := ioseek.NewReaderAtSeeker(bufferReader, totalSize)
	return seekerReader, nil
}
