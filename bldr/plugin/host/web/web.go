package plugin_host_web

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	web_document "github.com/s4wave/spacewave/bldr/web/document"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	"github.com/s4wave/spacewave/db/unixfs"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/s4wave/spacewave/net/util/randstring"
	"github.com/sirupsen/logrus"
)

// ControllerID is the WebWorker plugin host controller ID.
const ControllerID = "bldr/plugin/host/web"

const defaultWebHostPlatformID = "web/js/wasm"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// WebHost implements browser plugin execution with WebWorker processes.
type WebHost struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// controllerID is the controller id for this platform instance
	controllerID string
	// pluginPlatformID is the plugin platform to use
	pluginPlatformID string
	// webRuntimeID is the identifier of the web runtime
	webRuntimeID string
	// forceDedicatedWorkers forces dedicated Workers instead of SharedWorkers.
	forceDedicatedWorkers bool
}

// NewWebHost constructs a new WebHost.
func NewWebHost(b bus.Bus, le *logrus.Entry, webRuntimeID, platformID string, forceDedicatedWorkers bool) (*WebHost, error) {
	platform, err := parseWebHostPlatform(platformID)
	if err != nil {
		return nil, err
	}
	pluginPlatformID := platform.GetPlatformID()

	return &WebHost{
		b:                     b,
		le:                    le,
		controllerID:          controllerIDForPlatform(pluginPlatformID),
		pluginPlatformID:      pluginPlatformID,
		webRuntimeID:          webRuntimeID,
		forceDedicatedWorkers: forceDedicatedWorkers,
	}, nil
}

// NewWebHostController constructs the WebHost and PluginHost controller.
func NewWebHostController(
	le *logrus.Entry,
	b bus.Bus,
	c *Config,
) (*host_controller.Controller, *WebHost, error) {
	// Validate the config and construct the WebHost controller.
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	pluginHost, err := NewWebHost(b, le, c.GetWebRuntimeId(), c.GetPlatformId(), c.GetForceDedicatedWorkers())
	if err != nil {
		return nil, nil, err
	}
	hctrl := host_controller.NewController(
		le,
		b,
		controller.NewInfo(pluginHost.controllerID, Version, "plugin host with WebWorker processes for "+pluginHost.pluginPlatformID),
		pluginHost,
	)
	return hctrl, pluginHost, nil
}

// Execute is a stub as the web host does not need a global management goroutine.
func (h *WebHost) Execute(ctx context.Context) error {
	return nil
}

