//go:build !tinygo

package block_packsweep_admin

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packsweep"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/writelog"
	"github.com/s4wave/spacewave/db/packfile"
)

// Batch bounds: a call over a larger batch is refused, and the command splits
// its work to fit.
const (
	// MaxRefs is the blocks a Locate or Refs call takes.
	MaxRefs = 4096
	// MaxPacks is the packs a Status, Quarantine or Lift call names.
	MaxPacks = 1024
	// MaxAdopt is the packs an Adopt call registers: each carries every key
	// it holds.
	MaxAdopt = 16
	// MaxLines is the lines a ReadLog call returns, and its bound when the
	// request sets none.
	MaxLines = 65536
)

// ErrDisabled refuses a call that changes a pack's state while the switch is
// off.
var ErrDisabled = errors.New("the pack sweep switch is off: the backend changes no pack's state")

// PackStates is the pack store's side of a sweep, as block_store_s3.PackStore
// serves it.
type PackStates interface {
	// Holds returns the sweep state of packs ids, or of every pack the sweep
	// moved when ids is empty.
	Holds(ctx context.Context, ids []string) ([]block_store_s3.PackHold, error)
	// LocatePacks returns the registered pack the index places each block
	// in, or "" for none.
	LocatePacks(ctx context.Context, refs []*block.BlockRef) ([]string, error)
	// Quarantine holds registered packs out of the reads.
	Quarantine(ctx context.Context, ids []string) error
	// Lift returns quarantined packs to the reads.
	Lift(ctx context.Context, ids []string) error
	// AdoptPack registers a pack another writer uploaded.
	AdoptPack(ctx context.Context, entry *packfile.PackfileEntry, keys [][]byte) error
}

// Log is the backend's write log, as writelog.Log serves it.
type Log interface {
	packsweep.LogSource
	// Status returns the log's state.
	Status() writelog.Status
	// Position flushes the buffered lines and returns the open epoch's last
	// line, false while the log is off.
	Position() (writelog.Position, bool)
}

// Options configures a Server.
type Options struct {
	// Packs is the pack store. Required.
	Packs PackStates
	// Log is the write log; nil fails the log calls and reports no log.
	Log Log
	// Refs serves the child lists of the blocks the backend holds; nil fails
	// the Refs call.
	Refs packsweep.RefSource
	// Roots lists the named roots; nil fails the Roots call.
	Roots RootLister
	// Pins holds the refs the process pins outside its named roots; nil
	// reports none.
	Pins *Pins
	// Enabled is the switch, read at each call that changes a pack's state;
	// nil is always off.
	Enabled func() bool
}

// Server serves PackSweepAdmin over a backend's pack store and write log.
type Server struct {
	// opts configures the server.
	opts Options
}

// NewServer builds a Server.
func NewServer(opts Options) *Server {
	return &Server{opts: opts}
}

// enabled reports whether the switch allows changing a pack's state.
func (s *Server) enabled() bool {
	return s.opts.Enabled != nil && s.opts.Enabled()
}

// Status reports the switch, the packs' sweep states and the write log.
func (s *Server) Status(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
	// Read the named packs, or every pack the sweep moved.
	if len(req.GetPackIds()) > MaxPacks {
		return nil, errors.Errorf("status names %d packs, more than %d", len(req.GetPackIds()), MaxPacks)
	}
	holds, err := s.opts.Packs.Holds(ctx, req.GetPackIds())
	if err != nil {
		return nil, err
	}

	// Report the log beside them.
	resp := &StatusResponse{Enabled: s.enabled(), Packs: packHolds(holds)}
	if s.opts.Log != nil {
		st := s.opts.Log.Status()
		resp.Log = &LogStatus{On: st.On, Epoch: st.Epoch, Seq: st.Seq, Bytes: st.Bytes, Broken: st.Broken}
	}
	return resp, nil
}

// Locate returns the registered pack the index places each block in.
func (s *Server) Locate(ctx context.Context, req *LocateRequest) (*LocateResponse, error) {
	if err := checkRefs(req.GetRefs()); err != nil {
		return nil, err
	}
	ids, err := s.opts.Packs.LocatePacks(ctx, req.GetRefs())
	if err != nil {
		return nil, err
	}
	return &LocateResponse{PackIds: ids}, nil
}

// Refs returns the child lists of blocks the backend holds.
func (s *Server) Refs(ctx context.Context, req *RefsRequest) (*RefsResponse, error) {
	// Check the batch and that the backend serves refs.
	if err := checkRefs(req.GetRefs()); err != nil {
		return nil, err
	}
	if s.opts.Refs == nil {
		return nil, errors.New("the backend serves no block refs")
	}

	// Read and convert each block's children.
	results, err := s.opts.Refs.BlockRefs(ctx, req.GetRefs())
	if err != nil {
		return nil, err
	}
	if len(results) != len(req.GetRefs()) {
		return nil, errors.Errorf("the ref source answered %d of %d blocks", len(results), len(req.GetRefs()))
	}
	resp := &RefsResponse{Results: make([]*BlockRefs, len(results))}
	for i, r := range results {
		resp.Results[i] = &BlockRefs{Refs: r.Refs, Reason: r.Reason}
	}
	return resp, nil
}

