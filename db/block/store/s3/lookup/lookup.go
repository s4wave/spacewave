package block_store_s3_lookup

import (
	"context"
	io "io"
	"net/http"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "hydra/block/store/s3/lookup"

// Version is the API version.
var Version = semver.MustParse("0.0.1")

// Controller looks up blocks via an S3 HTTP service for LookupBlockFromNetwork directives.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// conf is the config
	conf *Config
	// client is the client to use
	client *http.Client
}

// NewController constructs a controller that looks up blocks via an HTTP
// service for LookupBlockFromNetwork directives.
//
// if client is nil, uses http.DefaultClient
func NewController(le *logrus.Entry, conf *Config, client *http.Client) *Controller {
	if client == nil {
		client = http.DefaultClient
	}
	return &Controller{
		le:     le,
		conf:   conf,
		client: client,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"lookup blocks via s3 http",
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The context passed is canceled when the directive instance expires.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case dex.LookupBlockFromNetwork:
		return c.resolveLookupBlockFromNetwork(ctx, di, d)
	}
	return nil, nil
}

// GetBlockFromService looks up a block from the http service.
func (c *Controller) GetBlockFromService(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	reqURL, err := c.conf.ParseURL()
	if err != nil {
		return nil, false, err
	}
	reqURL = reqURL.JoinPath(ref.MarshalString())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return nil, false, err
	}
	resp, err := httplog.DoRequest(c.le, c.client, req, c.conf.GetVerbose())
	if err != nil {
		return nil, false, err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	// unexpected error
	if resp.StatusCode == 500 {
		err = errors.Errorf("service returned internal error: %s", strings.TrimSpace(string(respBody)))
		return nil, false, err
	}

	var found bool
	var data []byte
	if resp.StatusCode == 200 {
		found = len(respBody) != 0
		data = respBody
	} else if resp.StatusCode == 403 {
		err = errors.New(resp.Status)
	} else if resp.StatusCode != 404 {
		err = errors.Errorf("unexpected response status: %d: %s", resp.StatusCode, resp.Status)
	}

	return data, found, err
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
