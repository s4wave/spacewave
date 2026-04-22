//go:build !js

package dist_entrypoint

import (
	"context"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/util/refcount"
	fcolor "github.com/fatih/color"
	"github.com/s4wave/spacewave/bldr/banner"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/sirupsen/logrus"
)

// Main runs the default main entrypoint for a native program.
func Main(
	distMetaB58 string,
	logLevel logrus.Level,
	assetsFS fs.FS,
	commandBuilders []cli_entrypoint.BuildCommandsFunc,
) {
	if len(commandBuilders) != 0 && len(os.Args) > 1 {
		if err := func() error {
			distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
			if err != nil {
				return err
			}
			return runCliMain(distMeta, logLevel, assetsFS, commandBuilders)
		}(); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		return
	}

	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    false,
		DisableTimestamp: false,
	})
	log.SetLevel(logLevel)
	le := logrus.NewEntry(log)

	// Attach log file hooks from BLDR_LOG_FILE env var.
	if raw := os.Getenv("BLDR_LOG_FILE"); raw != "" {
		parts := strings.Split(raw, ",")
		specs, err := logfile.ParseLogFileSpecs(parts, time.Now())
		if err != nil {
			le.WithError(err).Warn("failed to parse BLDR_LOG_FILE")
		}
		if len(specs) != 0 {
			cleanup, err := logfile.AttachLogFiles(log, specs)
			if err != nil {
				le.WithError(err).Warn("failed to attach log files")
			}
			if cleanup != nil {
				defer cleanup()
			}
		}
	}

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

		err = Run(ctx, le, distMeta, assetsFS, "", nil, nil)
		if err != context.Canceled {
			return err
		}
		return nil
	}(); err != nil {
		le.WithError(err).Error("exiting with fatal error")
		ctxCancel()
		<-time.After(time.Millisecond * 100)
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
		fileSize := uint64(fi.Size()) //nolint:gosec

		rdr, err := kvfile.BuildReader(readerAt, fileSize)
		if err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		return rdr, func() { _ = f.Close() }, nil
	}
}