// Quarantine holds registered packs out of the reads.
func (s *Server) Quarantine(ctx context.Context, req *PacksRequest) (*PacksResponse, error) {
	return s.move(ctx, req, s.opts.Packs.Quarantine)
}

// Lift returns quarantined packs to the reads.
func (s *Server) Lift(ctx context.Context, req *PacksRequest) (*PacksResponse, error) {
	return s.move(ctx, req, s.opts.Packs.Lift)
}

// move runs a state move on the packs a request names, and reports their
// states after it.
func (s *Server) move(ctx context.Context, req *PacksRequest, fn func(context.Context, []string) error) (*PacksResponse, error) {
	// Refuse while the switch is off, and an empty or unbounded batch.
	if !s.enabled() {
		return nil, ErrDisabled
	}
	ids := req.GetPackIds()
	if len(ids) == 0 || len(ids) > MaxPacks {
		return nil, errors.Errorf("a move names %d packs, want 1 to %d", len(ids), MaxPacks)
	}

	// Move the packs, then read their states back.
	if err := fn(ctx, ids); err != nil {
		return nil, err
	}
	holds, err := s.opts.Packs.Holds(ctx, ids)
	if err != nil {
		return nil, err
	}
	return &PacksResponse{Packs: packHolds(holds)}, nil
}

// Adopt registers packs another writer uploaded.
func (s *Server) Adopt(ctx context.Context, req *AdoptRequest) (*AdoptResponse, error) {
	// Refuse while the switch is off, and an empty or unbounded batch.
	if !s.enabled() {
		return nil, ErrDisabled
	}
	packs := req.GetPacks()
	if len(packs) == 0 || len(packs) > MaxAdopt {
		return nil, errors.Errorf("an adoption names %d packs, want 1 to %d", len(packs), MaxAdopt)
	}

	// Adopt each pack; a pack adopted before changes nothing.
	for _, p := range packs {
		if p.GetEntry().GetId() == "" {
			return nil, errors.New("an adopted pack has no entry")
		}
		if err := s.opts.Packs.AdoptPack(ctx, p.GetEntry(), p.GetKeys()); err != nil {
			return nil, errors.Wrap(err, "adopt pack "+p.GetEntry().GetId())
		}
	}
	return &AdoptResponse{}, nil
}

// ReadLog returns the write log's lines after a position.
func (s *Server) ReadLog(ctx context.Context, req *ReadLogRequest) (*ReadLogResponse, error) {
	// Parse the position and bound the lines.
	if s.opts.Log == nil {
		return nil, errors.New("the backend has no write log")
	}
	from, err := writelog.ParsePosition(req.GetFrom())
	if err != nil {
		return nil, err
	}
	limit := int(req.GetMax())
	if limit <= 0 || limit > MaxLines {
		limit = MaxLines
	}

	// Read the lines.
	resp := &ReadLogResponse{}
	res, err := s.opts.Log.ReadLog(ctx, from, limit, func(l writelog.Line) error {
		resp.Lines = append(resp.Lines, &LogLine{Seq: l.Seq, Kind: string(rune(l.Kind)), Hash: l.Hash})
		return nil
	})
	if err != nil {
		return nil, err
	}
	resp.State, resp.Next, resp.Reason = res.State.String(), res.Next.String(), res.Reason
	return resp, nil
}

// TrimLog deletes the write log's lines at or before a position. It changes
// no pack's state, so it runs while the switch is off: the backend's copy of
// the log must stay within its cap through a dry run.
func (s *Server) TrimLog(ctx context.Context, req *TrimLogRequest) (*TrimLogResponse, error) {
	// Parse the position and trim the log to it.
	if s.opts.Log == nil {
		return nil, errors.New("the backend has no write log")
	}
	to, err := writelog.ParsePosition(req.GetTo())
	if err != nil {
		return nil, err
	}
	if err := s.opts.Log.TrimLog(ctx, to); err != nil {
		return nil, err
	}
	return &TrimLogResponse{}, nil
}

// checkRefs refuses an unbounded batch of blocks or an invalid ref.
func checkRefs(refs []*block.BlockRef) error {
	if len(refs) > MaxRefs {
		return errors.Errorf("%d blocks, more than %d", len(refs), MaxRefs)
	}
	for _, ref := range refs {
		if err := ref.Validate(false); err != nil {
			return err
		}
	}
	return nil
}

// parsePosition parses a log position, empty for the start of the oldest
// epoch.
func parsePosition(s string) (writelog.Position, error) {
	if s == "" {
		return writelog.Position{}, nil
	}
	return writelog.ParsePosition(s)
}

// packHolds converts the pack store's states to messages: the enum numbers the
// states as block_store_s3 does.
func packHolds(holds []block_store_s3.PackHold) []*PackHold {
	out := make([]*PackHold, len(holds))
	for i, h := range holds {
		out[i] = &PackHold{PackId: h.ID, State: PackState(h.State), SinceUnixNano: h.Since, EventUnixNano: h.Event, Key: h.Key}
	}
	return out
}

// _ checks that Server serves the service.
var _ SRPCPackSweepAdminServer = (*Server)(nil)
