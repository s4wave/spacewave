package block_store_nodepush

import (
	"bufio"
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/conc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// maxFramesQueued bounds the frames waiting for the socket, each at most
// maxMessageBytes, including those of pushes that gave up.
const maxFramesQueued = 16

// ErrNodeAway is returned by Send while no connection to the node is up.
var ErrNodeAway = errors.New("storage node away")

// Applier takes the packs the node reports.
type Applier interface {
	// ApplyPack makes report's pack readable through the local pack index,
	// durably, and only then clears the queue entries of its blocks. It returns
	// nil only once that clear is done, since the node never repeats a report
	// it heard applied. With the clear's switch off, by design, it returns nil
	// having cleared nothing, and the local upload carries those entries. A
	// failure, such as a failed index lookup, answers the report failed, and
	// the node repeats it.
	ApplyPack(ctx context.Context, report *PackReport) error
}

// Client is the local store's end of the connection, and the Sender a
// Publisher pushes through.
type Client struct {
	le *logrus.Entry
	// applier takes the reports; SetApplier replaces it.
	applier atomic.Pointer[Applier]

	// mtx guards the fields below.
	mtx sync.Mutex
	// out takes frames for the live connection, nil while none is up.
	out chan []byte
	// next numbers the pushes.
	next uint64
	// waiting holds the answer channel of each push in flight.
	waiting map[uint64]chan error
	// reading holds the answer channel of each read in flight.
	reading map[uint64]chan *ReadData
	// tailing holds the answer channel of each journal tail in flight.
	tailing map[uint64]chan *JournalRecords
}

// NewClient builds a Client that hands reported packs to applier.
func NewClient(le *logrus.Entry, applier Applier) *Client {
	c := &Client{le: le, waiting: make(map[uint64]chan error), reading: make(map[uint64]chan *ReadData), tailing: make(map[uint64]chan *JournalRecords)}
	c.SetApplier(applier)
	return c
}

// SetApplier hands later reports to applier. A nil applier answers each
// report failed, so the node sends it again.
func (c *Client) SetApplier(applier Applier) {
	c.applier.Store(&applier)
}

// Run keeps a connection to the node up through dial until ctx ends.
func (c *Client) Run(ctx context.Context, dial func(context.Context) (net.Conn, error), retry time.Duration) error {
	for {
		// Serve a connection until it ends, then dial again after a pause.
		conn, err := dial(ctx)
		if err == nil {
			err = c.serve(ctx, conn)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.le.WithError(err).Debug("storage node connection ended")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retry):
		}
	}
}

// Send pushes entries and waits for the node's flush.
func (c *Client) Send(ctx context.Context, entries []*block.PutBatchEntry) error {
	// Encode first: nothing reads entries after this.
	blocks := make([]*Block, 0, len(entries))
	for _, entry := range entries {
		value, err := block.EncodeBlockObject(entry.Data, entry.Refs, entry.RefsKnown)
		if err != nil {
			return err
		}
		blocks = append(blocks, &Block{Key: packfile.BlockKey(entry.Ref.GetHash()), Data: value})
	}

	// Take a push id on the live connection.
	id, answer, out := c.begin()
	if out == nil {
		return ErrNodeAway
	}
	defer c.forget(id)

	// Send the push and wait for its answer.
	frame, err := encodeMessage(&Message{Body: &Message_Push{Push: &Push{Id: id, Blocks: blocks, Sum: blocksSum(blocks)}}})
	if err != nil {
		return err
	}
	select {
	case out <- frame:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-answer:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// begin numbers a push on the live connection and returns its id, its answer
// channel and the connection's frames, nil while no connection is up.
func (c *Client) begin() (uint64, chan error, chan<- []byte) {
	// Only a live connection takes a push.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.out == nil {
		return 0, nil, nil
	}

	// Number it and keep its answer channel.
	c.next++
	answer := make(chan error, 1)
	c.waiting[c.next] = answer
	return c.next, answer, c.out
}

// ReadRange asks the node for a range of pack id. It reports false when the
// node lacks the pack, is away, or ctx ends first.
func (c *Client) ReadRange(ctx context.Context, id string, off int64, length int) ([]byte, bool) {
	// Take a read id on the live connection.
	req, answer, out := c.beginRead()
	if out == nil {
		return nil, false
	}
	defer c.forgetRead(req)

	// Send the read.
	frame, err := encodeMessage(&Message{Body: &Message_Read{Read: &Read{Id: req, PackId: id, Off: off, Length: uint32(max(length, 0))}}}) //nolint:gosec // a range is far below 4 GiB
	if err != nil {
		return nil, false
	}
	select {
	case out <- frame:
	case <-ctx.Done():
		return nil, false
	}

	// Wait for the answer. A closed channel means the connection ended.
	select {
	case data, ok := <-answer:
		if !ok || data.GetMiss() {
			return nil, false
		}
		return data.GetData(), true
	case <-ctx.Done():
		return nil, false
	}
}

// beginRead numbers a read on the live connection and returns its id, its
// answer channel and the connection's frames, nil while no connection is up.
func (c *Client) beginRead() (uint64, chan *ReadData, chan<- []byte) {
	// Only a live connection takes a read.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.out == nil {
		return 0, nil, nil
	}

	// Number it and keep its answer channel.
	c.next++
	answer := make(chan *ReadData, 1)
	c.reading[c.next] = answer
	return c.next, answer, c.out
}

