package block_store_nodepush

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// reportSuffix ends a report's file name. An empty file is a report the local
// store applied.
const reportSuffix = ".report"

// maxPushesInFlight bounds the pushes one connection appends at once, which
// share the log's flushes.
const maxPushesInFlight = 16

// maxReadsInFlight bounds the reads one connection serves at once.
const maxReadsInFlight = 32

// reportWait is how long a report waits for its answer before the next one
// takes its turn.
const reportWait = 30 * time.Second

// maxReportTries bounds the tries of one report per process, so a report that
// can never apply, such as for a pack since merged away, stops taking turns.
// Its blocks stay queued for the local upload, and its file keeps it for the
// next start.
const maxReportTries = 20

// Holder keeps pushed blocks: packahead.Log does.
type Holder interface {
	// Append returns once every block is durable.
	Append(ctx context.Context, blocks []packahead.Block) error
}

// Server is the node's end of the connection. It holds pushes in a Holder and
// reports each pack a ReportingRemote uploaded until the local store applies
// it, across restarts of either side.
//
// One report is in flight at a time, sent by the connection's own routine and
// repeated until answered. So reports can't fill the connection that carries
// the answers commits wait for, and the upload never waits on a connection.
type Server struct {
	holder Holder
	// dir holds one report file per uploaded pack.
	dir string
	// wait is how long a report waits for its answer.
	wait time.Duration
	// wake tells the report routine that pending changed.
	wake chan struct{}
	// packs serves reads, or is nil on a node that serves none.
	packs *PackFiles

	// mtx guards pending.
	mtx sync.Mutex
	// pending lists the reports the local store has not applied, next first.
	pending []*pendingReport
}

// pendingReport is a report not yet applied.
type pendingReport struct {
	id    string
	tries int
}

// NewServer builds a Server keeping its reports in dir, and resumes the
// reports an earlier process left unapplied.
func NewServer(holder Holder, dir string) (*Server, error) {
	// Open the directory.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Server{holder: holder, dir: dir, wait: reportWait, wake: make(chan struct{}, 1)}

	// An empty report was applied. The sizes come from the directory walk, so
	// this reads no report.
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		id, ok := strings.CutSuffix(name.Name(), reportSuffix)
		if !ok {
			continue
		}
		if info, err := name.Info(); err == nil && info.Size() != 0 {
			s.pending = append(s.pending, &pendingReport{id: id})
		}
	}
	return s, nil
}

// SetPacks serves reads from packs. Call it before Serve.
func (s *Server) SetPacks(packs *PackFiles) {
	s.packs = packs
}

// Serve runs conn until it ends.
func (s *Server) Serve(ctx context.Context, conn net.Conn) error {
	// End the connection's work on the way out.
	ctx, cancel := context.WithCancel(ctx)
	out := make(chan []byte, maxFramesQueued)
	var running sync.WaitGroup
	defer func() {
		cancel()
		_ = conn.Close()
		running.Wait()
	}()

	// Write frames until the connection ends.
	running.Add(2)
	go func() {
		defer running.Done()
		writeFrames(ctx, conn, out)
		_ = conn.Close()
	}()

	// Send the pending reports, one at a time.
	go func() {
		defer running.Done()
		s.sendReports(ctx, out)
	}()

	// Take pushes, appending several at once so they share a flush, and
	// reads, in their own slots so a burst of them never holds back a push.
	slots := make(chan struct{}, maxPushesInFlight)
	readSlots := make(chan struct{}, maxReadsInFlight)
	r := bufio.NewReader(conn)
	for {
		msg, err := readMessage(r)
		if err != nil {
			return err
		}
		switch body := msg.GetBody().(type) {
		case *Message_Push:
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			running.Add(1)
			go func() {
				defer running.Done()
				defer func() { <-slots }()
				send(ctx, out, s.hold(ctx, body.Push))
			}()
		case *Message_Read:
			select {
			case readSlots <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			running.Add(1)
			go func() {
				defer running.Done()
				defer func() { <-readSlots }()
				send(ctx, out, s.read(body.Read))
			}()
		case *Message_PackApplied:
			if err := s.applied(body.PackApplied.GetPackId()); err != nil {
				return err
			}
		}
	}
}

// read answers req from the pack files.
func (s *Server) read(req *Read) *Message {
	answer := &ReadData{Id: req.GetId(), Miss: true}
	if s.packs != nil {
		if data, ok := s.packs.ReadRange(req.GetPackId(), req.GetOff(), req.GetLength()); ok {
			answer.Data, answer.Miss = data, false
		}
	}
	return &Message{Body: &Message_ReadData{ReadData: answer}}
}

// hold makes push durable and returns its answer.
func (s *Server) hold(ctx context.Context, push *Push) *Message {
	// Check the batch against the sum the local store computed.
	refuse := func(reason string) *Message {
		return &Message{Body: &Message_Refused{Refused: &Refused{Id: push.GetId(), Reason: reason}}}
	}
	blocks := push.GetBlocks()
	if !bytes.Equal(blocksSum(blocks), push.GetSum()) {
		return refuse("push sum mismatch")
	}

	// Answer once the blocks are on disk.
	held := make([]packahead.Block, len(blocks))
	for i, b := range blocks {
		held[i] = packahead.Block{Key: b.GetKey(), Value: b.GetData()}
	}
	if err := s.holder.Append(ctx, held); err != nil {
		return refuse(err.Error())
	}
	return &Message{Body: &Message_Flushed{Flushed: &Flushed{Id: push.GetId()}}}
}

// report records pack id's report durably and queues it. It waits on no
// connection, so the upload carries on with the local store away.
func (s *Server) report(report *PackReport) error {
	// Write the report whole, then name it.
	data, err := report.MarshalVT()
	if err != nil {
		return err
	}
	if err := writeDurable(s.reportPath(report.GetPackId()), data); err != nil {
		return err
	}

	// Queue it unless an earlier upload of the same pack did.
	s.mtx.Lock()
	if !slices.ContainsFunc(s.pending, func(p *pendingReport) bool { return p.id == report.GetPackId() }) {
		s.pending = append(s.pending, &pendingReport{id: report.GetPackId()})
	}
	s.mtx.Unlock()
	s.poke()
	return nil
}

// poke wakes the report routine.
func (s *Server) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// reported reports whether pack id has a durable report, applied or not.
func (s *Server) reported(id string) bool {
	_, err := os.Stat(s.reportPath(id))
	return err == nil
}

// applied records that the local store applied pack id, keeping the empty file
// so a restarted drain knows the pack needs no new report.
func (s *Server) applied(id string) error {
	// Ignore a repeat.
	if !s.reported(id) {
		return nil
	}
	if err := writeDurable(s.reportPath(id), nil); err != nil {
		return err
	}

	// Let the next report take its turn.
	s.mtx.Lock()
	s.pending = slices.DeleteFunc(s.pending, func(p *pendingReport) bool { return p.id == id })
	s.mtx.Unlock()
	s.poke()
	return nil
}

// sendReports sends the pending reports to out, one at a time, until ctx ends.
func (s *Server) sendReports(ctx context.Context, out chan<- []byte) {
	var sent string
	timer := time.NewTimer(s.wait)
	defer timer.Stop()
	for {
		// Send the next report, unless it is the one still in flight.
		if id := s.nextReport(); id != "" && id != sent {
			s.sendReport(ctx, out, id)
			sent = id
			timer.Reset(s.wait)
		}

		// Wait for a change, or for the report's turn to end.
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-timer.C:
			s.rotate(sent)
			sent = ""
			timer.Reset(s.wait)
		}
	}
}

