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
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	"github.com/s4wave/spacewave/bldr/entrypoint/compose"
	"github.com/s4wave/spacewave/bldr/entrypoint/storagepath"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// Main runs the default main entrypoint for a native program.
// Keep the native runtime's export data out of generated main packages.
//
//go:noinline
func Main(
	distMetaB58 string,
	logLevel logrus.Level,
	assetsFS fs.FS,
	composition *compose.Composition,
) {
	MainWithRunner(distMetaB58, logLevel, assetsFS, composition, nil)
}

// MainWithRunner retains native assets, logging, CLI dispatch and signals while
// allowing a native event loop to own the distribution's foreground lifetime.
func MainWithRunner(
	distMetaB58 string,
	logLevel logrus.Level,
	assetsFS fs.FS,
	composition *compose.Composition,
	runner NativeRunner,
) {
	assetsFS = nativeAssetsFS{FS: assetsFS, executable: os.Executable}

	if len(composition.Commands) != 0 && len(os.Args) > 1 {
		if err := func() error {
			distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
			if err != nil {
				return err
			}
			return runCliMain(distMeta, logLevel, assetsFS, composition)
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

	distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	projectID := distMeta.GetProjectId()

	// Resolve console log level from the env chain
	// (<PROJECT>_LOG_LEVEL / BLDR_LOG_LEVEL / compiled-in default).
	resolvedLevel := logfile.ResolveLogLevel(
		[]string{storagepath.LogLevelEnvVar(projectID), "BLDR_LOG_LEVEL"},
		logLevel,
	)
	log.SetLevel(resolvedLevel)
	le := logrus.NewEntry(log)

	// Attach log file hooks from BLDR_LOG_FILE env var when set.
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
				logfile.EnsureLoggerLevel(log, specs)
				defer cleanup()
			}
		}
	}

	// Auto-enable a DEBUG-level file hook under <storageRoot>/logs/.
	// EnableAutoDefault no-ops when BLDR_LOG_FILE is set, so the explicit
	// branch above takes precedence.
	if storageRoot, err := storagepath.DetermineStorageRoot(projectID); err == nil {
		cleanup, err := logfile.EnableAutoDefault(
			log,
			storageRoot,
			storagepath.LogRetentionDaysEnvVar(projectID),
			time.Now(),
		)
		if err != nil {
			le.WithError(err).Warn("failed to enable auto-default log file")
		}
		if cleanup != nil {
			defer cleanup()
		}
	}

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	// Print banner
	red := fcolor.New(fcolor.FgRed)
	red.Fprint(os.Stderr, banner.FormatBanner()+"\n")

	run := func(ctx context.Context, preBuildHooks, postStartHooks []DistBusHook) error {
		return Run(ctx, le, distMeta, assetsFS, "", composition, preBuildHooks, postStartHooks)
	}
	if runner == nil {
		runner = func(ctx context.Context, _ *logrus.Entry, run NativeRun) error {
			return run(ctx, nil, nil)
		}
	}
	if err := runner(ctx, le, run); err != nil && err != context.Canceled {
		le.WithError(err).Error("exiting with fatal error")
		ctxCancel()
		<-time.After(time.Millisecond * 100)
		os.Exit(1)
	}
}

// newStaticBlockStoreReaderBuilder creates the builder for the assets.kvfile block store reader.
func newStaticBlockStoreReaderBuilder(
	_ *logrus.Entry,
	assetsFS fs.FS,
	_ bool,
	rootRef *block.BlockRef,
) refcount.RefCountResolver[*kvfile.Reader] {
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
		if err := validateStaticBlockStoreRoot(rdr, rootRef); err != nil {
			_ = f.Close()
			return nil, nil, err
		}
		return rdr, func() { _ = f.Close() }, nil
	}
}
