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
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WebQuickJSHostControllerID is the quickjs web host controller ID.
const WebQuickJSHostControllerID = "bldr/plugin/host/web-quickjs"

// WebQuickJSHost implements the plugin host with QuickJS WASI in browser SharedWorkers.
// This runs JS plugins (platform "js") in a sandboxed QuickJS environment.
type WebQuickJSHost struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
	// webRuntimeID is the identifier of the web runtime
	webRuntimeID string
	// useDedicatedWorkers forces dedicated Workers instead of SharedWorkers.
	useDedicatedWorkers bool
}

// NewWebQuickJSHost constructs a new WebQuickJSHost.
func NewWebQuickJSHost(b bus.Bus, le *logrus.Entry, webRuntimeID string, useDedicatedWorkers bool) (*WebQuickJSHost, error) {
	// "js" platform - runs in QuickJS WASI
	platform := bldr_platform.NewJsPlatform()
	return &WebQuickJSHost{
		b:                   b,
		le:                  le,
		pluginPlatformID:    platform.GetPlatformID(),
		webRuntimeID:        webRuntimeID,
		useDedicatedWorkers: useDedicatedWorkers,
	}, nil
}

// NewWebQuickJSHostController constructs the WebQuickJSHost and PluginHost controller.
func NewWebQuickJSHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *QuickJSConfig,
) (*host_controller.Controller, *WebQuickJSHost, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	pluginHost, err := NewWebQuickJSHost(b, le, c.GetWebRuntimeId(), c.GetUseDedicatedWorkers())
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		controller.NewInfo(WebQuickJSHostControllerID, Version, "plugin host with QuickJS WASI in browser SharedWorkers"),
		pluginHost,
	)
	return hctrl, pluginHost, nil
}

// Execute is a stub as the web host does not need a global management goroutine.
func (h *WebQuickJSHost) Execute(ctx context.Context) error {
	return nil
}

// GetPlatformId returns the plugin platform ID for this host.
func (h *WebQuickJSHost) GetPlatformId() string {
	return h.pluginPlatformID
}

// ListPlugins lists the set of initialized plugins.
func (h *WebQuickJSHost) ListPlugins(ctx context.Context) ([]string, error) {
	return nil, nil
}

