package block_store_nodepush

import (
	"context"

	"github.com/pkg/errors"
	kvtx_journal "github.com/s4wave/spacewave/db/kvtx/journal"
)

// ErrJournalRefused is returned by a journal call the node refused.
var ErrJournalRefused = errors.New("storage node refused the journal call")

// Connected reports whether a connection to the node is up.
func (c *Client) Connected() bool {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.out != nil
}

// SendRecord sends rec to the node's journal and returns nil once the node
// flushed it. It sets rec's id. Like Send, it returns promptly when ctx ends,
// and ErrNodeAway while no connection is up.
func (c *Client) SendRecord(ctx context.Context, rec *Record) error {
	// Take an id on the live connection.
	id, answer, out := c.begin()
	if out == nil {
		return ErrNodeAway
	}
	defer c.forget(id)

	// Send the record and wait for its answer.
	sent := rec.CloneVT()
	sent.Id = id
	return c.call(ctx, out, answer, &Message{Body: &Message_Record{Record: sent}})
}

// CutJournal has the node set aside every record except epoch's up to
// through, and returns once the cut is on the node's disk.
func (c *Client) CutJournal(ctx context.Context, epoch, through uint64) error {
	id, answer, out := c.begin()
	if out == nil {
		return ErrNodeAway
	}
	defer c.forget(id)
	return c.call(ctx, out, answer, &Message{Body: &Message_JournalCut{JournalCut: &JournalCut{Id: id, Epoch: epoch, Through: through}}})
}

// TrimJournal tells the node the local store holds epoch's records up to
// through durably. It waits for no answer, and does nothing with no
// connection up: a later trim covers it.
func (c *Client) TrimJournal(epoch, through uint64) {
	// Take the live connection.
	c.mtx.Lock()
	out := c.out
	c.mtx.Unlock()
	if out == nil {
		return
	}

	// Send the trim unless the connection's queue is full.
	frame, err := encodeMessage(&Message{Body: &Message_JournalTrim{JournalTrim: &JournalTrim{Epoch: epoch, Through: through}}})
	if err != nil {
		return
	}
	select {
	case out <- frame:
	default:
	}
}

// TailJournal returns the node's consecutive records of epoch past after,
// following More until the run ends.
func (c *Client) TailJournal(ctx context.Context, epoch, after uint64) ([]*Record, error) {
	var out []*Record
	for {
		page, err := c.tailPage(ctx, epoch, after)
		if err != nil {
			return nil, err
		}
		out = append(out, page.GetRecords()...)
		if !page.GetMore() || len(page.GetRecords()) == 0 {
			return out, nil
		}
		after = page.GetRecords()[len(page.GetRecords())-1].GetSeq()
	}
}