// GetPlatformId returns the plugin platform ID for this host.
func (h *WebHost) GetPlatformId() string {
	return h.pluginPlatformID
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
	pluginID, instanceKey, executionKey, manifestRoot, entrypoint string,
	pluginDist, pluginAssets *unixfs.FSHandle,
	hostMux srpc.Mux,
	rpcInit plugin_host.PluginRpcInitCb,
) error {
	// Set up the plugin execution context and the fatal error channel.
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	fatalErrCh := make(chan error, 1)
	reportFatalErr := func(err error) {
		if err == nil || err == context.Canceled || !isWebWorkerFailureError(err) {
			return
		}
		select {
		case fatalErrCh <- err:
			ctxCancel()
		default:
		}
	}
	popFatalErr := func() error {
		select {
		case err := <-fatalErrCh:
			return err
		default:
			return nil
		}
	}

	// restrict to .mjs and .js only
	if !strings.HasSuffix(entrypoint, ".mjs") && !strings.HasSuffix(entrypoint, ".js") {
		return errors.Errorf("entrypoint must have a .mjs or .js extension: %q", entrypoint)
	}
	le := h.le.WithField("plugin-id", pluginID)

	// double-check the entrypoint exists and is executable
	entrypoint = filepath.Clean(entrypoint)
	le.
		WithField("entrypoint", entrypoint).
		Debug("looking up web plugin entrypoint")
	entrypointHandle, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		return errors.Wrapf(err, "entrypoint at %s", entrypoint)
	}
	le.
		WithField("entrypoint", entrypoint).
		Debug("web plugin entrypoint lookup complete")

	// Read the entrypoint file info and release the lookup handle.
	le.
		WithField("entrypoint", entrypoint).
		Debug("reading web plugin entrypoint file info")
	entrypointFi, err := entrypointHandle.GetFileInfo(ctx)
	entrypointHandle.Release()
	if err != nil {
		return errors.Wrap(err, "entrypoint")
	}
	le.
		WithField("entrypoint", entrypoint).
		WithField("mode", entrypointFi.Mode().String()).
		Debug("web plugin entrypoint file info ready")

	// Reject entrypoints that are not regular executable files.
	entrypointFiMode := entrypointFi.Mode()
	if !entrypointFiMode.IsRegular() {
		return errors.Errorf("entrypoint must be an executable regular file: %s", entrypointFiMode.String())
	}

	// create unique plugin instance id
	pluginInstanceID := randstring.RandomIdentifier(4)
	pluginStartInfo := plugin.NewPluginStartInfo(pluginInstanceID, pluginID, instanceKey, manifestRoot)
	pluginStartInfoJsonB64, err := pluginStartInfo.MarshalJsonBase64()
	if err != nil {
		return err
	}
	pluginStartInfoBin := []byte(pluginStartInfoJsonB64)

	// web worker create request
	// each execution of an instanced plugin gets a unique worker ID.
	pluginWebWorkerID := "plugin/" + pluginID
	if executionKey != "" {
		pluginWebWorkerID += "/" + executionKey
	}
	pluginWebWorkerPath := plugin.PluginDistHTTPPath(plugin.PluginArtifactID(pluginID, manifestRoot), entrypoint)

	// Look up the web runtime and hold its reference for the plugin lifetime.
	le.
		WithField("web-runtime", h.webRuntimeID).
		Debug("looking up web runtime for plugin")
	webRuntime, _, webRuntimeRef, err := web_runtime.ExLookupWebRuntime(ctx, h.b, false, h.webRuntimeID)
	if err != nil {
		return err
	}
	defer webRuntimeRef.Release()
	le.
		WithField("web-runtime", h.webRuntimeID).
		Debug("web runtime lookup complete for plugin")

	// Log the HTTP path the plugin entrypoint will execute from.
	h.le.
		WithField("entrypoint", entrypoint).
		WithField("web-runtime", h.webRuntimeID).
		Debugf("executing plugin entrypoint via http: %s", pluginWebWorkerPath)

	// Mount the RPC handler to the bus.
	baseControllerID := h.controllerID + "/" + pluginID
	if executionKey != "" {
		baseControllerID += "/" + executionKey
	}
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

	// Track dedicated worker ownership and shared RPC readiness state.
	var workerOwner dedicatedWorkerOwner
	var rpcReadyPublished bool
	var cmtx csync.Mutex

	// Create the web worker on each document.
	var webDocumentsKeyed *keyed.Keyed[string, struct{}]
	wakeOtherWebDocs := func(otherThanDoc string) {
		_, _ = webDocumentsKeyed.RestartAllRoutines(func(docID string, _ struct{}) bool {
			return docID != otherThanDoc
		})
	}

	// Create the web worker on one web document, publishing the RPC client once.
	createWorkerWithDoc := func(ctx context.Context, doc web_document.WebDocument) error {
		// Lock the shared creation mutex for the whole worker creation attempt.
		unlock, err := cmtx.Lock(ctx)
		if err != nil {
			return err
		}
		locked := true
		defer func() {
			if locked {
				unlock()
			}
		}()

		// Ask the dedicatedWorkerOwner whether this document should create the worker.
		webDocumentID := doc.GetWebDocumentUuid()
		create, wake := workerOwner.beginCreate(webDocumentID)
		if wake {
			wakeOtherWebDocs(webDocumentID)
		}
		if !create {
			return nil
		}

		// Build the per-document logger and select the requested worker mode.
		le := h.le.
			WithFields(logrus.Fields{
				"web-document": webDocumentID,
				"web-runtime":  h.webRuntimeID,
				"web-worker":   pluginWebWorkerID,
			})
		le.Debug("creating web worker")

		// When forceDedicatedWorkers is set, use DedicatedWorker.
		// Otherwise, send WORKER_MODE_DEFAULT so the browser-side
		// detectWorkerCommsConfig() selects the best mode.
		workerMode := web_document.WebWorkerMode_WORKER_MODE_DEFAULT
		if h.forceDedicatedWorkers {
			workerMode = web_document.WebWorkerMode_WORKER_MODE_DEDICATED
		}

		// Create the web worker on the document and handle creation failures.
		createdWorker, err := doc.CreateWebWorker(ctx, &web_document.CreateWebWorkerRequest{
			Id:         pluginWebWorkerID,
			Path:       pluginWebWorkerPath,
			WorkerMode: workerMode,
			InitData:   pluginStartInfoBin,
			Generation: pluginInstanceID,
		})
		if err != nil {
			workerOwner.observeCreateFailed(webDocumentID)
			le.WithError(err).Warn("unable to create web worker")
			return err
		}

		// A document without worker ownership waits for its next readiness update.
		if createdWorker == nil {
			workerOwner.observeCreateSkipped(webDocumentID, doc.GetWebDocumentStatusCtr().GetValue().GetHidden())
			return nil
		}

		// Record the created worker and release the shared creation lock.
		createdShared := createdWorker.GetShared()
		le.
			WithField("web-worker-shared", createdShared).
			Debug("successfully created web worker")

		// Record the created worker ownership and release the shared creation lock.
		workerOwner.observeCreatedWorker(webDocumentID, createdShared)

		// Release the shared creation mutex after the worker is recorded.
		unlock()
		locked = false

		// Wait for the created worker to report readiness outside the lock.
		ready, err := waitForCreatedWebWorkerReady(ctx, doc.GetWebDocumentStatusCtr(), createdWorker)
		if err != nil {
			return err
		}
		if !ready {
			le.Info("web worker did not become ready before timeout, recreating")
			return nil
		}

		// Reacquire the lock and publish the RPC client once.
		unlock, err = cmtx.Lock(ctx)
		if err != nil {
			return err
		}
		locked = true
		if !rpcReadyPublished {
			if err := rpcInit(pluginRpcClient); err != nil {
				return err
			}
			rpcReadyPublished = true
		}

		return nil
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

		// Remove stale worker instances until the bounded cleanup window expires.
		for cleanupCtx.Err() == nil {
			removedInstances, err := removeStaleWebWorkerInstances(ctx, doc, h.le, h.webRuntimeID, pluginWebWorkerID, pluginInstanceID)
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

		// Stop cleanup if the parent context was canceled during the loop.
		cleanupCtxCancel()
		if ctx.Err() != nil {
			return context.Canceled
		}

		// Watch the list of web workers to ensure ours is running.
		docStatusCtr := doc.GetWebDocumentStatusCtr()
		var docStatus *web_document.WebDocumentStatus
		var workerInstance *web_document.WebWorkerStatus
		for {
			// Only the document holding worker ownership can satisfy this request.
			// Its background visibility does not suspend the shared runtime.
			if workerInstance == nil && docStatus != nil && !docStatus.GetPluginWorkersBlocked() {
				if err := createWorkerWithDoc(ctx, doc); err != nil {
					return err
				}
			}

			// Wait for the next web document status update.
			docStatus, err = docStatusCtr.WaitValueChange(ctx, docStatus, nil)
			if err != nil {
				return err
			}
			// Release the document's worker ownership when the document closes.
			if docStatus.GetClosed() {
				unlock, err := cmtx.Lock(ctx)
				if err != nil {
					return err
				}
				if workerOwner.observeDocumentRemoved(webDocumentID) {
					wakeOtherWebDocs(webDocumentID)
				}
				unlock()
				return nil
			}
			// Update worker ownership with the document's latest visibility.
			unlock, err := cmtx.Lock(ctx)
			if err != nil {
				return err
			}
			if workerOwner.observeDocumentStatus(webDocumentID, docStatus.GetHidden()) {
				wakeOtherWebDocs(webDocumentID)
			}
			unlock()

			// Find our worker instance, including workers in background documents.
			workerInstance = nil
			for _, worker := range docStatus.GetWebWorkers() {
				if worker.GetDeleted() {
					continue
				}
				if worker.GetId() == pluginWebWorkerID && worker.GetGeneration() == pluginInstanceID {
					workerInstance = worker
					break
				}
			}
			// Report a failed worker instance and release its ownership.
			if workerInstance != nil && workerInstance.GetFailed() {
				unlock, err := cmtx.Lock(ctx)
				if err != nil {
					return err
				}
				workerOwner.observeWorkerFailed(webDocumentID)
				unlock()
				return webWorkerFailureError(workerInstance, "web worker failed")
			}
		}
	}

	// fully kill & wait for exit to be confirmed when returning
	cleanupInstances := func() error {
		// Remove this plugin's worker instances from every web document.
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

			// Remove own worker instances on each document and count the removals.
			var retErr error
			var nOldInstances int
			for _, doc := range docs {
				oldInstances, err := removeOwnWebWorkerInstances(ctx, doc, h.le, h.webRuntimeID, pluginWebWorkerID, pluginInstanceID)
				if err != nil {
					retErr = err
				}
				nOldInstances += len(oldInstances)
			}
			if retErr != nil {
				return retErr
			}

			// Wait briefly before rechecking for remaining old instances.
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
				err := trackWebDocument(ctx, webDocumentId)
				reportFatalErr(err)
				return err
			}, struct{}{}
		},
		h.le,
	)

	// Start the web document tracking routines under the plugin context.
	webDocumentsKeyed.SetContext(ctx, true)
	defer webDocumentsKeyed.ClearContext()

	// Watch the list of web documents.
	webRuntimeStatusCtr := webRuntime.GetWebRuntimeStatusCtr()
	var webRuntimeStatus *web_runtime.WebRuntimeStatus
	for {
		// Wait for the next web runtime status update and surface fatal errors.
		webRuntimeStatus, err = webRuntimeStatusCtr.WaitValueChange(ctx, webRuntimeStatus, nil)
		if err != nil {
			if fatalErr := popFatalErr(); fatalErr != nil {
				return fatalErr
			}
			return err
		}
		if fatalErr := popFatalErr(); fatalErr != nil {
			return fatalErr
		}
		if webRuntimeStatus.GetClosed() {
			return errors.New("web runtime is closed")
		}

		// Sync the tracked web document keys with the runtime status.
		docs := webRuntimeStatus.GetWebDocuments()
		docIDs := make([]string, len(docs))
		for i, doc := range docs {
			docIDs[i] = doc.GetId()
		}

		_, removed := webDocumentsKeyed.SyncKeys(docIDs, true)

		// Track removed web documents to make sure we have at least one worker.
		if len(removed) != 0 {
			unlock, err := cmtx.Lock(ctx)
			if err != nil {
				return err
			}

			for _, removedDocID := range removed {
				if workerOwner.observeDocumentRemoved(removedDocID) {
					wakeOtherWebDocs(removedDocID)
				}
			}

			unlock()
		}
	}
}

// DeletePlugin clears cached plugin data for the given plugin ID.
func (h *WebHost) DeletePlugin(ctx context.Context, pluginID string) error {
	// TODO remove caches or local storage?
	return nil
}

func parseWebHostPlatform(platformID string) (bldr_platform.Platform, error) {
	// Parse the platform ID, defaulting to the built-in web host platform.
	if platformID == "" {
		platformID = defaultWebHostPlatformID
	}
	platform, err := bldr_platform.ParsePlatform(platformID)
	if err != nil {
		return nil, err
	}
	if platform.GetExecutableExt() != ".mjs" {
		return nil, errors.Errorf("web host platform must produce a .mjs executable: %s", platformID)
	}
	return platform, nil
}

func controllerIDForPlatform(platformID string) string {
	if platformID == defaultWebHostPlatformID {
		return ControllerID
	}
	return ControllerID + "/" + platformID
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*WebHost)(nil)
