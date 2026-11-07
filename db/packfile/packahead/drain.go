package packahead

import (
	"context"
	"os"
	"slices"
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
}

// NewDrain builds a Drain of log.
func NewDrain(log *Log, opts DrainOptions) *Drain {
	return &Drain{log: log, opts: opts}
}

// Status reports the Log's finalized packs, and a channel that closes when
// the Log next changes: a pack finalizes, uploads or is evicted.
func (d *Drain) Status() (DrainStatus, <-chan struct{}) {
	packs, changed := d.log.GetPacks()
	var st DrainStatus
	for _, p := range packs {
		st.LocalBytes += p.Size
		if p.Uploaded {
			continue
		}
		if st.Pending == 0 {
			st.Oldest = p.FirstAt
		}
		st.Pending++
		st.PendingBytes += p.Size
	}
	return st, changed
}

// Run uploads finalized packs to remote, oldest first, and evicts uploaded
// packs beyond the budget, until ctx ends or a step fails.
func (d *Drain) Run(ctx context.Context, remote Remote) error {
	var last int64
	for {
		// Upload the oldest pack not yet uploaded, or wait for the next.
		packs, changed := d.log.GetPacks()
		i := slices.IndexFunc(packs, func(p PackInfo) bool { return !p.Uploaded })
		if i < 0 {
			if err := d.evict(ctx, packs); err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-changed:
			}
			continue
		}
		sent, err := d.upload(ctx, remote, packs[i], last)
		if err != nil {
			return err
		}
		last = sent
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
	}
	d.log.MarkUploaded(p.Seq)
	return sent, nil
}

// evict unlinks the oldest uploaded packs while the finalized packs exceed
// the budget. It stops at the first pack not yet uploaded, so the packs kept
// are the newest.
func (d *Drain) evict(ctx context.Context, packs []PackInfo) error {
	// A zero budget keeps every pack.
	if d.opts.KeepBytes <= 0 {
		return nil
	}

	// Evict oldest first until the rest fit the budget.
	var total int64
	for _, p := range packs {
		total += p.Size
	}
	for _, p := range packs {
		if total <= d.opts.KeepBytes || !p.Uploaded {
			return nil
		}
		if err := d.log.Evict(ctx, p.Seq); err != nil {
			return err
		}
		total -= p.Size
	}
	return nil
}

// UploadedBefore reports whether the remote holds every value the Log took
// before at: no pack not marked uploaded holds one.
func (d *Drain) UploadedBefore(at time.Time) bool {
	return !d.log.HoldsBefore(at)
}
