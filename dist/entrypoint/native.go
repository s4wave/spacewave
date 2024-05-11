//go:build !js
// +build !js

package dist_entrypoint

import (
	"context"
	"io"
	"io/fs"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bldr/banner"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/refcount"
	fcolor "github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

// Main runs the default main entrypoint for a native program.
func Main(distMetaB58 string, logLevel logrus.Level, assetsFS fs.FS) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    false,
		DisableTimestamp: false,
	})
	log.SetLevel(logLevel)
	le := logrus.NewEntry(log)

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	if err := func() error {
		distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
		if err != nil {
			return err
		}

		// Print banner
		red := fcolor.New(fcolor.FgRed)
		red.Fprint(os.Stderr, banner.FormatBanner()+"\n")

		err = Run(ctx, le, distMeta, assetsFS, "", nil)
		if err != context.Canceled {
			return err
		}
		return nil
	}(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// newStaticBlockStoreReaderBuilder creates the builder for the assets.kvfile block store reader
func newStaticBlockStoreReaderBuilder(_ *logrus.Entry, assetsFS fs.FS, _ bool) refcount.RefCountResolver[*kvfile.Reader] {
	return func(ctx context.Context, released func()) (*kvfile.Reader, func(), error) {
		f, err := assetsFS.Open("assets.kvfile")
		if err != nil {
			return nil, nil, err
		}

		fi, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}

		readerAt := f.(io.ReaderAt)
		fileSize := uint64(fi.Size())

		rdr, err := kvfile.BuildReader(readerAt, fileSize)
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		return rdr, func() { _ = f.Close() }, nil
	}
}
