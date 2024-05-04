//go:build js
// +build js

package dist_entrypoint

import (
	"context"
	"io/fs"
	"os"

	fetch "github.com/aperturerobotics/bifrost/util/js-fetch"
	"github.com/aperturerobotics/bldr/banner"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	web_entrypoint_browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	bldr_web_plugin_browser_controller "github.com/aperturerobotics/bldr/web/plugin/browser/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/go-kvfile"
	buffered_reader_at "github.com/aperturerobotics/hydra/util/buffered-reader-at"
	fetch_range "github.com/aperturerobotics/hydra/util/http-range/fetch"
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

	startWebRuntimeHost := func(distBus *DistBus) ([]func(), error) {
		// load the web runtime controller
		// communicates with the frontend
		ctx, b := distBus.GetContext(), distBus.GetBus()
		distBus.GetStaticResolver().AddFactory(browser.NewFactory(b))
		_, _, webRuntimeRef, err := loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(&browser.Config{
				WebRuntimeId: initm.GetWebRuntimeId(),
				MessagePort:  "BLDR_WEB_RUNTIME_CLIENT_OPEN",
			}),
			nil,
		)
		if err != nil {
			err = errors.Wrap(err, "start web runtime controller")
			return nil, err
		}

		return []func(){webRuntimeRef.Release}, nil
	}

	startWebPluginHost := func(distBus *DistBus) ([]func(), error) {
		// load the web plugin browser host controller
		// services any web plugins forwarding their request to the plugin host
		// starts the web plugin controller
		ctx, b := distBus.GetContext(), distBus.GetBus()
		distBus.GetStaticResolver().AddFactory(bldr_web_plugin_browser_controller.NewFactory(b))
		_, _, webPluginBrowserHostRef, err := loader.WaitExecControllerRunning(
			ctx,
			b,
			resolver.NewLoadControllerWithConfig(&bldr_web_plugin_browser_controller.Config{}),
			nil,
		)
		if err != nil {
			err = errors.Wrap(err, "start web plugin browser host controller")
			return nil, err
		}
		return []func(){webPluginBrowserHostRef.Release}, nil
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
			[]PostStartHook{
				startWebRuntimeHost,
				startWebPluginHost,
			},
		)
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

				// The assets file has a hash in the filename.
				// We can force caching since the hash change will flush the cache.
				Cache: "force-cache",
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
