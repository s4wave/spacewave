package plugin_host_web

import (
	"context"
	"sync"

	"github.com/aperturerobotics/starpc/srpc"
)

// workerStreams opens RPC streams to one web worker and ends the open ones
// when the worker is gone. The web runtime routes a stream by worker ID and
// holds it until a worker with that ID answers, so without this a call made
// to a worker that exited waits forever.
type workerStreams struct {
	// open opens a stream through the web runtime.
	open srpc.OpenStreamFunc

	// mtx guards the fields below.
	mtx sync.Mutex
	// streams holds the streams opened and not yet closed.
	streams map[*workerStream]struct{}
	// closed is set once the execution ends; later opens fail with it.
	closed error
}

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

// newWorkerStreams wraps open, which opens a stream to the worker.
func newWorkerStreams(open srpc.OpenStreamFunc) *workerStreams {
	return &workerStreams{open: open, streams: make(map[*workerStream]struct{})}
}

// OpenStream opens a stream to the worker. It implements srpc.OpenStreamFunc.
func (s *workerStreams) OpenStream(ctx context.Context, msgHandler srpc.PacketDataHandler, closeHandler srpc.CloseHandler) (srpc.PacketWriter, error) {
	// Register the stream first, so an end during open reaches it.
	strm := &workerStream{closeHandler: closeHandler}
	s.mtx.Lock()
	if err := s.closed; err != nil {
		s.mtx.Unlock()
		return nil, err
	}
	s.streams[strm] = struct{}{}
	s.mtx.Unlock()

	// Open it and publish the writer, unless an end came first.
	writer, err := s.open(ctx, msgHandler, func(closeErr error) {
		s.remove(strm)
		strm.close(closeErr)
	})
	s.mtx.Lock()
	ended := strm.ended
	if err == nil && ended == nil {
		strm.writer = writer
	}
	s.mtx.Unlock()
	switch {
	case err != nil:
		s.remove(strm)
		return nil, err
	case ended != nil:
		_ = writer.Close()
		return nil, ended
	}
	return writer, nil
}

// endStreams fails every open stream with err, leaving later opens to reach
// the next worker.
func (s *workerStreams) endStreams(err error) {
	// Take the open streams and mark each ended.
	s.mtx.Lock()
	strms := make([]*workerStream, 0, len(s.streams))
	writers := make([]srpc.PacketWriter, 0, len(s.streams))
	for strm := range s.streams {
		strm.ended = err
		strms = append(strms, strm)
		writers = append(writers, strm.writer)
	}
	clear(s.streams)
	s.mtx.Unlock()

	// Fail each call, then close its stream to the runtime.
	for i, strm := range strms {
		strm.close(err)
		if writers[i] != nil {
			_ = writers[i].Close()
		}
	}
}

// close ends every open stream and fails later opens with err.
func (s *workerStreams) close(err error) {
	s.mtx.Lock()
	s.closed = err
	s.mtx.Unlock()
	s.endStreams(err)
}

// remove forgets strm.
func (s *workerStreams) remove(strm *workerStream) {
	s.mtx.Lock()
	delete(s.streams, strm)
	s.mtx.Unlock()
}

// close calls the caller's close handler once.
func (s *workerStream) close(err error) {
	s.once.Do(func() { s.closeHandler(err) })
}
