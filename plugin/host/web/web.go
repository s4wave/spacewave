package plugin_host_web

import (
	"context"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/bifrost/util/randstring"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	web_document "github.com/aperturerobotics/bldr/web/document"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	web_worker "github.com/aperturerobotics/bldr/web/worker"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

// ControllerID is the process host controller ID.
const ControllerID = "bldr/plugin/host/web"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// WebHost implements the plugin host with WebWorker processes.
type WebHost struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
	// webRuntimeID is the identifier of the web runtime
	webRuntimeID string
}

// NewWebHost constructs a new WebHost.
func NewWebHost(b bus.Bus, le *logrus.Entry, webRuntimeID string) (*WebHost, error) {
	// determine the platform id for the host
	platformID := (&bldr_platform.WebPlatform{}).GetPlatformID()
	return &WebHost{
		b:                b,
		le:               le,
		pluginPlatformID: platformID,
		webRuntimeID:     webRuntimeID,
	}, nil
}

// NewWebHostController constructs the WebHost and PluginHost controller.
func NewWebHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *Config,
) (*host_controller.Controller, *WebHost, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	pluginHost, err := NewWebHost(b, le, c.GetWebRuntimeId())
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		c.GetHostConfig(),
		controller.NewInfo(ControllerID, Version, "plugin host with WebWorker processes"),
		pluginHost,
	)
	return hctrl, pluginHost, nil
}

// GetPlatformId returns the plugin platform ID for this host.
// Return empty if the host accepts any platform ID.
func (h *WebHost) GetPlatformId(ctx context.Context) (string, error) {
	return h.pluginPlatformID, nil
}

// ListPlugins lists the set of initialized plugins.
func (h *WebHost) ListPlugins(ctx context.Context) ([]string, error) {
	// TODO list stored plugins or temporary storage
	// the plugin host will call Delete for any unrecognized
	return nil, nil
}

