package plugin_host_wazero_quickjs

import (
	"io"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/linkedlist"
	"github.com/tetratelabs/wazero/experimental/fsapi"
	wazero_exp_sys "github.com/tetratelabs/wazero/experimental/sys"
)

// StdinBuffer is the buffer for stdin.
// The zero value is valid.
type StdinBuffer struct {
	// bcast guards below fields
	bcast broadcast.Broadcast
	// readQueue is the queued data to read.
	readQueue linkedlist.LinkedList[[]byte]
	// readQueueSize is the queued data size in bytes
	readQueueSize int
	// readQueueOffset is the offest in the first index that we have read through.
	readQueueOffset int
	// closed returns eof once readQueue is empty
	closed bool
}

// Read reads from the buffer.
func (b *StdinBuffer) Read(p []byte) (n int, err error) {
	b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		readn := len(p)
		if readn == 0 {
			n = 0
			if b.readQueueSize == 0 && b.closed {
				err = io.EOF
			} else {
				err = nil
			}
			return
		}

		// We have no data available to read, this would block Read.
		// Special behavior here: we don't have EAGAIN, so just return 0, nil
		// This works ONLY with wazero as it currently is implemented.
		// It's not a conventional behavior in Go but in this context it works.
		// IsNonblock always returns false, see internal/sys/stdio.go -> noopStdioFile
		if b.readQueueSize == 0 {
			n = 0
			err = nil
			return
		}

		// Read data to p.
		nextEntry, nextEntryOk := b.readQueue.Peek()
		if !nextEntryOk {
			// Should not happen since we checked readQueueSize > 0
			n = 0
			err = nil
			return
		}

		n = copy(p, nextEntry[b.readQueueOffset:])
		b.readQueueOffset += n
		b.readQueueSize -= n

		// If we've read all data from the first entry, remove it and reset offset
		if b.readQueueOffset >= len(nextEntry) {
			_, _ = b.readQueue.Pop()
			b.readQueueOffset = 0
		}

		// Check if we should return EOF
		if b.readQueueSize == 0 && b.closed {
			err = io.EOF
		} else {
			err = nil
		}

		// done with Read
	})
	return
}

// Write writes data to the buffer to be read later.
func (b *StdinBuffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if b.closed {
			err = io.ErrClosedPipe
			return
		}

		// Make a copy of the data to store in the queue
		data := make([]byte, len(p))
		copy(data, p)

		// Add to the read queue
		b.readQueue.Push(data)
		b.readQueueSize += len(data)
		n = len(p)

		// Notify any waiting readers
		broadcast()
	})

	return
}

// Close closes the buffer, causing Read to return EOF once all data is consumed.
func (b *StdinBuffer) Close() error {
	b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		b.closed = true
		// Notify any waiting readers
		broadcast()
	})
	return nil
}

// Poll checks if there is available data to read.
func (b *StdinBuffer) Poll(flag fsapi.Pflag, timeoutMillis int32) (ready bool, errno wazero_exp_sys.Errno) {
	// wait once only
	var waited bool
	for {
		// Check if we have data to read.
		var waitCh <-chan struct{}
		b.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// Ready if we have data to read or if we're closed
			ready = b.readQueueSize > 0 || b.closed
			errno = 0

			if !waited && !ready {
				waitCh = getWaitCh()
				waited = true
			}
		})
		if waitCh == nil {
			return
		}

		// Wait for either waitCh or timeoutMillis
		select {
		case <-waitCh:
		case <-time.After(time.Millisecond * time.Duration(timeoutMillis)):
		}
	}
}

// pollable has just the Poll function.
// https://github.com/tetratelabs/wazero/issues/1500#issuecomment-3041125375
type pollable interface {
	// Poll(fsapi.Pflag, int32) (ready bool, errno experimentalsys.Errno)
	Poll(fsapi.Pflag, int32) (ready bool, errno wazero_exp_sys.Errno)
}

// _ is a type assertion
var (
	_ io.ReadWriter = ((*StdinBuffer)(nil))
	_ io.Closer     = ((*StdinBuffer)(nil))
	_ pollable      = ((*StdinBuffer)(nil))
)