// nextReport returns the report whose turn it is, or "" with none pending.
func (s *Server) nextReport() string {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if len(s.pending) == 0 {
		return ""
	}
	return s.pending[0].id
}

// rotate ends report id's turn, sending it to the back of the line, or out of
// it once it used its tries.
func (s *Server) rotate(id string) {
	// Find the report, which may have been applied meanwhile.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	i := slices.IndexFunc(s.pending, func(p *pendingReport) bool { return p.id == id })
	if i < 0 {
		return
	}

	// Take it out, and put it back if it has tries left.
	report := s.pending[i]
	s.pending = slices.Delete(s.pending, i, i+1)
	if report.tries++; report.tries < maxReportTries {
		s.pending = append(s.pending, report)
	}
}

// sendReport sends report id from its file.
func (s *Server) sendReport(ctx context.Context, out chan<- []byte, id string) {
	// A file that can't be read waits for its turn to end.
	data, err := os.ReadFile(s.reportPath(id))
	if err != nil {
		return
	}
	report := &PackReport{}
	if err := report.UnmarshalVT(data); err != nil {
		return
	}
	send(ctx, out, &Message{Body: &Message_PackReport{PackReport: report}})
}

// reportPath returns pack id's report file.
func (s *Server) reportPath(id string) string {
	return filepath.Join(s.dir, id+reportSuffix)
}

// send writes msg to out unless ctx ends first. A frame too large to encode is
// dropped: the local upload still holds its blocks.
func send(ctx context.Context, out chan<- []byte, msg *Message) {
	frame, err := encodeMessage(msg)
	if err != nil {
		return
	}
	select {
	case out <- frame:
	case <-ctx.Done():
	}
}

// writeDurable replaces path with data, surviving a crash at any point.
func writeDurable(path string, data []byte) error {
	// Write a temporary file and flush it.
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- the path is under the server's own directory
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}

	// Name it, and flush the name.
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// ReportingRemote is the node's upload target: a packahead.Remote that records
// a report for each pack it uploads.
//
// Invariant: a pack counts as uploaded only once its report is durable. A crash
// between the upload and the report leaves HasPackfile false, so the drain
// uploads the same bytes again and reports it, rather than skipping a pack the
// local store would never hear of.
type ReportingRemote struct {
	remote packahead.Remote
	server *Server
}

// NewReportingRemote wraps remote, reporting through server.
func NewReportingRemote(remote packahead.Remote, server *Server) *ReportingRemote {
	return &ReportingRemote{remote: remote, server: server}
}

// HasPackfile reports whether the pack has a durable report, which is written
// only after the remote took the pack. So the node never lists the remote.
func (r *ReportingRemote) HasPackfile(_ context.Context, id string) (bool, error) {
	return r.server.reported(id), nil
}

// PutPackFile uploads a pack, then reports it.
func (r *ReportingRemote) PutPackFile(ctx context.Context, id string, data []byte) (*packfile.PackfileEntry, error) {
	// Upload the pack.
	entry, err := r.remote.PutPackFile(ctx, id, data)
	if err != nil {
		return nil, err
	}

	// Report it with what the local store needs to find its blocks.
	index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return nil, errors.Wrap(err, "read packfile index")
	}
	keys := make([][]byte, len(index))
	for i, e := range index {
		keys[i] = e.GetKey()
	}
	encoded, err := entry.MarshalVT()
	if err != nil {
		return nil, err
	}
	if err := r.server.report(&PackReport{PackId: id, Entry: encoded, Keys: keys}); err != nil {
		return nil, err
	}

	// Keep the pack for reads, beyond the log's eviction of it.
	if r.server.packs != nil {
		r.server.packs.Keep(id, data)
	}
	return entry, nil
}

// _ is a type assertion
var _ packahead.Remote = (*ReportingRemote)(nil)
