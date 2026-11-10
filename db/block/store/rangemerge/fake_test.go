package rangemerge

import (
	"container/list"
	"context"
	"sync"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// costs is the server time of each SFTP request in the fake, on a virtual
// clock. OpenSSH's sftp-server answers one request of a session at a time,
// so a session's clock is the sum of the costs of its requests.
type costs struct {
	// open is the time to open a file the session holds no handle for.
	open time.Duration
	// close is the time to close an evicted handle.
	close time.Duration
	// seek is the time of the first packet of a read: a random disk read.
	seek time.Duration
	// packet is the time of each further 32 KiB packet of the same read.
	packet time.Duration
}

// packetBytes is the most a READ request returns, as pkg/sftp asks.
const packetBytes = 32 << 10

// fakeSFTP is an ObjectStore served by a few fake SFTP sessions. Each session
// answers one request at a time and keeps up to handles files open, as the
// real client keeps them.
type fakeSFTP struct {
	block_store_s3.ObjectStore
	objects  map[string][]byte
	cost     costs
	handles  int
	sessions []*fakeSession

	mu   sync.Mutex
	next int
	// fail makes every read fail with the error.
	fail error
}

// fakeSession is one SFTP session with its virtual clock and open handles.
type fakeSession struct {
	mu    sync.Mutex
	clock time.Duration
	reqs  int
	reads int
	lru   *list.List
	open  map[string]*list.Element
}

// newFakeSFTP builds a fake with n sessions over objects.
func newFakeSFTP(objects map[string][]byte, n, handles int, c costs) *fakeSFTP {
	f := &fakeSFTP{objects: objects, cost: c, handles: handles}
	for range n {
		f.sessions = append(f.sessions, &fakeSession{lru: list.New(), open: map[string]*list.Element{}})
	}
	return f
}

// pick returns the session holding name open, else the next in turn, as the
// real client prefers the connection with the file open.
func (f *fakeSFTP) pick(name string) *fakeSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		s.mu.Lock()
		_, ok := s.open[name]
		s.mu.Unlock()
		if ok {
			return s
		}
	}
	s := f.sessions[f.next%len(f.sessions)]
	f.next++
	return s
}

// GetObjectRange charges the session the open, the read packets and any
// close, one request at a time, and returns the bytes.
func (f *fakeSFTP) GetObjectRange(_ context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	// Fail when told to.
	f.mu.Lock()
	fail := f.fail
	f.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	name := bucket + "/" + key
	obj, ok := f.objects[name]
	if !ok {
		return nil, block_store_s3.ErrNotFound
	}

	// Charge the requests on one session, serially.
	s := f.pick(name)
	s.mu.Lock()
	if el, ok := s.open[name]; ok {
		s.lru.MoveToFront(el)
	} else {
		s.clock += f.cost.open
		s.reqs++
		s.open[name] = s.lru.PushFront(name)
		if s.lru.Len() > f.handles {
			old := s.lru.Back()
			s.lru.Remove(old)
			delete(s.open, old.Value.(string))
			s.clock += f.cost.close
			s.reqs++
		}
	}
	packets := max(1, (length+packetBytes-1)/packetBytes)
	s.clock += f.cost.seek + time.Duration(packets-1)*f.cost.packet
	s.reqs += packets
	s.reads++
	s.mu.Unlock()

	// Return the bytes, short at the end of the object.
	if off >= int64(len(obj)) {
		return nil, nil
	}
	end := min(int64(len(obj)), off+int64(length))
	return append([]byte(nil), obj[off:end]...), nil
}

// totals returns the summed virtual server time, requests and reads.
func (f *fakeSFTP) totals() (time.Duration, int, int) {
	var clock time.Duration
	var reqs, reads int
	for _, s := range f.sessions {
		s.mu.Lock()
		clock += s.clock
		reqs += s.reqs
		reads += s.reads
		s.mu.Unlock()
	}
	return clock, reqs, reads
}
