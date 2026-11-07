//go:build !tinygo

package block_packsweep_admin

import (
	"context"
	"time"

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

// Backend serves the client as the backend's side of a cycle.
func (c *Client) Backend() packsweep.Backend {
	return backend{c}
}

// backend is the client as a packsweep.Backend.
type backend struct {
	*Client
}

// Status returns the packs' states, the log's and the open operations.
func (b backend) Status(ctx context.Context, ids []string) (*packsweep.BackendStatus, error) {
	// Ask, then convert the answer.
	resp, err := b.SRPCPackSweepAdminClient.Status(ctx, &StatusRequest{PackIds: ids})
	if err != nil {
		return nil, err
	}
	packs, err := sweepHolds(resp.GetPacks())
	if err != nil {
		return nil, err
	}
	st := &packsweep.BackendStatus{
		Packs:    packs,
		LogOn:    resp.GetLog().GetOn(),
		LogEpoch: resp.GetLog().GetEpoch(),
		Now:      fromUnixNano(resp.GetNowUnixNano()),
	}
	if ops := resp.GetOperations(); ops != nil {
		st.Operations = &packsweep.OpenOperations{
			Write: fromUnixNano(ops.GetOldestWriteUnixNano()),
			Read:  fromUnixNano(ops.GetOldestReadUnixNano()),
		}
	}
	return st, nil
}

// Quarantine holds registered packs out of the reads.
func (b backend) Quarantine(ctx context.Context, ids []string) ([]packsweep.PackHold, error) {
	resp, err := b.SRPCPackSweepAdminClient.Quarantine(ctx, &PacksRequest{PackIds: ids})
	if err != nil {
		return nil, err
	}
	return sweepHolds(resp.GetPacks())
}

// Lift returns quarantined packs to the reads.
func (b backend) Lift(ctx context.Context, ids []string) ([]packsweep.PackHold, error) {
	resp, err := b.SRPCPackSweepAdminClient.Lift(ctx, &PacksRequest{PackIds: ids})
	if err != nil {
		return nil, err
	}
	return sweepHolds(resp.GetPacks())
}

// BeginTrash moves quarantined packs to the trashing state.
func (b backend) BeginTrash(ctx context.Context, ids []string) ([]packsweep.PackHold, error) {
	resp, err := b.SRPCPackSweepAdminClient.BeginTrash(ctx, &PacksRequest{PackIds: ids})
	if err != nil {
		return nil, err
	}
	return sweepHolds(resp.GetPacks())
}

// Remove repairs the h rows of keys and drops the trashing packs ids, or
// only repairs the rows when more is set.
func (b backend) Remove(ctx context.Context, ids []string, keys []packsweep.KeyHolders, more bool) ([]packsweep.PackHold, error) {
	// Build the request, then send it.
	req := &RemoveRequest{PackIds: ids, Keys: make([]*KeyHolders, len(keys)), More: more}
	for i, k := range keys {
		req.Keys[i] = &KeyHolders{Key: k.Key, Others: k.Others}
	}
	resp, err := b.SRPCPackSweepAdminClient.Remove(ctx, req)
	if err != nil {
		return nil, err
	}
	return sweepHolds(resp.GetPacks())
}

// Restore registers again trashing packs whose entries and files are back.
func (b backend) Restore(ctx context.Context, ids []string) ([]packsweep.PackHold, error) {
	resp, err := b.SRPCPackSweepAdminClient.Restore(ctx, &PacksRequest{PackIds: ids})
	if err != nil {
		return nil, err
	}
	return sweepHolds(resp.GetPacks())
}

// Adopt registers packs whose entries and files are in the store.
func (b backend) Adopt(ctx context.Context, packs []packsweep.AdoptedPack) error {
	req := &AdoptRequest{Packs: make([]*AdoptPack, len(packs))}
	for i, p := range packs {
		req.Packs[i] = &AdoptPack{Entry: p.Entry, Keys: p.Keys}
	}
	_, err := b.SRPCPackSweepAdminClient.Adopt(ctx, req)
	return err
}

// sweepHolds converts the service's pack states, refusing a state this
// client does not know, which a sweep must not read as quarantined.
func sweepHolds(holds []*PackHold) ([]packsweep.PackHold, error) {
	out := make([]packsweep.PackHold, len(holds))
	for i, h := range holds {
		state := h.GetState()
		if state < PackState_PACK_STATE_REGISTERED || state > PackState_PACK_STATE_TRASHING {
			return nil, errors.Errorf("pack %s is in state %d, which this client does not know", h.GetPackId(), state)
		}
		out[i] = packsweep.PackHold{
			ID:     h.GetPackId(),
			State:  packsweep.PackState(state),
			Since:  fromUnixNano(h.GetSinceUnixNano()),
			Event:  fromUnixNano(h.GetEventUnixNano()),
			Absent: h.GetAbsent(),
		}
	}
	return out, nil
}

// fromUnixNano returns a time from Unix nanoseconds, the zero time for zero.
func fromUnixNano(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(0, v)
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
