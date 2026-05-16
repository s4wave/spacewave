package provider_spacewave

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
)

// DefaultCacheSeedBufferCapacity is the default ring buffer size for the
// cache-seed inspector. Sized to comfortably cover a cold-mount cascade
// without growing unboundedly in long-running sessions.
const DefaultCacheSeedBufferCapacity = 1024

// CacheSeedEntry is a single recorded HTTP request tagged with a seed reason.
type CacheSeedEntry struct {
	// TimestampMs is the unix timestamp in milliseconds when the request was
	// recorded (at dispatch time, not completion).
	TimestampMs int64
	// Reason is the SeedReason header value; empty if the request was not
	// tagged.
	Reason SeedReason
	// Path is the URL path the request was sent to.
	Path string
}

// CacheSeedBuffer is a goroutine-safe bounded ring buffer recording every
// tagged HTTP call the provider issues. Subscribers receive a snapshot of the
// current buffer plus any future appends until they stop reading.
type CacheSeedBuffer struct {
	bcast   broadcast.Broadcast
	cap     int
	entries []cacheSeedRecord
	nextSeq uint64
}

type cacheSeedRecord struct {
	seq   uint64
	entry CacheSeedEntry
}

// NewCacheSeedBuffer constructs a new CacheSeedBuffer with the given capacity.
// A capacity of zero or less falls back to DefaultCacheSeedBufferCapacity.
func NewCacheSeedBuffer(capacity int) *CacheSeedBuffer {
	if capacity <= 0 {
		capacity = DefaultCacheSeedBufferCapacity
	}
	return &CacheSeedBuffer{
		cap:     capacity,
		entries: make([]cacheSeedRecord, 0, capacity),
	}
}

// Record appends an entry to the buffer, evicting the oldest entry when the
// buffer is full. Safe to call concurrently from any goroutine.
func (b *CacheSeedBuffer) Record(reason SeedReason, path string) {
	entry := CacheSeedEntry{
		TimestampMs: time.Now().UnixMilli(),
		Reason:      reason,
		Path:        path,
	}
	b.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		b.nextSeq++
		record := cacheSeedRecord{seq: b.nextSeq, entry: entry}
		if len(b.entries) < b.cap {
			b.entries = append(b.entries, record)
		} else {
			copy(b.entries, b.entries[1:])
			b.entries[len(b.entries)-1] = record
		}
		broadcast()
	})
}

// Snapshot returns a copy of the current buffer contents in insertion order
// (oldest first).
func (b *CacheSeedBuffer) Snapshot() []CacheSeedEntry {
	var out []CacheSeedEntry
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		out = make([]CacheSeedEntry, 0, len(b.entries))
		for _, record := range b.entries {
			out = append(out, record.entry)
		}
	})
	return out
}

// Capacity returns the configured buffer capacity.
func (b *CacheSeedBuffer) Capacity() int {
	return b.cap
}

// Subscribe returns a snapshot of the current buffer plus a channel that
// receives future appends. The caller must invoke the returned release
// function to remove its subscription and close the channel. The channel has
// a small buffer; if a slow consumer falls behind, newer entries are dropped
// rather than blocking the producer.
func (b *CacheSeedBuffer) Subscribe() (snapshot []CacheSeedEntry, updates <-chan CacheSeedEntry, release func()) {
	ch := make(chan CacheSeedEntry, b.cap)
	var seq uint64
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snapshot = make([]CacheSeedEntry, 0, len(b.entries))
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
		b.watchCacheSeedUpdates(ctx, seq, ch)
	}()

	var once sync.Once
	return snapshot, ch, func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (b *CacheSeedBuffer) watchCacheSeedUpdates(ctx context.Context, seq uint64, ch chan<- CacheSeedEntry) {
	for {
		var records []cacheSeedRecord
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

// NewCacheSeedRecordingTransport wraps base (nil uses http.DefaultTransport)
// so that every request tagged with SeedReasonHeader is recorded to buf
// before being forwarded.
func NewCacheSeedRecordingTransport(base http.RoundTripper, buf *CacheSeedBuffer) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &cacheSeedTransport{base: base, buf: buf}
}

type cacheSeedTransport struct {
	base http.RoundTripper
	buf  *CacheSeedBuffer
}

func (t *cacheSeedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.buf != nil {
		reason := SeedReason(req.Header.Get(SeedReasonHeader))
		path := ""
		if req.URL != nil {
			path = req.URL.Path
		}
		t.buf.Record(reason, path)
	}
	return t.base.RoundTrip(req)
}
