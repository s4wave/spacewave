//go:build !tinygo

package block_packsweep_admin

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packsweep"
	"github.com/s4wave/spacewave/db/block/writelog"
)

// Client is the command's side of the service: the write log and the refs of
// the blocks the backend holds, read through it, and the raw service calls.
type Client struct {
	// SRPCPackSweepAdminClient makes the service calls.
	SRPCPackSweepAdminClient
}

// NewClient wraps a service client.
func NewClient(c SRPCPackSweepAdminClient) *Client {
	return &Client{SRPCPackSweepAdminClient: c}
}

// ReadLog calls fn with the backend's log lines after from, at most max when
// max is positive, reading as many pages as that takes.
func (c *Client) ReadLog(ctx context.Context, from writelog.Position, max int, fn func(writelog.Line) error) (writelog.ReadResult, error) {
	res := writelog.ReadResult{Next: from}
	read := 0
	for {
		// Read the next page, bounded by what max leaves.
		page := MaxLines
		if max > 0 {
			page = min(page, max-read)
		}
		resp, err := c.SRPCPackSweepAdminClient.ReadLog(ctx, &ReadLogRequest{From: res.Next.String(), Max: uint32(page)}) //nolint:gosec // At most MaxLines.
		if err != nil {
			return res, err
		}

		// Hand each line to fn.
		for _, l := range resp.GetLines() {
			if err := fn(writelog.Line{Seq: l.GetSeq(), Kind: logKind(l.GetKind()), Hash: l.GetHash()}); err != nil {
				return res, err
			}
		}
		read += len(resp.GetLines())

		// Stop where the backend stopped, or at max.
		if res, err = readResult(resp); err != nil {
			return res, err
		}
		if res.State != writelog.ReadMore || (max > 0 && read >= max) || len(resp.GetLines()) == 0 {
			return res, nil
		}
	}
}

// TrimLog deletes the backend's log lines at or before to.
func (c *Client) TrimLog(ctx context.Context, to writelog.Position) error {
	_, err := c.SRPCPackSweepAdminClient.TrimLog(ctx, &TrimLogRequest{To: to.String()})
	return err
}

// BlockRefs reads the child lists of the blocks the backend holds, in batches
// of MaxRefs.
func (c *Client) BlockRefs(ctx context.Context, refs []*block.BlockRef) ([]packsweep.RefsResult, error) {
	out := make([]packsweep.RefsResult, 0, len(refs))
	for start := 0; start < len(refs); start += MaxRefs {
		// Read one batch and check that it answers every block.
		batch := refs[start:min(start+MaxRefs, len(refs))]
		resp, err := c.Refs(ctx, &RefsRequest{Refs: batch})
		if err != nil {
			return nil, err
		}
		if len(resp.GetResults()) != len(batch) {
			return nil, errors.Errorf("the backend answered %d of %d blocks", len(resp.GetResults()), len(batch))
		}

		// Convert the answers.
		for _, r := range resp.GetResults() {
			out = append(out, packsweep.RefsResult{Refs: r.GetRefs(), Reason: r.GetReason()})
		}
	}
	return out, nil
}

// logKind parses a line kind the service named.
func logKind(s string) writelog.Kind {
	if len(s) != 1 {
		return 0
	}
	return writelog.Kind(s[0])
}

// readResult parses where the backend's read stopped.
func readResult(resp *ReadLogResponse) (writelog.ReadResult, error) {
	// Parse the position.
	next, err := parsePosition(resp.GetNext())
	if err != nil {
		return writelog.ReadResult{}, err
	}

	// Parse the state by the names ReadState.String gives.
	res := writelog.ReadResult{Next: next, Reason: resp.GetReason()}
	switch resp.GetState() {
	case writelog.ReadMore.String():
		res.State = writelog.ReadMore
	case writelog.ReadOpen.String():
		res.State = writelog.ReadOpen
	case writelog.ReadClosed.String():
		res.State = writelog.ReadClosed
	case writelog.ReadGap.String():
		res.State = writelog.ReadGap
	default:
		return res, errors.Errorf("the backend's log read stopped in state %q", resp.GetState())
	}
	return res, nil
}

// _ checks that Client serves the command's log and refs.
var (
	_ packsweep.LogSource = (*Client)(nil)
	_ packsweep.RefSource = (*Client)(nil)
)
