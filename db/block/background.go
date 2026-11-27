package block

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// ReadClass ranks a read by who waits for it. A store with a bounded link to
// remote data serves the classes in this order.
type ReadClass uint8

const (
	// ReadUrgent is a read made while holding something every write waits
	// for, such as a World write body under the writer lock. Each such read
	// delays every queued write, so it goes before every other class.
	ReadUrgent ReadClass = iota
	// ReadInteractive is a read a person or a write waits for, such as a
	// request's own reads. It is the class of an untagged context.
	ReadInteractive
	// ReadRefill is a read that brings a view already shown up to date. Its
	// viewer sees the older state meanwhile, so it yields to interactive reads
	// but not to background work.
	ReadRefill
	// ReadBackground is work no person is waiting for, such as a fill.
	ReadBackground
)

// readClassKey identifies the read class of a context.
type readClassKey struct{}

// readDemand is the read class of a context and what raises it.
type readDemand struct {
	// class is the class the read starts in.
	class ReadClass
	// raised closes when an interactive read starts waiting on this one.
	raised <-chan struct{}
	// demand, when set, holds the class instead of class and raised.
	demand *Demand
}

// Demand is the read class of a fetch that several readers share: the most
// urgent class of the readers that joined it. It rises as a more urgent
// reader joins and never falls, so a fetch that once served an urgent reader
// stays urgent until it ends.
type Demand struct {
	// bcast guards class and broadcasts each rise.
	bcast broadcast.Broadcast
	// class is the most urgent class of the readers that joined.
	class ReadClass
}

// NewDemand returns a demand that starts at class.
func NewDemand(class ReadClass) *Demand {
	return &Demand{class: class}
}

// Raise raises the demand to class when class is more urgent.
func (d *Demand) Raise(class ReadClass) {
	d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if class < d.class {
			d.class = class
			broadcast()
		}
	})
}

// State returns the demand's class and a channel closed when it next rises,
// nil once it is urgent and cannot rise.
func (d *Demand) State() (ReadClass, <-chan struct{}) {
	var class ReadClass
	var ch <-chan struct{}
	d.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		class = d.class
		if class != ReadUrgent {
			ch = getWaitCh()
		}
	})
	return class, ch
}

// WithDemand returns ctx whose reads take the current class of d.
func WithDemand(ctx context.Context, d *Demand) context.Context {
	return context.WithValue(ctx, readClassKey{}, readDemand{demand: d})
}

// WithReadClass returns ctx whose reads are of class.
func WithReadClass(ctx context.Context, class ReadClass) context.Context {
	return context.WithValue(ctx, readClassKey{}, readDemand{class: class})
}

// WithRaise returns ctx whose reads keep their class until raised closes and
// count as interactive after, as when a fetch shared by several readers gains
// one that a person waits for.
func WithRaise(ctx context.Context, raised <-chan struct{}) context.Context {
	return context.WithValue(ctx, readClassKey{}, readDemand{class: ReadClassOf(ctx), raised: raised})
}

// ReadClassOf returns the class of the reads of ctx, ReadInteractive when
// ctx carries none.
func ReadClassOf(ctx context.Context) ReadClass {
	demand, ok := ctx.Value(readClassKey{}).(readDemand)
	switch {
	case !ok:
		return ReadInteractive
	case demand.demand != nil:
		class, _ := demand.demand.State()
		return class
	case demand.class > ReadInteractive && isClosed(demand.raised):
		return ReadInteractive
	}
	return demand.class
}

// isClosed reports whether ch is closed. A nil channel is not.
func isClosed(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Foreground reports whether someone waits on a read of class c now: an
// urgent or interactive read, as opposed to a refill or background read.
func (c ReadClass) Foreground() bool {
	return c <= ReadInteractive
}

// Raised returns a channel closed when the class of the reads of ctx next
// rises, after which ReadClassOf returns the new class, or nil when nothing
// can raise it further.
func Raised(ctx context.Context) <-chan struct{} {
	demand, _ := ctx.Value(readClassKey{}).(readDemand)
	if demand.demand != nil {
		_, ch := demand.demand.State()
		return ch
	}
	if demand.class <= ReadInteractive || isClosed(demand.raised) {
		return nil
	}
	return demand.raised
}

// WithBackground marks reads and writes as background work.
// Use it for fills and bookkeeping whose result a later pass can redo.
// Keep each writer hold bounded: the marker cannot bound caller bodies or I/O.
//
// Native s4db admits a finite cohort of queued foreground writers, then one
// shared background slice, including marked optimistic commits and internal
// slices. A foreground head encounters at most one background slice total,
// including a slice already running. Background progresses under continuing
// foreground arrivals. This scheduling gives no finite wall-clock bound on
// caller bodies, storage, scheduling or existing memtable backpressure.
// Bolt and SQLite instead serve every queued foreground writer first, so
// their background writes may wait indefinitely under steady foreground load.
// Stores without background scheduling ignore the mark.
//
// The marker also selects ReadBackground in the existing read-demand API.
// A remote fetch yields while foreground fetches need the store's bounded link.
// Reads served from local or resident data do not wait for that link.
func WithBackground(ctx context.Context) context.Context {
	return WithReadClass(ctx, ReadBackground)
}

// WithPutOpts returns ctx marked as background work when opts asks for it, so
// a put handed in from another process keeps the priority its writer gave it.
func WithPutOpts(ctx context.Context, opts *PutOpts) context.Context {
	if opts.GetBackground() {
		return WithBackground(ctx)
	}
	return ctx
}

// IsBackground reports whether ctx carries the background read/write marker.
// Other read classes retain foreground write admission.
func IsBackground(ctx context.Context) bool {
	return ReadClassOf(ctx) == ReadBackground
}
