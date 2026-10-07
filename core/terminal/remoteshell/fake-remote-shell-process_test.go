//go:build !js

package remoteshell

import (
	"bytes"
	"io"
	"sync"
)

// fakeRemoteShellProcess is an in-memory remoteShellProcess whose output the
// test controls.
type fakeRemoteShellProcess struct {
	input     bytes.Buffer
	readCh    chan []byte
	done      chan struct{}
	closeOnce sync.Once
	readOnce  sync.Once
	cols      uint32
	rows      uint32
	closed    bool
}

func newFakeRemoteShellProcess() *fakeRemoteShellProcess {
	return &fakeRemoteShellProcess{
		readCh: make(chan []byte),
		done:   make(chan struct{}),
	}
}

func (p *fakeRemoteShellProcess) Read(buf []byte) (int, error) {
	data, ok := <-p.readCh
	if !ok {
		return 0, io.EOF
	}
	return copy(buf, data), nil
}

func (p *fakeRemoteShellProcess) Write(buf []byte) (int, error) {
	return p.input.Write(buf)
}

func (p *fakeRemoteShellProcess) Resize(cols, rows uint32) error {
	p.cols = cols
	p.rows = rows
	return nil
}

func (p *fakeRemoteShellProcess) Close() error {
	p.closeOnce.Do(func() {
		p.closed = true
		close(p.done)
		p.closeOutput()
	})
	return nil
}

func (p *fakeRemoteShellProcess) closeOutput() {
	p.readOnce.Do(func() {
		close(p.readCh)
	})
}

func (p *fakeRemoteShellProcess) Wait() (int, error) {
	<-p.done
	return 0, nil
}
