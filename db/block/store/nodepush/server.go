package block_store_nodepush

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// reportSuffix ends a report's file name. An empty file is a report the local
// store applied.
const reportSuffix = ".report"

// maxPushesInFlight bounds the pushes one connection appends at once, which
// share the log's flushes.
const maxPushesInFlight = 64

// Holder keeps pushed blocks: packahead.Log does.
type Holder interface {
	// Append returns once every block is durable.
	Append(ctx context.Context, blocks []packahead.Block) error
}

// Server is the node's end of the connection. It holds pushes in a Holder and
// reports each pack a ReportingRemote uploaded until the local store applies
// it, across restarts of either side.
type Server struct {
	holder Holder
	// dir holds one report file per uploaded pack.
	dir string

	// mtx guards out.
	mtx sync.Mutex
	// out takes frames for the live connection, nil while none is up.
	out chan []byte
}

// NewServer builds a Server keeping its reports in dir.
func NewServer(holder Holder, dir string) (*Server, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Server{holder: holder, dir: dir}, nil
}

// Serve runs conn until it ends. A newer connection replaces an older one, so
// reports go to the local store that is listening.
func (s *Server) Serve(ctx context.Context, conn net.Conn) error {
	// Publish the connection.
	ctx, cancel := context.WithCancel(ctx)
	out := make(chan []byte, 64)
	s.publish(out)

	// End the connection's work on the way out.
	var running sync.WaitGroup
	defer func() {
		cancel()
		_ = conn.Close()
		running.Wait()
		s.retire(out)
	}()

	// Write frames until the connection ends.
	running.Add(1)
	go func() {
		defer running.Done()
		writeFrames(ctx, conn, out)
		_ = conn.Close()
	}()

	// Repeat every report the local store has not applied.
	running.Add(1)
	go func() {
		defer running.Done()
		s.repeatReports(ctx, out)
	}()

	// Take pushes, appending several at once so they share a flush.
	slots := make(chan struct{}, maxPushesInFlight)
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
		case *Message_PackApplied:
			if err := s.applied(body.PackApplied.GetPackId()); err != nil {
				return err
			}
		}
	}
}

// publish makes out the live connection's frames.
func (s *Server) publish(out chan []byte) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.out = out
}

// retire clears out unless a newer connection replaced it.
func (s *Server) retire(out chan []byte) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.out == out {
		s.out = nil
	}
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

// report records pack id's report durably and offers it to the live
// connection.
func (s *Server) report(ctx context.Context, report *PackReport) error {
	// Write the report whole, then name it.
	data, err := report.MarshalVT()
	if err != nil {
		return err
	}
	path := s.reportPath(report.GetPackId())
	if err := writeDurable(path, data); err != nil {
		return err
	}

	// A connection that misses it hears it on the next repeat.
	s.mtx.Lock()
	out := s.out
	s.mtx.Unlock()
	if out != nil {
		send(ctx, out, &Message{Body: &Message_PackReport{PackReport: report}})
	}
	return nil
}

// reported reports whether pack id has a durable report, applied or not.
func (s *Server) reported(id string) bool {
	_, err := os.Stat(s.reportPath(id))
	return err == nil
}

// applied records that the local store applied pack id, keeping the empty file
// so a restarted drain knows the pack needs no new report.
func (s *Server) applied(id string) error {
	if !s.reported(id) {
		return nil
	}
	return writeDurable(s.reportPath(id), nil)
}

// repeatReports sends each unapplied report to out.
func (s *Server) repeatReports(ctx context.Context, out chan<- []byte) {
	// Walk the report files, one read at a time.
	names, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, name := range names {
		if !strings.HasSuffix(name.Name(), reportSuffix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, name.Name()))
		if err != nil || len(data) == 0 {
			continue
		}
		report := &PackReport{}
		if err := report.UnmarshalVT(data); err != nil {
			continue
		}
		send(ctx, out, &Message{Body: &Message_PackReport{PackReport: report}})
	}
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
	if err := r.server.report(ctx, &PackReport{PackId: id, Entry: encoded, Keys: keys}); err != nil {
		return nil, err
	}
	return entry, nil
}

// _ is a type assertion
var _ packahead.Remote = (*ReportingRemote)(nil)
