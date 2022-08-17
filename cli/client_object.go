package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	api "github.com/aperturerobotics/hydra/daemon/api"
	"github.com/urfave/cli"
)

// RunGetObject returns an object from the store.
func (a *ClientArgs) RunGetObject(_ *cli.Context) error {
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

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

	data := resp.GetData()
	resp.Data = nil
	d, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	le.Debug(string(d))
	os.Stdout.Write(data)
	return nil
}

// RunRmObject removes an object from the store.
func (a *ClientArgs) RunRmObject(_ *cli.Context) error {
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

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

	data := resp.GetData()
	resp.Data = nil
	d, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	le.Debug(string(d))
	os.Stdout.Write(data)
	return nil
}

// RunPutObject puts an object to the store.
func (a *ClientArgs) RunPutObject(_ *cli.Context) error {
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

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

	req := &a.ObjectStoreOpReq
	req.Data = dat
	req.Op = api.ObjectStoreOp_ObjectStoreOp_PUT_KEY
	resp, err := c.ObjectStoreOp(ctx, req)
	if err != nil {
		return err
	}
	_ = resp
	return nil
}

// RunListObjectKeys lists object keys in a store.
func (a *ClientArgs) RunListObjectKeys(_ *cli.Context) error {
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	req := &a.ObjectStoreOpReq
	req.Op = api.ObjectStoreOp_ObjectStoreOp_LIST_KEYS
	if err := req.Validate(); err != nil {
		return err
	}

	resp, err := c.ObjectStoreOp(ctx, req)
	if err != nil {
		return err
	}
	le.WithField("key-count", len(resp.GetKeys())).Debug("returned keys")
	for _, key := range resp.GetKeys() {
		os.Stdout.WriteString(key)
		os.Stdout.WriteString("\n")
	}
	return nil
}
