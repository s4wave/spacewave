//go:build !js && !wasip1

package cli

import (
	"errors"
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	api "github.com/s4wave/spacewave/db/daemon/api"
)

// RunGetObject returns an object from the store.
func (a *ClientArgs) RunGetObject(_ *cli.Context) error {
	// Resolve the client and request context.
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Validate and execute the object lookup.
	req := &a.ObjectStoreOpReq
	req.Op = api.ObjectStoreOp_ObjectStoreOp_GET_KEY
	if err := req.Validate(); err != nil {
		return err
	}
	resp, err := c.ObjectStoreOp(ctx, req)
	if err != nil {
		return err
	}
	if !resp.GetFound() {
		return errors.New("object not found")
	}

	// Log the response metadata and write the object data.
	data := resp.GetData()
	resp.Data = nil
	d, err := resp.MarshalJSON()
	if err != nil {
		return err
	}
	le.Debug(string(d))
	os.Stdout.Write(data)
	return nil
}

// RunRmObject removes an object from the store.
func (a *ClientArgs) RunRmObject(_ *cli.Context) error {
	// Resolve the client and request context.
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Validate and execute the object removal.
	req := &a.ObjectStoreOpReq
	req.Op = api.ObjectStoreOp_ObjectStoreOp_DELETE_KEY
	if err := req.Validate(); err != nil {
		return err
	}
	resp, err := c.ObjectStoreOp(ctx, req)
	if err != nil {
		return err
	}
	if !resp.GetFound() {
		return errors.New("object not found")
	}

	// Log the response metadata and write the removed object data.
	data := resp.GetData()
	resp.Data = nil
	d, err := resp.MarshalJSON()
	if err != nil {
		return err
	}
	le.Debug(string(d))
	os.Stdout.Write(data)
	return nil
}

// RunPutObject puts an object to the store.
func (a *ClientArgs) RunPutObject(_ *cli.Context) error {
	// Resolve the client and request context.
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Read object data from stdin or the configured file.
	var dat []byte
	if a.ObjectStoreFile == "" || a.ObjectStoreFile == "-" {
		le.Debug("reading from stdin")
		dat, err = io.ReadAll(os.Stdin)
	} else {
		le.Debugf("reading from file %s", a.ObjectStoreFile)
		dat, err = os.ReadFile(a.ObjectStoreFile)
	}
	if err != nil {
		return err
	}

	// Configure and submit the object write.
	req := &a.ObjectStoreOpReq
	req.Data = dat
	req.Op = api.ObjectStoreOp_ObjectStoreOp_PUT_KEY
	if _, err := c.ObjectStoreOp(ctx, req); err != nil {
		return err
	}
	return nil
}

// RunListObjectKeys lists object keys in a store.
func (a *ClientArgs) RunListObjectKeys(_ *cli.Context) error {
	// Resolve the client and request context.
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Validate and execute the key listing.
	req := &a.ObjectStoreOpReq
	req.Op = api.ObjectStoreOp_ObjectStoreOp_LIST_KEYS
	if err := req.Validate(); err != nil {
		return err
	}
	resp, err := c.ObjectStoreOp(ctx, req)
	if err != nil {
		return err
	}

	// Log the key count and print each returned key.
	le.WithField("key-count", len(resp.GetKeys())).Debug("returned keys")
	for _, key := range resp.GetKeys() {
		os.Stdout.WriteString(string(key))
		os.Stdout.WriteString("\n")
	}
	return nil
}
