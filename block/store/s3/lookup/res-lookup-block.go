package block_store_s3_lookup

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/pkg/errors"
)

// lookupBlockFromNetworkResolver resolves LookupBlockFromNetwork
type lookupBlockFromNetworkResolver struct {
	c *Controller
	d dex.LookupBlockFromNetwork
}

// resolveLookupBlockFromNetwork resolves the LookupBlockFromNetwork directive.
func (c *Controller) resolveLookupBlockFromNetwork(
	ctx context.Context,
	di directive.Instance,
	dir dex.LookupBlockFromNetwork,
) ([]directive.Resolver, error) {
	matchBucketID := c.conf.GetBucketId()
	lookupBucketID := dir.LookupBlockFromNetworkBucketId()
	if lookupBucketID == "" || matchBucketID != lookupBucketID {
		return nil, nil
	}
	return directive.R(&lookupBlockFromNetworkResolver{
		c: c,
		d: dir,
	}, nil)
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *lookupBlockFromNetworkResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.ClearValues()
	reqURL, err := r.c.conf.ParseURL()
	if err != nil {
		return err
	}
	reqURL = reqURL.JoinPath(r.d.LookupBlockFromNetworkRef().MarshalString())
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL.String(), nil)
	if err != nil {
		return err
	}
	resp, err := r.c.client.Do(req)
	if err != nil {
		return err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// exceptional error
	if resp.StatusCode == 500 {
		err = errors.Errorf("service returned internal error: %s", strings.TrimSpace(string(respBody)))
		return err
	}

	var found bool
	var data []byte
	if resp.StatusCode == 200 {
		found = len(respBody) != 0
		data = respBody
	} else if resp.StatusCode == 403 {
		err = errors.New(resp.Status)
	} else if resp.StatusCode != 404 {
		return errors.Errorf("unexpected response status: %d: %s", resp.StatusCode, resp.Status)
	}
	if found || !r.c.conf.GetSkipNotFound() || err != nil {
		var val dex.LookupBlockFromNetworkValue = dex.NewLookupBlockFromNetworkValue(data, err)
		_, _ = handler.AddValue(val)
	}

	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupBlockFromNetworkResolver)(nil))
