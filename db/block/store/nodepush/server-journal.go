package block_store_nodepush

import (
	"context"

	"github.com/aperturerobotics/util/conc"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// journalTailBytes bounds the records one JournalRecords frame carries.
const journalTailBytes = maxMessageBytes / 2

// blockGetter reads one block of the node's pack log by key: packahead.Log
// does.
type blockGetter interface {
	// Get returns the block's stored value, or false when the log lacks it.
	Get(key []byte) ([]byte, bool, error)
}

// _ is a type assertion: the node's log must serve ReadBlock.
var _ blockGetter = (*packahead.Log)(nil)

// SetJournal keeps the local store's commit records in journal. Call it
// before Serve.
func (s *Server) SetJournal(journal *Journal) {
	s.journal = journal
}

// serveJournal handles the journal's messages and block reads for Serve. A
// record takes one of slots, as a push does, so it waits for nothing but a
// slot and its own flush.
func (s *Server) serveJournal(ctx context.Context, msg *Message, out chan<- []byte, slots chan struct{}, running *conc.ConcurrentQueue) error {
	switch body := msg.GetBody().(type) {
	case *Message_Record:
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		running.Enqueue(func() {
			defer func() { <-slots }()
			send(ctx, out, s.appendRecord(body.Record))
		})
	case *Message_JournalTail:
		var answer *JournalRecords
		if s.journal != nil {
			records, more := s.journal.Tail(body.JournalTail.GetEpoch(), body.JournalTail.GetAfter(), journalTailBytes)
			answer = &JournalRecords{Id: body.JournalTail.GetId(), Records: records, More: more}
		} else {
			answer = &JournalRecords{Id: body.JournalTail.GetId()}
		}
		send(ctx, out, &Message{Body: &Message_JournalRecords{JournalRecords: answer}})
	case *Message_JournalCut:
		// Cut beside the read loop, which keeps reading pushes meanwhile.
		cut := body.JournalCut
		running.Enqueue(func() {
			// Take every slot, so no record is appending during the cut.
			for range cap(slots) {
				slots <- struct{}{}
			}
			defer func() {
				for range cap(slots) {
					<-slots
				}
			}()

			// Cut, and answer.
			if s.journal == nil {
				send(ctx, out, refused(cut.GetId(), "node keeps no journal"))
				return
			}
			if err := s.journal.Cut(cut.GetEpoch(), cut.GetThrough()); err != nil {
				send(ctx, out, refused(cut.GetId(), err.Error()))
				return
			}
			send(ctx, out, &Message{Body: &Message_Flushed{Flushed: &Flushed{Id: cut.GetId()}}})
		})
	case *Message_JournalTrim:
		if s.journal != nil {
			// A failed compaction keeps the file as it was; the next trim
			// tries again.
			_ = s.journal.Trim(body.JournalTrim.GetEpoch(), body.JournalTrim.GetThrough())
		}
	case *Message_ReadBlock:
		// As for a Read: never wait for a slot. Without one, answer busy,
		// which is not a miss: the node did not look.
		select {
		case s.readSlots <- struct{}{}:
			go func() {
				defer func() { <-s.readSlots }()
				send(ctx, out, s.readBlock(body.ReadBlock))
			}()
		default:
			trySend(out, &Message{Body: &Message_ReadData{ReadData: &ReadData{Id: body.ReadBlock.GetId(), Miss: true, Busy: true}}})
		}
	}
	return nil
}

// appendRecord makes rec durable in the journal and returns its answer.
func (s *Server) appendRecord(rec *Record) *Message {
	if s.journal == nil {
		return refused(rec.GetId(), "node keeps no journal")
	}
	if err := s.journal.Append(rec); err != nil {
		return refused(rec.GetId(), err.Error())
	}
	return &Message{Body: &Message_Flushed{Flushed: &Flushed{Id: rec.GetId()}}}
}

// readBlock answers req from the node's pack log.
func (s *Server) readBlock(req *ReadBlock) *Message {
	// A failed read did not look, so it answers busy, not a miss.
	answer := &ReadData{Id: req.GetId(), Miss: true}
	if g, ok := s.holder.(blockGetter); ok {
		data, found, err := g.Get(req.GetKey())
		switch {
		case err != nil:
			answer.Busy = true
		case found:
			answer.Data, answer.Miss = data, false
		}
	}
	return &Message{Body: &Message_ReadData{ReadData: answer}}
}

// refused returns a Refused answer to frame id.
func refused(id uint64, reason string) *Message {
	return &Message{Body: &Message_Refused{Refused: &Refused{Id: id, Reason: reason}}}
}
