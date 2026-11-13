package volume

import (
	"errors"
	"fmt"
)

// Opening owns what a Volume open has taken, such as a device and its lock or
// a database file, from the first resource until the Volume takes them over.
//
// The opener defers Settle with its error result, holds each release as it
// takes a resource, and calls Handoff once the Volume owns them, so no exit of
// the open, whether an error, a cancelled context or a panic, leaks them:
//
//	var op volume.Opening
//	defer op.Settle(&err)
//	op.Hold(dev.Close)
//	...
//	op.Handoff()
//
// Invariants:
//   - One opener holds the resources at a time: the open until Handoff, then
//     the Volume.
//   - The release runs exactly once: by Settle on a failed open, or by the
//     Volume's Close after Handoff, never both.
//   - Settle adds no write of its own. The release closes what the open holds
//     through its own close, which finishes only work already committed, and
//     never deletes, resets or repairs stored data.
type Opening struct {
	// release releases everything the open holds, or is nil once handed off.
	release func() error
}

// Hold sets the release of everything the open holds so far. A later Hold
// replaces the earlier release, which the new one must include.
func (o *Opening) Hold(release func() error) {
	o.release = release
}

// Handoff passes what the open holds to the Volume, which releases it on Close.
func (o *Opening) Handoff() {
	o.release = nil
}

// Settle releases what a failed open still holds. Defer it directly, with the
// open's error result. A returned error is joined with the release's error. A
// panic is recovered only to release first, then raised again as an error
// carrying its text, so the panic still reaches the caller's recovery and its
// message survives one that keeps only errors.
func (o *Opening) Settle(errp *error) {
	// Release only what an unfinished open still holds.
	release := o.release
	if release == nil {
		return
	}
	o.release = nil

	// Release first, then raise a panic again or join the release's error.
	r := recover()
	rerr := release()
	if r != nil {
		panic(errors.Join(panicError(r), rerr))
	}
	*errp = errors.Join(*errp, rerr)
}

// panicError returns the panic value r as an error with its text.
func panicError(r any) error {
	if err, ok := r.(error); ok {
		return fmt.Errorf("volume open panicked: %w", err)
	}
	return fmt.Errorf("volume open panicked: %v", r)
}
