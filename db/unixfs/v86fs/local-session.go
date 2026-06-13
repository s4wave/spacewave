package unixfs_v86fs

import (
	"context"

	"github.com/pkg/errors"
)

// LocalSession is an in-process v86fs client session. It shares the SRPC
// relay's mount, inode, handle, notification, and release lifecycle.
type LocalSession struct {
	server *Server
	sess   *session
}

// NewLocalSession opens a v86fs session without an SRPC stream.
func NewLocalSession(ctx context.Context, server *Server) *LocalSession {
	if ctx == nil {
		ctx = context.Background()
	}
	if server == nil {
		server = NewServer(nil)
	}
	sess := &session{
		server:  server,
		ctx:     ctx,
		inodes:  make(map[uint64]*inodeEntry),
		handles: make(map[uint64]*handleEntry),
	}

	server.mtx.Lock()
	server.sessions[sess] = struct{}{}
	for _, entry := range server.mounts {
		if msg := mountNotifyMsg(entry); msg != nil {
			sess.pending = append(sess.pending, msg)
		}
	}
	server.mtx.Unlock()

	return &LocalSession{server: server, sess: sess}
}

// Close releases all FSHandle references owned by the session.
func (s *LocalSession) Close() {
	if s == nil || s.sess == nil {
		return
	}
	s.server.mtx.Lock()
	delete(s.server.sessions, s.sess)
	s.server.mtx.Unlock()
	s.sess.cleanup()
	s.sess = nil
}

// HandleMessage dispatches one request and returns the reply frame.
func (s *LocalSession) HandleMessage(ctx context.Context, msg *V86FsMessage) (*V86FsMessage, error) {
	if s == nil || s.sess == nil {
		return nil, errors.New("v86fs local session is closed")
	}
	if ctx == nil {
		ctx = s.sess.context()
	}
	reply, err := s.sess.dispatch(ctx, msg)
	if err != nil {
		return &V86FsMessage{
			Tag: msg.GetTag(),
			Body: &V86FsMessage_ErrorReply{
				ErrorReply: &V86FsErrorReply{Status: errnoFromError(err)},
			},
		}, nil
	}
	return reply, nil
}

// DrainNotifications returns pending server-to-guest notifications.
func (s *LocalSession) DrainNotifications() []*V86FsMessage {
	if s == nil || s.sess == nil {
		return nil
	}
	var pending []*V86FsMessage
	s.sess.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pending = s.sess.pending
		s.sess.pending = nil
	})
	return pending
}

// RequeueNotifications restores drained notifications the guest could not yet
// accept to the front of the pending queue, preserving delivery order so a
// notification leaves the session only once it lands in a guest receive buffer.
func (s *LocalSession) RequeueNotifications(msgs []*V86FsMessage) {
	if s == nil || s.sess == nil || len(msgs) == 0 {
		return
	}
	s.sess.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.sess.pending = append(msgs, s.sess.pending...)
		broadcast()
	})
}