// ExecutePlugin executes the plugin with the given ID.
// Very similar to WebHost.ExecutePlugin but uses WorkerType_SAB for blocking WASI I/O.
func (h *WebQuickJSHost) ExecutePlugin(
	rctx context.Context,
	pluginID, instanceKey, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// restrict to .mjs and .js only
	if !strings.HasSuffix(entrypoint, ".mjs") && !strings.HasSuffix(entrypoint, ".js") {
		return errors.Errorf("entrypoint must have a .mjs or .js extension: %q", entrypoint)
	}

	// double-check the entrypoint exists and is executable
	entrypoint = filepath.Clean(entrypoint)
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrapf(err, "entrypoint at %s", entrypoint)
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

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(4)
	pluginStartInfo := plugin.NewPluginStartInfo(pluginInstanceID, pluginID, instanceKey)
	pluginStartInfoJsonB64, err := pluginStartInfo.MarshalJsonBase64()
	if err != nil {
		return err
	}
	pluginStartInfoBin := []byte(pluginStartInfoJsonB64)

	// web worker create request - use QuickJS worker type
	pluginWebWorkerID := "plugin/" + pluginID
	if instanceKey != "" {
		pluginWebWorkerID += "/" + instanceKey
	}
	pluginWebWorkerPath := plugin.PluginDistHTTPPath(pluginID, entrypoint)

	webRuntime, _, webRuntimeRef, err := web_runtime.ExLookupWebRuntime(ctx, h.b, false, h.webRuntimeID)
	if err != nil {
		return err
	}
	defer webRuntimeRef.Release()

	h.le.
		WithField("entrypoint", entrypoint).
		WithField("web-runtime", h.webRuntimeID).
		Debugf("executing QuickJS plugin entrypoint via http: %s", pluginWebWorkerPath)

	// Mount the RPC handler to the bus.
	baseControllerID := WebQuickJSHostControllerID + "/" + pluginID
	if instanceKey != "" {
		baseControllerID += "/" + instanceKey
	}
	rpcServiceControllerID := baseControllerID + "/rpc-host"
	var hostInvoker srpc.Invoker = hostMux
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo(rpcServiceControllerID, Version, "rpc host for quickjs plugin"),
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

	// Track web documents and create workers (similar to WebHost)
	var singletonWorkerDoc string
	var cmtx csync.Mutex

	var webDocumentsKeyed *keyed.Keyed[string, struct{}]
	wakeOtherWebDocs := func(otherThanDoc string) {
		_, _ = webDocumentsKeyed.RestartAllRoutines(func(docID string, _ struct{}) bool {
			return docID != otherThanDoc
		})
	}

	createWorkerWithDoc := func(ctx context.Context, doc web_document.WebDocument) error {
		unlock, err := cmtx.Lock(ctx)
		if err != nil {
			return err
		}
		defer unlock()

		webDocumentID := doc.GetWebDocumentUuid()
		if singletonWorkerDoc == webDocumentID {
			singletonWorkerDoc = ""
			wakeOtherWebDocs(webDocumentID)
		} else if singletonWorkerDoc != "" {
			return nil
		}

		le := h.le.WithFields(logrus.Fields{
			"web-document": webDocumentID,
			"web-runtime":  h.webRuntimeID,
			"web-worker":   pluginWebWorkerID,
		})
		le.Debug("creating QuickJS web worker")

		// Create worker with QUICKJS worker type for QuickJS reactor.
		// When useDedicatedWorkers is set, force DedicatedWorker.
		// Otherwise, send WORKER_MODE_DEFAULT so the browser-side
		// detectWorkerCommsConfig() selects the best mode.
		workerMode := web_document.WebWorkerMode_WORKER_MODE_DEFAULT
		if h.useDedicatedWorkers {
			workerMode = web_document.WebWorkerMode_WORKER_MODE_DEDICATED
		}
		createdWorker, err := doc.CreateWebWorker(ctx, &web_document.CreateWebWorkerRequest{
			Id:         pluginWebWorkerID,
			Path:       pluginWebWorkerPath,
			WorkerMode: workerMode,
			InitData:   pluginStartInfoBin,
			WorkerType: web_document.WebWorkerType_WEB_WORKER_TYPE_QUICKJS,
		})
		if err != nil {
			le.WithError(err).Warn("unable to create QuickJS web worker")
			return err
		}
		// nil, nil means document is hidden - return nil to wait for visibility change
		if createdWorker == nil {
			le.Debug("document is hidden, waiting for visibility")
			return nil
		}

		createdShared := createdWorker.GetShared()
		le.WithField("web-worker-shared", createdShared).Debug("successfully created QuickJS web worker")

		if !createdShared {
			singletonWorkerDoc = webDocumentID
		}

		return nil
	}

	removeWorkerInstances := func(ctx context.Context, doc web_document.WebDocument) (map[string]web_worker.WebWorker, error) {
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

			h.le.WithFields(logrus.Fields{
				"web-document": doc.GetWebDocumentUuid(),
				"web-runtime":  h.webRuntimeID,
				"web-worker":   pluginWebWorkerID,
			}).Debug("removing old instance of QuickJS web worker")
			_, err := worker.Remove(ctx)
			if err != nil {
				h.le.WithError(err).Warn("unable to remove old QuickJS web worker instance")
			}
		}

		return docWebWorkers, nil
	}

	trackWebDocument := func(ctx context.Context, webDocumentID string) error {
		doc, err := webRuntime.GetWebDocument(ctx, webDocumentID, true)
		if err != nil {
			return err
		}

		cleanupCtx, cleanupCtxCancel := context.WithTimeout(ctx, time.Second*3)
		defer cleanupCtxCancel()

		for cleanupCtx.Err() == nil {
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

		docStatusCtr := doc.GetWebDocumentStatusCtr()
		var docStatus *web_document.WebDocumentStatus
		var workerInstance *web_document.WebWorkerStatus
		for {
			// Wait for the document to become visible before creating the worker.
			// CreateWebWorker returns nil, nil when the document is hidden.
			if workerInstance == nil && (docStatus == nil || !docStatus.GetHidden()) {
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

			// Find our worker instance in the status, or nil if not found or hidden.
			workerInstance = nil
			if !docStatus.GetHidden() {
				for _, worker := range docStatus.GetWebWorkers() {
					if worker.GetId() == pluginWebWorkerID {
						workerInstance = worker
						break
					}
				}
			}
		}
	}

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
			h.le.WithError(err).Warn("unable to cleanup old QuickJS web worker instances")
		}
	}()

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

		if len(removed) != 0 {
			unlock, err := cmtx.Lock(ctx)
			if err != nil {
				return err
			}

			if singletonWorkerDoc != "" && slices.Contains(removed, singletonWorkerDoc) {
				wakeOtherWebDocs(singletonWorkerDoc)
				singletonWorkerDoc = ""
			}

			unlock()
		}
	}
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *WebQuickJSHost) DeletePlugin(ctx context.Context, pluginID string) error {
	return nil
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WebQuickJSHost)(nil)
