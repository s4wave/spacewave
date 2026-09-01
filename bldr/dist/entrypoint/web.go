//go:build js

package dist_entrypoint

import (
	"context"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/banner"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_dist_assetpack "github.com/s4wave/spacewave/bldr/dist/assetpack"
	web_entrypoint_browser "github.com/s4wave/spacewave/bldr/web/entrypoint/browser"
	web_runtime_bootstrap "github.com/s4wave/spacewave/bldr/web/runtime/bootstrap"
	"github.com/s4wave/spacewave/db/block"
	buffered_reader_at "github.com/s4wave/spacewave/db/util/buffered-reader-at"
	fetch_range "github.com/s4wave/spacewave/db/util/http-range/fetch"
	"github.com/sirupsen/logrus"
)

// httpRangeMinSize controls the browser assets.kvfile read coalescing window.
// Browser release packs are tens of MiB, and startup walks enough of the pack
// index that 512 KiB pages can strand first render behind many CDN range
// round-trips. Keep the window large enough that cold startup does not depend
// on hundreds of small partial responses.
const httpRangeMinSize = 4 * 1024 * 1024

// Main runs the default main entrypoint for the web.
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
				WebRuntimeID:   initm.GetWebRuntimeId(),
				MessagePort:    "BLDR_WEB_RUNTIME_CLIENT_OPEN",
				StaticResolver: distBus.GetStaticResolver(),
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

// newStaticBlockStoreReaderBuilder creates the builder for the assets.kvfile block store reader.
func newStaticBlockStoreReaderBuilder(
	le *logrus.Entry,
	assetsFS fs.FS,
	verbose bool,
	rootRef *block.BlockRef,
) refcount.RefCountResolver[*kvfile.Reader] {
	return func(ctx context.Context, released func()) (*kvfile.Reader, func(), error) {
		partsData, partsErr := fs.ReadFile(assetsFS, "assets.parts")
		var parts []bldr_dist_assetpack.Part
		if partsErr == nil {
			var err error
			parts, err = bldr_dist_assetpack.UnmarshalParts(partsData)
			if err != nil {
				return nil, nil, err
			}
		} else if errors.Is(partsErr, fs.ErrNotExist) {
			fetchURLData, err := fs.ReadFile(assetsFS, "assets.url")
			if err != nil {
				return nil, nil, err
			}
			if len(fetchURLData) == 0 {
				return nil, nil, errors.New("empty assets url")
			}
			parts = []bldr_dist_assetpack.Part{{URL: string(fetchURLData)}}
		} else {
			return nil, nil, partsErr
		}

		buildReader := func(cacheMode string) (*kvfile.Reader, error) {
			readers := make([]io.ReaderAt, 0, len(parts))
			resolvedParts := make([]bldr_dist_assetpack.Part, len(parts))
			copy(resolvedParts, parts)
			for i, part := range resolvedParts {
				fetchReader := fetch_range.NewFetchRangeReader(
					le,
					part.URL,
					&fetch.Opts{
						Method:     "GET",
						CommonOpts: fetch.CommonOpts{Cache: cacheMode},
					},
					verbose,
				)
				if part.Size == 0 {
					totalSize, err := fetchReader.Size()
					if err != nil {
						return nil, err
					}
					resolvedParts[i].Size = totalSize
				}
				readers = append(readers, fetchReader)
			}
			joinedReader, err := bldr_dist_assetpack.NewReaderAt(resolvedParts, readers)
			if err != nil {
				return nil, err
			}
			bufferReader := buffered_reader_at.NewBufferedReaderAt(joinedReader, httpRangeMinSize)
			return kvfile.BuildReader(bufferReader, uint64(joinedReader.Size()))
		}

		rdr, err := buildReader("force-cache")
		if err != nil {
			return nil, nil, err
		}
		if err := validateStaticBlockStoreRoot(rdr, rootRef); err != nil {
			if errors.Is(err, block.ErrNotFound) {
				le.WithError(err).Warn("cached static block store is missing dist world root; refetching")
				rdr, err = buildReader("reload")
				if err != nil {
					return nil, nil, err
				}
				if err := validateStaticBlockStoreRoot(rdr, rootRef); err != nil {
					return nil, nil, err
				}
			} else {
				return nil, nil, err
			}
		}

		return rdr, nil, nil
	}
}