// ExecutePlugin executes the plugin with the given ID.
// If the plugin was already initialized, existing state can be reused.
// The plugin should be stopped if/when the function exits.
// Return ErrPluginUninitialized if the plugin was not ready.
// Should expect to be called only once (at a time) for a plugin ID.
// pluginDist contains the plugin distribution files (binaries and assets).
func (h *WebHost) ExecutePlugin(
	rctx context.Context,
	pluginID, entrypoint string,
	pluginDist *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	h.le.Info("XXX Web ExecutePlugin Start")
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// restrict to .mjs and .js only
	if !strings.HasSuffix(entrypoint, ".mjs") && !strings.HasSuffix(entrypoint, ".js") {
		return errors.Errorf("entrypoint must have a .mjs or .js extension: %s", entrypoint)
	}

	// double-check the entrypoint exists and is executable
	entrypoint = filepath.Clean(entrypoint)
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}

	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}

	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be an executable regular file: %s", entrypointFiMode.String())
	}

	entrypointHttpPath := plugin.PluginDistHTTPPath(pluginID, entrypoint)

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(4)
	pluginStartInfo := &plugin.PluginStartInfo{
		InstanceId: pluginInstanceID,
	}
	pluginStartInfoB58 := pluginStartInfo.MarshalB58()
	pluginStartInfoBin := []byte(pluginStartInfoB58)

	// web worker create request
	pluginWebWorkerID := "plugin/" + pluginID
	pluginWebWorkerURL := entrypointHttpPath
	pluginShared := true

	// stderr: contains any logs
	// TODO: logging?
	// le := h.le.WithField("plugin-id", pluginID)
	// debugWriter := le.WriterLevel(logrus.DebugLevel)
	// entrypointProc.Stderr = debugWriter

	webRuntime, _, webRuntimeRef, err := web_runtime.ExLookupWebRuntime(ctx, h.b, false, h.webRuntimeID)
	if err != nil {
		return err
	}
	defer webRuntimeRef.Release()

	h.le.
		WithField("entrypoint", entrypoint).
		WithField("web-runtime", h.webRuntimeID).
		Debugf("executing plugin entrypoint via http: %s", pluginWebWorkerURL)

	// Mount the RPC handler to the bus.
	baseControllerID := ControllerID + "/" + pluginID
	rpcServiceControllerID := baseControllerID + "/rpc-host"
	var hostInvoker srpc.Invoker = hostMux
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(rpcServiceControllerID, Version, "rpc host for plugin"),
		func(ctx context.Context, released func()) (srpc.Invoker, func(), error) {
			return hostInvoker, nil, nil
		},
		nil,
		false,
		nil,
		nil,
		regexp.MustCompile("^"+regexp.QuoteMeta("web-worker/"+pluginWebWorkerID)+"$"),
	)
	relRpcServiceCtrl, err := h.b.AddController(ctx, rpcServiceCtrl, nil)
	if err != nil {
		return err
	}
	defer relRpcServiceCtrl()

	// Initialize the rpc client for calling the plugin.
	pluginRpcClient := srpc.NewClient(webRuntime.GetWebWorkerOpenStream(pluginWebWorkerID))
	if err := rpcInit(pluginRpcClient); err != nil {
		return err
	}

	// There are two operating modes for the below code:
	// 1. SharedWorker is supported:
	//    - Watch all of the WebDocument
	//    - Create a SharedWorker on each web document
	//    - If unable to create a shared worker (created Worker instead):
	// 2. Worker is supported but SharedWorker is not:
	//    - Mark that we do not support SharedWorker and at least 1 instance is running.
	//    - Skip creating the other worker instances if at least 1 is running
	//    - When that 1 instance exits, mark not running, then restart all web doc trackers.
	// If any web documents cannot create shared workers, assume all cannot.

	sema := semaphore.NewWeighted(1)
	var singletonWorkerDoc string

	// Create the web worker on each document.
	var webDocumentsKeyed *keyed.Keyed[string, struct{}]
	wakeOtherWebDocs := func(otherThanDoc string) {
		_, _ = webDocumentsKeyed.RestartAllRoutines(func(docID string, _ struct{}) bool {
			return docID != otherThanDoc
		})
	}

	createWorkerWithDoc := func(ctx context.Context, doc web_document.WebDocument) error {
		if err := sema.Acquire(ctx, 1); err != nil {
			return err
		}
		defer sema.Release(1)

		webDocumentID := doc.GetWebDocumentUuid()
		if singletonWorkerDoc == webDocumentID {
			// If the previous singleton worker instance was ours, remove it.
			singletonWorkerDoc = ""

			// Wake the other WebDocument trackers in case we fail to start a worker.
			wakeOtherWebDocs(webDocumentID)
		} else if singletonWorkerDoc != "" {
			// An instance is already running, exit now.
			return nil
		}

		le := h.le.
			WithFields(logrus.Fields{
				"web-document": webDocumentID,
				"web-runtime":  h.webRuntimeID,
				"web-worker":   pluginWebWorkerID,
			})
		le.Debug("creating web worker")
		createdWorker, err := doc.CreateWebWorker(ctx, &web_document.CreateWebWorkerRequest{
			Id:       pluginWebWorkerID,
			Url:      pluginWebWorkerURL,
			Shared:   pluginShared,
			InitData: pluginStartInfoBin,
		})
		if createdWorker == nil && err == nil {
			err = errors.New("document did not create the worker")
		}
		if err != nil {
			le.WithError(err).Warn("unable to create web worker")
			return err
		}

		createdShared := createdWorker.GetShared()
		le.
			WithField("web-worker-shared", createdShared).
			Debug("successfully created web worker")

		// If we cannot create shared workers, create only one Worker to avoid duplicates.
		if !createdShared {
			singletonWorkerDoc = webDocumentID
		}

		return nil
	}

	removeWorkerInstances := func(ctx context.Context, doc web_document.WebDocument) (map[string]web_worker.WebWorker, error) {
		// Remove any old instances of the web worker.
		docWebWorkers, err := doc.GetWebWorkers(ctx)
		if err != nil {
			return nil, err
		}

		docWebWorkers = maps.Clone(docWebWorkers)
		for id, worker := range docWebWorkers {
			if worker.GetId() != pluginWebWorkerID {
				delete(docWebWorkers, id)
				continue
			}

			h.le.
				WithFields(logrus.Fields{
					"web-document": doc.GetWebDocumentUuid(),
					"web-runtime":  h.webRuntimeID,
					"web-worker":   pluginWebWorkerID,
				}).
				Debug("removing old instance of web worker")
			_, err := worker.Remove(ctx)
			if err != nil {
				h.le.WithError(err).Warn("unable to remove old web worker instance")
			}
		}

		return docWebWorkers, nil
	}

	// Track web document is called for each of the running web documents.
	trackWebDocument := func(ctx context.Context, webDocumentID string) error {
		// Get the web document.
		doc, err := webRuntime.GetWebDocument(ctx, webDocumentID, true)
		if err != nil {
			return err
		}

		// Remove any old instances of the web worker.
		cleanupCtx, cleanupCtxCancel := context.WithTimeout(ctx, time.Second*3)
		defer cleanupCtxCancel()

		for {
			if cleanupCtx.Err() != nil {
				break
			}

			removedInstances, err := removeWorkerInstances(ctx, doc)
			if err != nil {
				return err
			}
			if len(removedInstances) == 0 {
				break
			}

			select {
			case <-cleanupCtx.Done():
			case <-time.After(time.Millisecond * 100):
			}
		}

		cleanupCtxCancel()
		if ctx.Err() != nil {
			return context.Canceled
		}

		// Watch the list of web workers to ensure ours is running.
		docStatusCtr := doc.GetWebDocumentStatusCtr()
		var docStatus *web_document.WebDocumentStatus
		var workerInstance *web_document.WebWorkerStatus
		for {
			// Create the instance of the worker if it doesn't exist.
			if workerInstance == nil {
				if err := createWorkerWithDoc(ctx, doc); err != nil {
					return err
				}
			}

			docStatus, err = docStatusCtr.WaitValueChange(ctx, docStatus, nil)
			if err != nil {
				return err
			}
			if docStatus.GetClosed() {
				return nil
			}

			workers := docStatus.GetWebWorkers()
			for _, worker := range workers {
				if worker.GetId() == pluginWebWorkerID {
					workerInstance = worker
					break
				}
			}
		}
	}

	// fully kill & wait for exit to be confirmed when returning
	cleanupInstances := func() error {
		ctx, ctxCancel := context.WithTimeout(context.WithoutCancel(rctx), time.Second*3)
		defer ctxCancel()

		for {
			if err := ctx.Err(); err != nil {
				return err
			}

			docs, err := webRuntime.GetWebDocuments(ctx)
			if err != nil {
				return err
			}

			var retErr error
			var nOldInstances int
			for _, doc := range docs {
				oldInstances, err := removeWorkerInstances(ctx, doc)
				if err != nil {
					retErr = err
				}
				nOldInstances += len(oldInstances)
			}
			if retErr != nil {
				return retErr
			}

			if nOldInstances == 0 {
				// success
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Millisecond * 100):
			}
		}
	}
	defer func() {
		ctxCancel()
		if err := cleanupInstances(); err != nil {
			h.le.WithError(err).Warn("unable to cleanup old web worker instances")
		}
	}()

	// Track the list of web documents.
	webDocumentsKeyed = keyed.NewKeyedWithLogger(
		func(webDocumentId string) (keyed.Routine, struct{}) {
			return func(ctx context.Context) error {
				return trackWebDocument(ctx, webDocumentId)
			}, struct{}{}
		},
		h.le,
	)
	webDocumentsKeyed.SetContext(ctx, true)
	defer webDocumentsKeyed.ClearContext()

	// Watch the list of web documents.
	webRuntimeStatusCtr := webRuntime.GetWebRuntimeStatusCtr()
	var webRuntimeStatus *web_runtime.WebRuntimeStatus
	for {
		webRuntimeStatus, err = webRuntimeStatusCtr.WaitValueChange(ctx, webRuntimeStatus, nil)
		if err != nil {
			return err
		}
		if webRuntimeStatus.GetClosed() {
			return errors.New("web runtime is closed")
		}

		docs := webRuntimeStatus.GetWebDocuments()
		docIDs := make([]string, len(docs))
		for i, doc := range docs {
			docIDs[i] = doc.GetId()
		}

		_, removed := webDocumentsKeyed.SyncKeys(docIDs, true)

		// Track removed web documents to make sure we have at least one worker.
		if len(removed) != 0 {
			if err := sema.Acquire(ctx, 1); err != nil {
				return err
			}

			if singletonWorkerDoc != "" && slices.Contains(removed, singletonWorkerDoc) {
				// This document was holding the singleton WebWorker.
				// Restart the other trackers.
				wakeOtherWebDocs(singletonWorkerDoc)
				singletonWorkerDoc = ""
			}

			sema.Release(1)
		}
	}
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *WebHost) DeletePlugin(ctx context.Context, pluginID string) error {
	// TODO remove caches or local storage?
	return nil
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WebHost)(nil)
