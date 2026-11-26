package saucer

import (
	"context"
	"io"
	"sync"
)

// muxConn bridges the HTTP GET and POST endpoints into an io.ReadWriteCloser for yamux.
type muxConn struct {
	// ctx bounds the mux transport.
	ctx context.Context
	// cancel stops the mux transport.
	cancel context.CancelFunc

	// writeCh receives data from JS (POST bodies) to be read by yamux.
	writeCh chan []byte

	// flushCh receives data from yamux to be written to the JS response.
	flushCh chan []byte

	// pendingMu guards pending.
	pendingMu sync.Mutex
	// pending holds leftover data from a previous writeCh read.
	pending []byte
}

// newMuxConn creates the document transport with a buffer in each direction.
func newMuxConn(ctx context.Context, cancel context.CancelFunc) *muxConn {
	return &muxConn{
		ctx:     ctx,
		cancel:  cancel,
		writeCh: make(chan []byte, 64),
		flushCh: make(chan []byte, 64),
	}
}

// Read returns data posted by JS to the mux write endpoint.
func (mc *muxConn) Read(p []byte) (int, error) {
	// Drain bytes retained from the previous document POST before waiting.
	mc.pendingMu.Lock()
	if len(mc.pending) > 0 {
		n := copy(p, mc.pending)
		mc.pending = mc.pending[n:]
		if len(mc.pending) == 0 {
			mc.pending = nil
		}
		mc.pendingMu.Unlock()
		return n, nil
	}
	mc.pendingMu.Unlock()

	// Read the next document POST and retain bytes beyond the caller buffer.
	select {
	case <-mc.ctx.Done():
		return 0, mc.ctx.Err()
	case data, ok := <-mc.writeCh:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, data)
		if n < len(data) {
			mc.pendingMu.Lock()
			mc.pending = data[n:]
			mc.pendingMu.Unlock()
		}
		return n, nil
	}
}

// Write sends data from yamux to JS via the streaming GET response.
func (mc *muxConn) Write(p []byte) (int, error) {
	// Copy the frame because the HTTP response can outlive this Write call.
	data := make([]byte, len(p))
	copy(data, p)

	// Queue the frame until the HTTP response can accept it or the mux closes.
	select {
	case mc.flushCh <- data:
		return len(p), nil
	case <-mc.ctx.Done():
		return 0, mc.ctx.Err()
	}
}

// Close closes the mux connection.
func (mc *muxConn) Close() error {
	mc.cancel()
	return nil
}

// flushTo forwards queued yamux frames to dst until either context ends,
// the output queue closes, or dst returns an error. It returns only writer errors.
func (mc *muxConn) flushTo(ctx context.Context, dst io.Writer) error {
	for {
		select {
		case <-mc.ctx.Done():
			return nil
		case <-ctx.Done():
			mc.cancel()
			return nil
		case data, ok := <-mc.flushCh:
			if !ok {
				return nil
			}
			if _, err := dst.Write(data); err != nil {
				return err
			}
		}
	}
}

// muxInputResult records which case ended a POST body's wait to enter the mux.
type muxInputResult uint8

const (
	// muxInputQueued means yamux can read the body from the queue.
	muxInputQueued muxInputResult = iota
	// muxInputClosed means the mux ended while the POST was waiting.
	muxInputClosed
	// muxInputCanceled means the POST request ended while it was waiting.
	muxInputCanceled
)

// queueInput waits to hand the POST body to yamux or for either caller to cancel.
func (mc *muxConn) queueInput(ctx context.Context, body []byte) muxInputResult {
	select {
	case mc.writeCh <- body:
		return muxInputQueued
	case <-mc.ctx.Done():
		return muxInputClosed
	case <-ctx.Done():
		return muxInputCanceled
	}
}

// _ is a type assertion.
var _ io.ReadWriteCloser = (*muxConn)(nil)
