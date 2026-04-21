//go:build !tinygo

package bifrost_http_listener

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/sirupsen/logrus"
)

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller listens for incoming HTTP connections on a port.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// srv is the http server
	srv *http.Server

	// certFile is the path to the cert file if tls is enabled
	// can be empty
	certFile string
	// keyFile is the path to the key file if tls is enabled
	// cannot be empty if certFile is set
	// can be empty otherwise
	keyFile string
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	srv *http.Server,
) *Controller {
	return &Controller{
		le:  le,
		srv: srv,
	}
}

// NewControllerWithTLS constructs a new controller with TLS.
//
// certFile is the path to the cert file
// keyFile is the path to the key file
func NewControllerWithTLS(
	le *logrus.Entry,
	srv *http.Server,
	certFile string,
	keyFile string,
) *Controller {
	return &Controller{
		le:       le,
		srv:      srv,
		certFile: certFile,
		keyFile:  keyFile,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"listen for incoming http connections",
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(rctx context.Context) (rerr error) {
	if c.srv == nil {
		return nil
	}
	hasHTTPS := c.certFile != "" && c.keyFile != ""
	protocol := "http"
	if hasHTTPS {
		protocol = "https"
	}
	if c.srv.Addr == "" {
		c.le.Debugf("starting %s server", protocol)
	} else {
		c.le.Debugf("starting %s server with addr %q", protocol, c.srv.Addr)
	}
	if hasHTTPS {
		return c.srv.ListenAndServeTLS(c.certFile, c.keyFile)
	} else {
		return c.srv.ListenAndServe()
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// ServeHTTP serves HTTP using the handler.
//
// Does nothing if no server or handler set.
func (c *Controller) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if c.srv != nil && c.srv.Handler != nil {
		c.srv.Handler.ServeHTTP(rw, req)
	}
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = ((*Controller)(nil))
	_ http.Handler          = ((*Controller)(nil))
)
