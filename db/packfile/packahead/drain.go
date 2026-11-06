package packahead

import (
	"context"
	"os"
	"sync"
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
// ones beyond a budget. Build one per remote: what it learned was uploaded
// holds only for that remote.
//
// Invariant: a pack is evicted only after this Drain saw the remote list it,
// or saw PutPackFile return for it, so every evicted value is on the remote.
type Drain struct {
	// log is the drained Log.
	log *Log
	// opts are the options.
	opts DrainOptions
	// mtx guards uploaded.
	mtx sync.Mutex
	// uploaded holds the seqs of the finalized packs the remote holds.
	uploaded map[uint64]struct{}
}

// NewDrain builds a Drain of log.
func NewDrain(log *Log, opts DrainOptions) *Drain {
	return &Drain{log: log, opts: opts, uploaded: make(map[uint64]struct{})}
}

// Run uploads finalized packs to remote, oldest first, and evicts uploaded
// packs beyond the budget, until ctx ends or a step fails.
func (d *Drain) Run(ctx context.Context, remote Remote) error {
	var last int64
	for {
		// Upload the oldest pack not yet uploaded, or wait for the next.
		packs, changed := d.log.GetPacks()
		next, ok := d.nextUpload(packs)
		if !ok {
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
		sent, err := d.upload(ctx, remote, next, last)
		if err != nil {
			return err
		}
		last = sent
	}
}

// nextUpload returns the oldest pack not known uploaded.
func (d *Drain) nextUpload(packs []PackInfo) (PackInfo, bool) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	for _, p := range packs {
		if _, ok := d.uploaded[p.Seq]; !ok {
			return p, true
		}
	}
	return PackInfo{}, false
}

// upload makes the remote hold pack p, sending it unless the remote lists it
// already, and records it uploaded. It returns the bytes sent.
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
	d.mtx.Lock()
	d.uploaded[p.Seq] = struct{}{}
	d.mtx.Unlock()
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
		if total <= d.opts.KeepBytes {
			return nil
		}
		d.mtx.Lock()
		_, uploaded := d.uploaded[p.Seq]
		d.mtx.Unlock()
		if !uploaded {
			return nil
		}
		if err := d.log.Evict(ctx, p.Seq); err != nil {
			return err
		}
		d.mtx.Lock()
		delete(d.uploaded, p.Seq)
		d.mtx.Unlock()
		total -= p.Size
	}
	return nil
}

// UploadedBefore reports whether the remote holds every value the Log took
// before at: no pack this Drain has not seen uploaded holds one.
func (d *Drain) UploadedBefore(at time.Time) bool {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return !d.log.HoldsBefore(at, func(seq uint64, final bool) bool {
		_, ok := d.uploaded[seq]
		return final && ok
	})
}
