//go:build !js && !wasip1 && !windows

package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Ref states the ledger records.
const (
	stateDone   = "done"
	stateFailed = "failed"
	stateAbsent = "absent"
	// stateNotInPack is a ref the index placed in a pack that lacks it.
	stateNotInPack = "not-in-pack"
	// stateTooLarge is a ref in a pack larger than --max-pack-mib, which is
	// not fetched.
	stateTooLarge = "too-large"
	// stateAmbiguous is a ref whose pack number several packs share and
	// none of whose bloom filters may hold it.
	stateAmbiguous = "ambiguous-num"
)

// ledger is the append-only record of a rescue, which a restart reads to
// skip finished work. Its lines are:
//
//	r <hex block key> <state> [reason]   a ref finished as done, failed, absent
//	                                     or not-in-pack
//	p <pack id> <fetched> <found> <not-in-pack>
//	                                     every wanted ref of the pack finished,
//	                                     with the bytes read for it, the wanted
//	                                     refs stored and those it lacked, then
//	                                     too-large or no-size for a pack
//	                                     skipped unfetched
//	b <bytes>                            the bytes fetched across runs so far
//	rb <bytes>                           bytes reserved ahead of the requests
//	                                     in flight, written and synced before
//	                                     a request crosses the last one
//	t <pack id> <ms> <requests>          the time from the start of a
//	                                     fetched pack's first request to the
//	                                     end of its last, and its requests,
//	                                     retries included; a reader ignores it
//
// A later line for a ref replaces an earlier one. A b line sets the byte
// total and an rb line raises it: after a crash the total counts every
// request that started, and at most reserveAhead bytes more. A partial last line, left
// by a crash, is ignored.
type ledger struct {
	// mtx guards the appends and the maps against concurrent writers.
	mtx sync.Mutex
	f   *os.File
	w   *bufio.Writer
	// refs holds the state of each ref by block key.
	refs map[string]string
	// packs holds the finished packs.
	packs map[string]bool
	// bytes is the last byte total recorded.
	bytes uint64
	// durable is the byte total the ledger on disk is sure to yield at
	// least, and seen the largest total a request reserved.
	durable, seen uint64
}

// reserveAhead is how far past a request's bytes a reservation reaches, so
// one sync covers many requests.
const reserveAhead = 64 << 20

// openLedger opens or creates the ledger at path and reads what it holds.
func openLedger(path string) (*ledger, error) {
	// Open the file for appending.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	// Load what it holds, then buffer the appends.
	l := &ledger{f: f, refs: make(map[string]string), packs: make(map[string]bool)}
	if err := l.read(); err != nil {
		_ = f.Close()
		return nil, errors.Wrap(err, path)
	}
	l.w = bufio.NewWriterSize(f, 1<<20)
	return l, nil
}

// read loads the ledger's lines and ends a partial last line, so the next
// append starts a line of its own.
func (l *ledger) read() error {
	if _, err := l.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	r := bufio.NewReaderSize(l.f, 1<<20)
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			if line != "" {
				// A crash cut the last line: end it and ignore it.
				if _, werr := l.f.WriteString("\n"); werr != nil {
					return werr
				}
			}
			return nil
		}
		if err != nil {
			return err
		}
		l.apply(strings.Fields(line))
	}
}

// apply records one complete line.
func (l *ledger) apply(fields []string) {
	switch {
	case len(fields) >= 3 && fields[0] == "r":
		key, err := hex.DecodeString(fields[1])
		if err == nil {
			l.refs[string(key)] = fields[2]
		}
	case len(fields) >= 2 && fields[0] == "p":
		l.packs[fields[1]] = true
	case len(fields) == 2 && fields[0] == "b":
		if n, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
			l.bytes = n
		}
	case len(fields) == 2 && fields[0] == "rb":
		if n, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
			l.bytes = max(l.bytes, n)
		}
	}
}

// ref appends the state of a ref, with a reason of one word for a failed or
// absent one.
func (l *ledger) ref(key []byte, state, reason string) error {
	// Format the line, its reason made one word.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.refs[string(key)] = state
	line := "r " + hex.EncodeToString(key) + " " + state
	if reason != "" {
		line += " " + strings.Join(strings.Fields(reason), "_")
	}

	// Append it to the buffer.
	_, err := l.w.WriteString(line + "\n")
	return err
}

// pack appends that a pack is finished, with the bytes read for it, the
// wanted refs found and those not in it, and why it was skipped unfetched.
func (l *ledger) pack(id string, fetched uint64, found, notInPack int, skip string) error {
	// Format the line, with the skip reason when there is one.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.packs[id] = true
	line := fmt.Sprintf("p %s %d %d %d", id, fetched, found, notInPack)
	if skip != "" {
		line += " " + skip
	}

	// Append it to the buffer.
	_, err := l.w.WriteString(line + "\n")
	return err
}

// timing appends the time and the request count of a fetched pack.
func (l *ledger) timing(id string, elapsed time.Duration, requests int) error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	_, err := fmt.Fprintf(l.w, "t %s %d %d\n", id, elapsed.Milliseconds(), requests)
	return err
}

// total appends the byte total n, raised to the largest total a request
// reserved: a request that reserved after n was read is counted too.
func (l *ledger) total(n uint64) error {
	// Raise n to the reserved total and append it.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	n = max(n, l.seen)
	l.bytes, l.durable = n, min(l.durable, n)
	_, err := l.w.WriteString("b " + strconv.FormatUint(n, 10) + "\n")
	return err
}

// reserve makes sure the ledger on disk counts a byte total of at least n
// before a request reaches it: past the last reservation it appends one
// reserveAhead further and syncs it.
func (l *ledger) reserve(n uint64) error {
	// Note the total, and stop when the disk already covers it.
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.seen = max(l.seen, n)
	if n <= l.durable {
		return nil
	}

	// Append and sync a reservation past it.
	ahead := n + reserveAhead
	if _, err := l.w.WriteString("rb " + strconv.FormatUint(ahead, 10) + "\n"); err != nil {
		return err
	}
	if err := l.syncLocked(); err != nil {
		return err
	}
	l.durable = ahead
	return nil
}

// sync makes the appended lines durable.
func (l *ledger) sync() error {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.syncLocked()
}

// syncLocked syncs with mtx held.
func (l *ledger) syncLocked() error {
	if err := l.w.Flush(); err != nil {
		return err
	}
	return l.f.Sync()
}

// close syncs and closes the ledger.
func (l *ledger) close() error {
	// Sync, then close the file.
	err := l.sync()
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	return err
}
