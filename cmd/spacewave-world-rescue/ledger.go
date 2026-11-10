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

	"github.com/pkg/errors"
)

// Ref states the ledger records.
const (
	stateDone   = "done"
	stateFailed = "failed"
	stateAbsent = "absent"
	// stateNotInPack is a ref the index placed in a pack that lacks it.
	stateNotInPack = "not-in-pack"
)

// ledger is the append-only record of a rescue, which a restart reads to
// skip finished work. Its lines are:
//
//	r <hex block key> <state> [reason]   a ref finished as done, failed, absent
//	                                     or not-in-pack
//	p <pack id> <fetched> <found> <not-in-pack>
//	                                     every wanted ref of the pack finished,
//	                                     with the bytes read for it, the wanted
//	                                     refs stored and those it lacked
//	b <bytes>                            the bytes fetched across runs so far
//
// A later line for a ref replaces an earlier one. A partial last line, left
// by a crash, is ignored.
type ledger struct {
	f *os.File
	w *bufio.Writer
	// refs holds the state of each ref by block key.
	refs map[string]string
	// packs holds the finished packs.
	packs map[string]bool
	// bytes is the last byte total recorded.
	bytes uint64
}

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
	}
}

// ref appends the state of a ref, with a reason of one word for a failed or
// absent one.
func (l *ledger) ref(key []byte, state, reason string) error {
	// Format the line, its reason made one word.
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
// wanted refs found and those not in it.
func (l *ledger) pack(id string, fetched uint64, found, notInPack int) error {
	l.packs[id] = true
	_, err := fmt.Fprintf(l.w, "p %s %d %d %d\n", id, fetched, found, notInPack)
	return err
}

// total appends the byte total.
func (l *ledger) total(n uint64) error {
	l.bytes = n
	_, err := l.w.WriteString("b " + strconv.FormatUint(n, 10) + "\n")
	return err
}

// sync makes the appended lines durable.
func (l *ledger) sync() error {
	if err := l.w.Flush(); err != nil {
		return err
	}
	return l.f.Sync()
}

// close syncs and closes the ledger.
func (l *ledger) close() error {
	err := l.sync()
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	return err
}
