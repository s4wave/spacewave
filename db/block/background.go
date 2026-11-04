package block

import "context"

// ReadClass ranks a read by who waits for it. A store with a bounded link to
// remote data serves the classes in this order.
type ReadClass uint8

const (
	// ReadInteractive is a read a person or a write waits for, such as a
	// request's own reads. It is the class of an untagged context.
	ReadInteractive ReadClass = iota
	// ReadRefill is a read that brings a view already shown up to date. Its
	// viewer sees the older state meanwhile, so it yields to interactive reads
	// but not to background work.
	ReadRefill
	// ReadBackground is work no person is waiting for, such as a fill.
	ReadBackground
)

// readClassKey identifies the read class of a context.
type readClassKey struct{}

// readDemand is the read class of a context and the channel that raises it.
type readDemand struct {
	// class is the class the read starts in.
	class ReadClass
	// raised closes when an interactive read starts waiting on this one.
	raised <-chan struct{}
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

// ReadClassOf returns the class of the reads of ctx.
func ReadClassOf(ctx context.Context) ReadClass {
	demand, _ := ctx.Value(readClassKey{}).(readDemand)
	return demand.class
}

// Raised returns the channel that raises the reads of ctx to interactive, or
// nil when nothing can raise them.
func Raised(ctx context.Context) <-chan struct{} {
	demand, _ := ctx.Value(readClassKey{}).(readDemand)
	return demand.raised
}

// WithBackground marks ctx as background work, such as a fill no person is
// waiting for. A store that fetches remote data admits a background fetch only
// while no foreground fetch on that store needs the link. Reads served from
// local or resident data do not wait.
func WithBackground(ctx context.Context) context.Context {
	return WithReadClass(ctx, ReadBackground)
}

// IsBackground reports whether ctx is marked as background work.
func IsBackground(ctx context.Context) bool {
	return ReadClassOf(ctx) == ReadBackground
}
