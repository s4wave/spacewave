package rangemerge

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// Defaults of Config.
const (
	// DefaultWindow is how long the first read of an object waits for others.
	// It is small against the hundreds of milliseconds a server read takes.
	DefaultWindow = 2 * time.Millisecond
	// DefaultMaxGap is the largest gap between two ranges read as one.
	DefaultMaxGap = 64 << 10
	// DefaultMaxSpan is the most bytes one merged read covers.
	DefaultMaxSpan = 4 << 20
	// DefaultMaxReads is the most reads one group holds before it is read.
	DefaultMaxReads = 64
)

// Config configures a Store. Zero fields take the defaults.
type Config struct {
	// Window is how long the first read of an object waits for others.
	Window time.Duration
	// MaxGap is the largest gap, in bytes, between ranges read as one.
	MaxGap int
	// MaxSpan is the most bytes one merged read covers.
	MaxSpan int
	// MaxReads is the most reads a group holds before it is read at once.
	MaxReads int
}

// Stats counts a Store's range reads.
type Stats struct {
	// Reads counts the range reads asked of the Store.
	Reads uint64
	// Spans counts the range reads the Store asked of the object store.
	Spans uint64
}

// Store is an ObjectStore that merges range reads of one object asked within
// a short window into reads of the spans they cover. On a server that answers
// a session's requests one at a time, as OpenSSH's sftp-server does, each
// merged read saves a request and a random disk read. Every other method is
// the wrapped store's.
type Store struct {
	block_store_s3.ObjectStore
	conf Config

	mu     sync.Mutex
	groups map[string]*group

	reads, spans atomic.Uint64
}

// group holds the reads of one object waiting for its window to end.
type group struct {
	bucket, key string
	reads       []*read
	// full is closed when the group reaches MaxReads.
	full chan struct{}
}

// read is one waiting range read.
type read struct {
	off  int64
	n    int
	data []byte
	err  error
	done chan struct{}
}

// New wraps inner.
func New(inner block_store_s3.ObjectStore, conf Config) *Store {
	// Default the zero fields.
	if conf.Window <= 0 {
		conf.Window = DefaultWindow
	}
	if conf.MaxGap <= 0 {
		conf.MaxGap = DefaultMaxGap
	}
	if conf.MaxSpan <= 0 {
		conf.MaxSpan = DefaultMaxSpan
	}
	if conf.MaxReads <= 0 {
		conf.MaxReads = DefaultMaxReads
	}
	return &Store{ObjectStore: inner, conf: conf, groups: map[string]*group{}}
}

// Stats returns the read counts.
func (s *Store) Stats() Stats {
	return Stats{Reads: s.reads.Load(), Spans: s.spans.Load()}
}

// GetObjectRange reads length bytes at off, merged with the other reads of
// the object asked within the window. The result is shorter when the object
// ends first. A reader whose ctx ends returns its cause; the merged read goes
// on for the others.
func (s *Store) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Count the read and name its object.
	s.reads.Add(1)
	r := &read{off: off, n: length, done: make(chan struct{})}
	name := bucket + "\x00" + key

	// Join the object's group, starting it when there is none.
	s.mu.Lock()
	g := s.groups[name]
	if g == nil {
		g = &group{bucket: bucket, key: key, full: make(chan struct{})}
		s.groups[name] = g
		go s.flush(name, g)
	}
	g.reads = append(g.reads, r)
	if len(g.reads) == s.conf.MaxReads {
		close(g.full)
	}
	s.mu.Unlock()

	// Wait for the read or the caller.
	select {
	case <-r.done:
		return r.data, r.err
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

// flush waits out the window or a full group, takes the group, and reads its
// spans concurrently. The reads run without the callers' contexts, so one
// caller leaving fails no other.
func (s *Store) flush(name string, g *group) {
	// Close the group to new reads.
	t := time.NewTimer(s.conf.Window)
	select {
	case <-t.C:
	case <-g.full:
		t.Stop()
	}
	s.mu.Lock()
	delete(s.groups, name)
	reads := g.reads
	s.mu.Unlock()

	// Read each span and hand its readers their bytes.
	var wg sync.WaitGroup
	for _, sp := range spans(reads, s.conf.MaxGap, s.conf.MaxSpan) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.spans.Add(1)
			data, err := s.ObjectStore.GetObjectRange(context.Background(), g.bucket, g.key, sp.off, sp.n)
			for _, r := range sp.reads {
				r.data, r.err = cut(data, err, r.off-sp.off, r.n)
				close(r.done)
			}
		}()
	}
	wg.Wait()
}

// merged is one span read for its reads.
type merged struct {
	off   int64
	n     int
	reads []*read
}

// spans sorts reads by offset and merges each into the span before it when
// the gap is at most maxGap and the span stays within maxSpan.
func spans(reads []*read, maxGap, maxSpan int) []merged {
	// Sort the reads.
	sorted := slices.Clone(reads)
	slices.SortFunc(sorted, func(a, b *read) int {
		switch {
		case a.off < b.off:
			return -1
		case a.off > b.off:
			return 1
		}
		return 0
	})

	// Merge each read into the last span, or start a span.
	var out []merged
	for _, r := range sorted {
		end := r.off + int64(r.n)
		if n := len(out); n != 0 {
			last := &out[n-1]
			lastEnd := last.off + int64(last.n)
			newEnd := max(lastEnd, end)
			if r.off <= lastEnd+int64(maxGap) && newEnd-last.off <= int64(maxSpan) {
				last.n = int(newEnd - last.off)
				last.reads = append(last.reads, r)
				continue
			}
		}
		out = append(out, merged{off: r.off, n: r.n, reads: []*read{r}})
	}
	return out
}

// cut returns the n bytes at rel in a span's data, shorter or nil where the
// data ends, or the span's error.
func cut(data []byte, err error, rel int64, n int) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	if rel >= int64(len(data)) {
		return nil, nil
	}
	end := min(int64(len(data)), rel+int64(n))
	return slices.Clone(data[rel:end]), nil
}
