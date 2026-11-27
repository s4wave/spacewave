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

// Drain uploads a Log's finalized packs to one remote and evicts the uploaded
// ones beyond a budget. Build one per Log: the Log's upload marks identify the
// one remote it drains to.
//
// Invariant: a pack is evicted only after the Log marked it uploaded.
// The Drain marks the pack uploaded once the remote lists the pack or
// PutPackFile returns for the pack, so every evicted value is on the remote.
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

// NewDrain builds a Drain of log.
func NewDrain(log *Log, opts DrainOptions) *Drain {
	return &Drain{log: log, opts: opts}
}

// Status reports the Log's finalized packs, and a channel that closes when
// the Log next changes: a pack finalizes, uploads or is evicted.
func (d *Drain) Status() (DrainStatus, <-chan struct{}) {
	// Read the Log's finalized pack totals and their watch together.
	st, changed := d.log.watchDrainStatus()

	// Upload metrics are recorded before the log marks a pack uploaded.
	st.Written, st.WrittenBytes = d.written.Load(), d.writtenBytes.Load()
	st.Listed, st.Failures = d.listed.Load(), d.failures.Load()
	return st, changed
}

// Run uploads finalized packs to remote, oldest first, and evicts uploaded
// packs beyond the budget, until ctx ends or a step fails.
func (d *Drain) Run(ctx context.Context, remote Remote) error {
	// Pace remote uploads by the bytes written in the previous upload.
	var last int64
	upload := func(p PackInfo) error {
		var err error
		last, err = d.upload(ctx, remote, p, last)
		return err
	}

	// Let the Log advance one pack or wait until it has work.
	for {
		if err := d.log.drainStep(ctx, d.opts.KeepBytes, upload); err != nil {
			if ctx.Err() == nil {
				d.failures.Add(1)
			}
			return err
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
// before at: every pack holding such a value is marked uploaded.
func (d *Drain) UploadedBefore(at time.Time) bool {
	return !d.log.HoldsBefore(at)
}