// tailPage asks for one frame of records past after.
func (c *Client) tailPage(ctx context.Context, epoch, after uint64) (*JournalRecords, error) {
	// Take an id on the live connection.
	c.mtx.Lock()
	if c.out == nil {
		c.mtx.Unlock()
		return nil, ErrNodeAway
	}
	c.next++
	id, out := c.next, c.out
	answer := make(chan *JournalRecords, 1)
	c.tailing[id] = answer
	c.mtx.Unlock()
	defer func() {
		c.mtx.Lock()
		delete(c.tailing, id)
		c.mtx.Unlock()
	}()

	// Ask, and wait. A closed channel means the connection ended.
	frame, err := encodeMessage(&Message{Body: &Message_JournalTail{JournalTail: &JournalTail{Id: id, Epoch: epoch, After: after}}})
	if err != nil {
		return nil, err
	}
	select {
	case out <- frame:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case page, ok := <-answer:
		if !ok {
			return nil, ErrNodeAway
		}
		return page, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// answerTail delivers a tail's page.
func (c *Client) answerTail(page *JournalRecords) {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if answer := c.tailing[page.GetId()]; answer != nil {
		answer <- page
		delete(c.tailing, page.GetId())
	}
}

// ReadBlock reads one block's stored value from the node's pack log by its
// pack key. It reports false when the node lacks it, is away, or ctx ends.
func (c *Client) ReadBlock(ctx context.Context, key []byte) ([]byte, bool) {
	data, ok, _ := c.ReadBlockAnswer(ctx, key)
	return data, ok
}

// ReadBlockAnswer reads as ReadBlock does and also reports whether the node
// answered. A node that is away, one that sent no answer before ctx ended, and
// one that answered busy did not answer; only an answered miss means the node
// lacks the block.
func (c *Client) ReadBlockAnswer(ctx context.Context, key []byte) ([]byte, bool, bool) {
	// Take a read id on the live connection.
	req, answer, out := c.beginRead()
	if out == nil {
		return nil, false, false
	}
	defer c.forgetRead(req)

	// Send the read and wait for the answer.
	frame, err := encodeMessage(&Message{Body: &Message_ReadBlock{ReadBlock: &ReadBlock{Id: req, Key: key}}})
	if err != nil {
		return nil, false, false
	}
	select {
	case out <- frame:
	case <-ctx.Done():
		return nil, false, false
	}
	select {
	case data, ok := <-answer:
		if !ok {
			return nil, false, false
		}
		if data.GetBusy() {
			return nil, false, false
		}
		if data.GetMiss() {
			return nil, false, true
		}
		return data.GetData(), true, true
	case <-ctx.Done():
		return nil, false, false
	}
}

// call sends msg and waits for its Flushed or Refused answer.
func (c *Client) call(ctx context.Context, out chan<- []byte, answer <-chan error, msg *Message) error {
	frame, err := encodeMessage(msg)
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
		if err != nil && !errors.Is(err, ErrNodeAway) {
			return errors.Wrap(ErrJournalRefused, err.Error())
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// JournalSink is the node's journal as a kvtx_journal.Sink, over a Client.
type JournalSink struct {
	// Client is the connection to the node.
	Client *Client
}

// Connected reports whether a connection to the node is up.
func (j JournalSink) Connected() bool {
	return j.Client.Connected()
}

// Append sends rec to the node's journal and returns once it is flushed.
func (j JournalSink) Append(ctx context.Context, rec *kvtx_journal.Record) error {
	// Convert the record and send it.
	writes := make([]*Write, len(rec.Writes))
	for i, w := range rec.Writes {
		writes[i] = &Write{Key: w.Key, Value: w.Value, Delete: w.Delete}
	}
	return j.Client.SendRecord(ctx, &Record{Epoch: rec.Epoch, Seq: rec.Seq, Writes: writes})
}

// Tail returns the node's consecutive records of epoch past after.
func (j JournalSink) Tail(ctx context.Context, epoch, after uint64) ([]*kvtx_journal.Record, error) {
	// Ask the node.
	records, err := j.Client.TailJournal(ctx, epoch, after)
	if err != nil {
		return nil, err
	}

	// Convert the records.
	out := make([]*kvtx_journal.Record, len(records))
	for i, rec := range records {
		writes := make([]kvtx_journal.Write, len(rec.GetWrites()))
		for k, w := range rec.GetWrites() {
			writes[k] = kvtx_journal.Write{Key: w.GetKey(), Value: w.GetValue(), Delete: w.GetDelete()}
		}
		out[i] = &kvtx_journal.Record{Epoch: rec.GetEpoch(), Seq: rec.GetSeq(), Writes: writes}
	}
	return out, nil
}

// Cut has the node set aside every record except epoch's up to through.
func (j JournalSink) Cut(ctx context.Context, epoch, through uint64) error {
	return j.Client.CutJournal(ctx, epoch, through)
}

// Trim tells the node it may drop epoch's records up to through.
func (j JournalSink) Trim(epoch, through uint64) {
	j.Client.TrimJournal(epoch, through)
}

// _ is a type assertion
var _ kvtx_journal.Sink = JournalSink{}
