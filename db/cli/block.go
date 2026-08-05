//go:build !js && !wasip1

package cli

import (
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/block"
	api "github.com/s4wave/spacewave/db/daemon/api"
)

// RunPutBlock runs putting a block into a bucket.
func (a *ClientArgs) RunPutBlock(_ *cli.Context) error {
	// Resolve the client and request context.
	le := a.GetLogger()
	ctx := a.GetContext()
	c, err := a.BuildClient()
	if err != nil {
		return err
	}

	// Read block data from stdin or the configured file.
	var dat []byte
	if a.BlockDataFile == "" || a.BlockDataFile == "-" {
		le.Debug("reading from stdin")
		dat, err = io.ReadAll(os.Stdin)
	} else {
		le.Debugf("reading from file %s", a.BlockDataFile)
		dat, err = os.ReadFile(a.BlockDataFile)
	}
	if err != nil {
		return err
	}

	// Submit the block operation to the daemon.
	resp, err := c.BucketOp(ctx, &api.BucketOpRequest{
		Op:           api.BucketOp_BucketOp_BLOCK_PUT,
		BucketOpArgs: &a.BucketOpArgs,
		Data:         dat,
	})
	if err != nil {
		return err
	}

	// Log the response and print the resulting block reference.
	d, err := resp.MarshalJSON()
	if err != nil {
		le.WithError(err).Warn("unable to marshal put block result")
		return err
	}
	le.Debug(string(d))
	sref := resp.GetEvent().GetPutBlock().GetBlockCommon().GetBlockRef().MarshalString()
	os.Stdout.WriteString(sref)
	os.Stdout.WriteString("\n")
	return nil
}

// RunGetBlock runs getting a block from a bucket.
func (a *ClientArgs) RunGetBlock(_ *cli.Context) error {
	// Resolve the request context and block reference.
	le := a.GetLogger()
	ctx := a.GetContext()
	br, err := block.UnmarshalBlockRefB58(a.GetBlockRef)
	if err != nil {
		return err
	}

	// Open the daemon client and fetch the block.
	c, err := a.BuildClient()
	if err != nil {
		return err
	}
	resp, err := c.BucketOp(ctx, &api.BucketOpRequest{
		Op:           api.BucketOp_BucketOp_BLOCK_GET,
		BucketOpArgs: &a.BucketOpArgs,
		BlockRef:     br,
	})
	if err != nil {
		return err
	}

	// Reject missing blocks and log the response before writing data.
	if !resp.GetFound() {
		return block.ErrNotFound
	}
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

// RunRmBlock runs removing a block from a bucket.
func (a *ClientArgs) RunRmBlock(_ *cli.Context) error {
	// Resolve and validate the block reference.
	ctx := a.GetContext()
	br, err := block.UnmarshalBlockRefB58(a.GetBlockRef)
	if err != nil {
		return err
	}

	// Submit the block removal operation.
	c, err := a.BuildClient()
	if err != nil {
		return err
	}
	_, err = c.BucketOp(ctx, &api.BucketOpRequest{
		Op:           api.BucketOp_BucketOp_BLOCK_RM,
		BucketOpArgs: &a.BucketOpArgs,
		BlockRef:     br,
	})
	if err != nil {
		return err
	}

	// Print the removed block reference.
	os.Stdout.WriteString(br.MarshalString())
	os.Stdout.WriteString("\n")
	return nil
}
