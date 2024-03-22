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

// openStaticVolume opens the static volume kvfile.
func openStaticVolume(assetsFS fs.FS) (io.ReaderAt, uint64, error) {
	f, err := assetsFS.Open("assets.kvfile")
	if err != nil {
		return nil, 0, err
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}

	return f.(io.ReaderAt), uint64(fi.Size()), nil
}