// forgetRead drops read id's answer channel.
func (c *Client) forgetRead(id uint64) {
	// The answer may have removed it already.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	delete(c.reading, id)
}

// answerRead delivers the answer of a read, which holds one answer.
func (c *Client) answerRead(data *ReadData) {
	// A read that gave up has no channel left.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if answer := c.reading[data.GetId()]; answer != nil {
		answer <- data
		delete(c.reading, data.GetId())
	}
}

// forget drops push id's answer channel.
func (c *Client) forget(id uint64) {
	c.mtx.Lock()
	delete(c.waiting, id)
	c.mtx.Unlock()
}

// answer delivers the result of push id.
func (c *Client) answer(id uint64, err error) {
	// A push that gave up has no channel left.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if answer := c.waiting[id]; answer != nil {
		deliver(answer, err)
	}
}

// deliver gives a push its answer. The first answer wins: a connection that
// drops just after a push's answer must not block on the full channel while it
// holds the lock every Send takes.
func deliver(answer chan<- error, err error) {
	// A full channel already holds the first answer.
	select {
	case answer <- err:
	default:
	}
}

// serve runs one connection until it ends. Pushes in flight then fail, and
// their blocks stay queued for the local upload.
func (c *Client) serve(ctx context.Context, conn net.Conn) error {
	// Publish the connection, and on the way out fail what it carried.
	ctx, cancel := context.WithCancel(ctx)
	out := make(chan []byte, maxFramesQueued)
	c.mtx.Lock()
	c.out = out
	c.mtx.Unlock()
	running := conc.NewConcurrentQueue(0)
	defer func() {
		cancel()
		_ = conn.Close()
		_ = running.WaitIdle(context.Background(), nil)
		c.disconnect()
	}()

	// Write frames, and apply reports off the read loop. The unbounded
	// running queue starts each at once and reports idle when both returned.
	running.Enqueue(func() {
		writeFrames(ctx, conn, out)
		_ = conn.Close()
	})
	reports := make(chan *PackReport, 2)
	running.Enqueue(func() {
		c.applyReports(ctx, reports, out)
	})

	// Read answers and reports until the connection ends.
	r := bufio.NewReader(conn)
	for {
		msg, err := readMessage(r)
		if err != nil {
			return err
		}
		switch body := msg.GetBody().(type) {
		case *Message_Flushed:
			c.answer(body.Flushed.GetId(), nil)
		case *Message_Refused:
			c.answer(body.Refused.GetId(), errors.New(body.Refused.GetReason()))
		case *Message_ReadData:
			c.answerRead(body.ReadData)
		case *Message_JournalRecords:
			c.answerTail(body.JournalRecords)
		case *Message_PackReport:
			// Never block here: this loop carries the answers every commit
			// waits for. The node sends one report at a time and repeats it,
			// so a report that does not fit is heard again.
			select {
			case reports <- body.PackReport:
			default:
			}
		}
	}
}

// disconnect marks the node away and fails the pushes in flight.
func (c *Client) disconnect() {
	// Take the connection down, then end everything that waits on it.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.out = nil
	for id, answer := range c.waiting {
		deliver(answer, ErrNodeAway)
		delete(c.waiting, id)
	}
	for id, answer := range c.reading {
		close(answer)
		delete(c.reading, id)
	}
	for id, answer := range c.tailing {
		close(answer)
		delete(c.tailing, id)
	}
}

// applyReports applies each report and answers it. The node sends a report that
// failed again later.
func (c *Client) applyReports(ctx context.Context, reports <-chan *PackReport, out chan<- []byte) {
	for {
		// Take the next report.
		var report *PackReport
		select {
		case <-ctx.Done():
			return
		case report = <-reports:
		}

		// Apply it, and say so.
		var err error
		if applier := c.applier.Load(); applier != nil && *applier != nil {
			err = (*applier).ApplyPack(ctx, report)
		} else {
			err = errors.New("no applier for reports yet")
		}
		if err != nil {
			c.le.WithError(err).WithField("pack", report.GetPackId()).Warn("unable to apply a reported pack")
		}
		frame, err := encodeMessage(&Message{Body: &Message_PackApplied{PackApplied: &PackApplied{PackId: report.GetPackId(), Failed: err != nil}}})
		if err != nil {
			continue
		}
		select {
		case out <- frame:
		case <-ctx.Done():
			return
		}
	}
}

// writeFrames writes out's frames to conn until ctx ends or a write fails.
func writeFrames(ctx context.Context, conn net.Conn, out <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-out:
			if _, err := conn.Write(frame); err != nil {
				return
			}
		}
	}
}

// _ is a type assertion
var _ Sender = (*Client)(nil)
