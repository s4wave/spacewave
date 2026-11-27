package plugin_host_web

import (
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
)

// workerStream is one stream opened through workerStreams.
type workerStream struct {
	// closeHandler is the caller's close handler, called once.
	closeHandler srpc.CloseHandler
	// once guards closeHandler.
	once sync.Once
	// writer is the opened stream, set under the set's mtx once open returns.
	writer srpc.PacketWriter
	// ended is the error the set ended the stream with, under the set's mtx.
	ended error
}

// opened publishes a successful open unless the set already ended the stream.
// The caller holds the set's mtx.
func (s *workerStream) opened(writer srpc.PacketWriter, err error) error {
	if err == nil && s.ended == nil {
		s.writer = writer
	}
	return s.ended
}

// end records the set's end and returns the writer to close after unlocking.
// The caller holds the set's mtx and calls close before closing the writer.
func (s *workerStream) end(err error) srpc.PacketWriter {
	s.ended = err
	return s.writer
}

// close calls the caller's close handler once.
func (s *workerStream) close(err error) {
	s.once.Do(func() { s.closeHandler(err) })
}
