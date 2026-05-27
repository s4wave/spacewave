// Package cacheseedbuffer owns bounded recording of seed-reason-tagged HTTP
// requests.
package cacheseedbuffer

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/provider/spacewave/seedreason"
)

// DefaultCapacity is the default ring buffer size for the cache-seed
// inspector. Sized to comfortably cover a cold-mount cascade without growing
// unboundedly in long-running sessions.
const DefaultCapacity = 1024

// Entry is a single recorded HTTP request tagged with a seed reason.
type Entry struct {
	// TimestampMs is the unix timestamp in milliseconds when the request was
	// recorded (at dispatch time, not completion).
	TimestampMs int64
	// Reason is the seed-reason header value; empty if the request was not
	// tagged.
	Reason seedreason.Reason
	// Path is the URL path the request was sent to.
	Path string
}

// Buffer is a goroutine-safe bounded ring buffer recording every tagged HTTP
// call the provider issues. Subscribers receive a snapshot of the current
// buffer plus any future appends until they stop reading.
type Buffer struct {
	bcast   broadcast.Broadcast
	cap     int
	entries []record
	nextSeq uint64
}

type record struct {
	seq   uint64
	entry Entry
}

// New constructs a new Buffer with the given capacity. A capacity of zero or
// less falls back to DefaultCapacity.
func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Buffer{
		cap:     capacity,
		entries: make([]record, 0, capacity),
	}
}

// Record appends an entry to the buffer, evicting the oldest entry when the
// buffer is full. Safe to call concurrently from any goroutine.
func (b *Buffer) Record(reason seedreason.Reason, path string) {
	entry := Entry{
		TimestampMs: time.Now().UnixMilli(),
		Reason:      reason,
		Path:        path,
	}
	b.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		b.nextSeq++
		record := record{seq: b.nextSeq, entry: entry}
		if len(b.entries) < b.cap {
			b.entries = append(b.entries, record)
			broadcast()
			return
		}
		copy(b.entries, b.entries[1:])
		b.entries[len(b.entries)-1] = record
		broadcast()
	})
}

// Snapshot returns a copy of the current buffer contents in insertion order
// (oldest first).
func (b *Buffer) Snapshot() []Entry {
	var out []Entry
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		out = make([]Entry, 0, len(b.entries))
		for _, record := range b.entries {
			out = append(out, record.entry)
		}
	})
	return out
}

// Capacity returns the configured buffer capacity.
func (b *Buffer) Capacity() int {
	return b.cap
}

// Subscribe returns a snapshot of the current buffer plus a channel that
// receives future appends. The caller must invoke the returned release function
// to remove its subscription and close the channel. The channel has a small
// buffer; if a slow consumer falls behind, newer entries are dropped rather
// than blocking the producer.
func (b *Buffer) Subscribe() (snapshot []Entry, updates <-chan Entry, release func()) {
	ch := make(chan Entry, b.cap)
	var seq uint64
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snapshot = make([]Entry, 0, len(b.entries))
		for _, record := range b.entries {
			snapshot = append(snapshot, record.entry)
		}
		seq = b.nextSeq
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(ch)
		b.watchUpdates(ctx, seq, ch)
	}()

	var once sync.Once
	return snapshot, ch, func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (b *Buffer) watchUpdates(ctx context.Context, seq uint64, ch chan<- Entry) {
	for {
		var records []record
		if err := b.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			for _, record := range b.entries {
				if record.seq > seq {
					records = append(records, record)
				}
			}
			return len(records) != 0, nil
		}); err != nil {
			return
		}

		for _, record := range records {
			seq = record.seq
			select {
			case ch <- record.entry:
			default:
			}
		}
	}
}

// NewRecordingTransport wraps base (nil uses http.DefaultTransport) so that
// every request tagged with seedreason.Header is recorded to buf before being
// forwarded.
func NewRecordingTransport(base http.RoundTripper, buf *Buffer) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base, buf: buf}
}

type transport struct {
	base http.RoundTripper
	buf  *Buffer
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.buf != nil {
		reason := seedreason.Reason(req.Header.Get(seedreason.Header))
		path := ""
		if req.URL != nil {
			path = req.URL.Path
		}
		t.buf.Record(reason, path)
	}
	return t.base.RoundTrip(req)
}
