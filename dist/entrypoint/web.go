//go:build js

package dist_entrypoint

import (
	"context"
	"io/fs"
	"os"
	"time"

	"github.com/aperturerobotics/bldr/banner"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	web_entrypoint_browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	web_runtime_bootstrap "github.com/aperturerobotics/bldr/web/runtime/bootstrap"
	"github.com/aperturerobotics/go-kvfile"
	buffered_reader_at "github.com/aperturerobotics/hydra/util/buffered-reader-at"
	fetch_range "github.com/aperturerobotics/hydra/util/http-range/fetch"
	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// 512KB
const httpRangeMinSize = 512 * 1024

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

	// get the init message from the bldr js runtime
	initm, err := web_entrypoint_browser.ReadInitMessage()
	if err != nil {
		le.WithError(err).Fatal("failed to read init message")
	}
	banner.WriteToConsole()

	startBrowserRuntimeStack := func(distBus *DistBus) ([]func(), error) {
		stack, err := web_runtime_bootstrap.StartRuntimeStack(
			distBus.GetContext(),
			le,
			distBus.GetBus(),
			web_runtime_bootstrap.RuntimeStackOpts{
				WebRuntimeID:      initm.GetWebRuntimeId(),
				MessagePort:       "BLDR_WEB_RUNTIME_CLIENT_OPEN",
				StartSqliteWorker: true,
				StaticResolver:    distBus.GetStaticResolver(),
			},
		)
		if err != nil {
			return nil, err
		}
		return []func(){stack.Release}, nil
	}

	startWebPluginHost := func(distBus *DistBus) ([]func(), error) {
		rel, err := web_runtime_bootstrap.StartPluginBrowserHost(
			distBus.GetContext(),
			distBus.GetBus(),
			distBus.GetStaticResolver(),
		)
		if err != nil {
			return nil, err
		}
		return []func(){rel}, nil
	}

	if err := func() error {
		distMeta, err := bldr_dist.UnmarshalDistMetaB58(distMetaB58)
		if err != nil {
			return err
		}

		err = Run(
			ctx,
			le,
			distMeta,
			assetsFS,
			initm.GetWebRuntimeId(),
			[]DistBusHook{
				startBrowserRuntimeStack,
			},
			[]DistBusHook{
				startWebPluginHost,
			},
		)
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
func newStaticBlockStoreReaderBuilder(le *logrus.Entry, assetsFS fs.FS, verbose bool) refcount.RefCountResolver[*kvfile.Reader] {
	return func(ctx context.Context, released func()) (*kvfile.Reader, func(), error) {
		// read the URL to fetch from the assets fs
		fetchUrlDat, err := fs.ReadFile(assetsFS, "assets.url")
		if err != nil {
			return nil, nil, err
		}
		fetchUrl := string(fetchUrlDat)
		if len(fetchUrl) == 0 {
			return nil, nil, errors.New("empty assets url")
		}

		// send http Range requests
		fetchReader := fetch_range.NewFetchRangeReader(
			le,
			fetchUrl,
			&fetch.Opts{
				Method: "GET",

				CommonOpts: fetch.CommonOpts{
					// The assets file has a hash in the filename.
					// We can force caching since the hash change will flush the cache.
					Cache: "force-cache",
				},
			},
			verbose,
		)

		totalSize, err := fetchReader.Size()
		if err != nil {
			return nil, nil, err
		}

		bufferReader := buffered_reader_at.NewBufferedReaderAt(fetchReader, httpRangeMinSize)
		rdr, err := kvfile.BuildReader(bufferReader, uint64(totalSize))
		if err != nil {
			return nil, nil, err
		}

		return rdr, nil, nil
	}
}
