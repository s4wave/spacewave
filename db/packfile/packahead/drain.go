package packahead

import (
	"context"
	"os"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
)

// Remote is the pack store a Drain uploads finalized packs to.
type Remote interface {
	// HasPackfile reports whether the store lists the pack.
	HasPackfile(ctx context.Context, id string) (bool, error)
	// PutPackFile uploads a finalized pack as it is under id. It returns once
	// the store holds the pack's object and entry and its index registers the
	// pack's keys.
	PutPackFile(ctx context.Context, id string, data []byte) (*packfile.PackfileEntry, error)
}

// DrainOptions configures a Drain.
type DrainOptions struct {
	// KeepBytes is the size of finalized packs kept local after upload, as a
	// read cache. Beyond it the oldest uploaded packs are evicted. Zero keeps
	// every pack, uploading without evicting.
	KeepBytes int64
	// Admit paces the uploads: it is called before each with the bytes of the
	// previous one, and may wait. Nil uploads at once.
	Admit func(ctx context.Context, lastBytes int64) error
}

// Drain uploads a Log's finalized packs to one remote and evicts the uploaded
// ones beyond a budget. Build one per Log: the Log's upload marks name the
// one remote it drains to.
//
// Invariant: a pack is evicted only after the Log marked it uploaded, which
// the Drain does once it saw the remote list the pack or PutPackFile return
// for it, so every evicted value is on the remote.
type Drain struct {
	// log is the drained Log.
	log *Log
	// opts are the options.
	opts DrainOptions
	// written counts the packs PutPackFile wrote, and writtenBytes their size.
	written, writtenBytes atomic.Uint64
	// listed counts the packs the remote listed already, uploading nothing.
	listed atomic.Uint64
	// failures counts the Run calls that ended in an error other than their
	// context's.
	failures atomic.Uint64
}

// DrainStatus counts a Drain's finalized local packs.
type DrainStatus struct {
	// Pending counts the packs not yet known uploaded.
	Pending int
	// PendingBytes is the size of the pending packs.
	PendingBytes int64
	// LocalBytes is the size of every finalized pack still local, uploaded
	// or not.
	LocalBytes int64
	// Oldest is when the oldest pending pack's first batch was written, zero
	// with none pending or for a pack loaded at Open.
	Oldest time.Time
	// Written counts the packs this Drain wrote to the remote.
	Written uint64
	// WrittenBytes is their size.
	WrittenBytes uint64
	// Listed counts the packs this Drain found the remote listed already,
	// such as one uploaded before a restart, and wrote nothing for.
	Listed uint64
	// Failures counts the Run calls that failed other than by their context.
	Failures uint64
}

// NewDrain builds a Drain of log.
func NewDrain(log *Log, opts DrainOptions) *Drain {
	return &Drain{log: log, opts: opts}
}

// Status reports the Log's finalized packs, and a channel that closes when
// the Log next changes: a pack finalizes, uploads or is evicted.
func (d *Drain) Status() (DrainStatus, <-chan struct{}) {
	// Read the aggregate and its watch without walking retained packs.
	l := d.log
	var changed <-chan struct{}
	var st DrainStatus
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		st.Pending, st.PendingBytes, st.LocalBytes = l.pending.Len(), l.pendingBytes, l.finalBytes
		if oldest := l.pending.Front(); oldest != nil {
			st.Oldest = oldest.Value.(*pack).firstAt
		}
		changed = getWaitCh()
	})

	// Upload metrics are recorded before the log marks a pack uploaded.
	st.Written, st.WrittenBytes = d.written.Load(), d.writtenBytes.Load()
	st.Listed, st.Failures = d.listed.Load(), d.failures.Load()
	return st, changed
}

// Run uploads finalized packs to remote, oldest first, and evicts uploaded
// packs beyond the budget, until ctx ends or a step fails.
func (d *Drain) Run(ctx context.Context, remote Remote) error {
	err := d.run(ctx, remote)
	if err != nil && ctx.Err() == nil {
		d.failures.Add(1)
	}
	return err
}

// run is Run without the failure count.
func (d *Drain) run(ctx context.Context, remote Remote) error {
	var last int64
	l := d.log
	for {
		// Select one upload or eviction, or watch for the next pack.
		var upload PackInfo
		var haveUpload bool
		var evict *pack
		var changed <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if oldest := l.pending.Front(); oldest != nil {
				upload, haveUpload = packInfo(oldest.Value.(*pack)), true
				return
			}
			if d.opts.KeepBytes > 0 && l.finalBytes > d.opts.KeepBytes {
				evict = l.finals.Front().Value.(*pack)
				return
			}
			changed = getWaitCh()
		})

		// Run disk and network work outside bcast.
		if haveUpload {
			sent, err := d.upload(ctx, remote, upload, last)
			if err != nil {
				return err
			}
			last = sent
			continue
		}
		if evict != nil {
			if err := l.Evict(ctx, evict.seq); err != nil {
				return err
			}
			continue
		}

		// A watch closes on the next change to the drained Log.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

// upload makes the remote hold pack p, sending it unless the remote lists it
// already, and marks it uploaded. It returns the bytes sent.
func (d *Drain) upload(ctx context.Context, remote Remote, p PackInfo, last int64) (int64, error) {
	// Skip a pack the remote lists, such as one uploaded before a restart.
	listed, err := remote.HasPackfile(ctx, p.ID)
	if err != nil {
		return 0, errors.Wrapf(err, "look up pack %s", p.ID)
	}
	var sent int64
	if !listed {
		if d.opts.Admit != nil {
			if err := d.opts.Admit(ctx, last); err != nil {
				return 0, err
			}
		}
		data, err := os.ReadFile(p.Path)
		if err != nil {
			return 0, errors.Wrapf(err, "read pack %s", p.ID)
		}
		if _, err := remote.PutPackFile(ctx, p.ID, data); err != nil {
			return 0, errors.Wrapf(err, "upload pack %s", p.ID)
		}
		sent = int64(len(data))
		d.written.Add(1)
		d.writtenBytes.Add(uint64(sent))
	} else {
		d.listed.Add(1)
	}
	d.log.MarkUploaded(p.Seq)
	return sent, nil
}

// UploadedBefore reports whether the remote holds every value the Log took
// before at: no pack not marked uploaded holds one.
func (d *Drain) UploadedBefore(at time.Time) bool {
	return !d.log.HoldsBefore(at)
}
