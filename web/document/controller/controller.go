package web_document_controller

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/bifrost/util/backoff"
	"github.com/aperturerobotics/bifrost/util/retry"
	web_document "github.com/aperturerobotics/bldr/web/document"
	web_view "github.com/aperturerobotics/bldr/web/document/view"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Controller implements a common bldr WebDocument controller.
// Tracks attached WebDocument state and manages RPC calls in/out.
type Controller struct {
	// ctx is the controller context
	// set in the execute() function
	// ensure not used before execute sets it.
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus

	// doc is the web document instance
	doc web_document.WebDocument
	// webDocumentId is the controller id to use
	webDocumentId string
	// webDocumentVersion is the version
	webDocumentVersion semver.Version
	// openStream opens a stream to the WebDocument.
	openStream srpc.OpenStreamFunc

	// trigger is pushed to when anything changes
	trigger chan struct{}
	// mtx guards the below fields
	mtx sync.Mutex
	// rt is the runtime
	rt web_document.WebDocument
	// cState is the current known controller state.
	cState cState
}

// NewController constructs a new runtime controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	webDocumentId string,
	webDocumentVersion semver.Version,
	doc web_document.WebDocument,
	openStream srpc.OpenStreamFunc,
) *Controller {
	return &Controller{
		le:  le.WithField("document-id", webDocumentId),
		bus: bus,
		doc: doc,

		webDocumentId:      webDocumentId,
		webDocumentVersion: webDocumentVersion,
		openStream:         openStream,

		trigger: make(chan struct{}, 1),
	}
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return strings.Join([]string{
		"bldr",
		"document",
		c.webDocumentId,
		c.webDocumentVersion.String(),
	}, "/")
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		c.GetControllerID(),
		c.webDocumentVersion,
		"WebDocument "+c.webDocumentId+"@"+c.webDocumentVersion.String(),
	)
}

// Execute executes the WebDocument controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(rctx context.Context) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	c.ctx = ctx
	defer ctxCancel()

	c.le.Debug("executing web document controller")
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.doc.Execute(ctx)
	}()

	bo := (&backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	}).Construct()
	for {
		// retry with a backoff in case the frontend is gone / non-responsive
		if err := retry.Retry(ctx, c.le, c.syncOnce, bo); err != nil {
			return err
		}

		// query / update state as necessary
		if err := c.syncOnce(ctx); err != nil {
			c.le.WithError(err).Warn("error synchronizing with frontend")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(bo.NextBackOff()):
				continue
			}
		} else {
			bo.Reset()
		}

		// note: will add case to re-sync when needed
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}
	}
}

// GetWebDocument returns the controlled Document, waiting for it to be non-nil.
func (c *Controller) GetWebDocument(ctx context.Context) (web_document.WebDocument, error) {
	for {
		c.mtx.Lock()
		rt, trig := c.rt, c.trigger
		c.mtx.Unlock()
		if rt != nil {
			return rt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-trig:
		}
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) (directive.Resolver, error) {
	return nil, nil
}

// HandleWebView handles an incoming WebView on a new Goroutine.
func (c *Controller) HandleWebView(wv web_view.WebView) {
	loadTestComponent(c.ctx, c.le, wv)
}

// OpenRpcStream opens an RPC stream to the WebDocument.
func (c *Controller) OpenRpcStream(
	ctx context.Context,
	msgHandler srpc.PacketHandler,
	closeHandler srpc.CloseHandler,
) (srpc.Writer, error) {
	return c.openStream(ctx, msgHandler, closeHandler)
}

// doTrigger triggers all waiting goroutines
func (c *Controller) doTrigger() {
	for {
		select {
		case c.trigger <- struct{}{}:
		default:
			return
		}
	}
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ web_document.WebDocumentController = ((*Controller)(nil))
	_ web_document.WebDocumentHandler    = ((*Controller)(nil))
)
