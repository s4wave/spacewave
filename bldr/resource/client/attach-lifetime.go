package resource_client

import (
	"errors"
	"sync"

	"github.com/s4wave/spacewave/bldr/resource"
)

type attachLifetime struct {
	client *Client

	// mtx guards below fields.
	mtx sync.Mutex
	// sess is the cached shared attach session.
	sess *attachSession
	// initCh is closed when in-flight attach session initialization finishes.
	initCh chan struct{}
}

func newAttachLifetime(client *Client) *attachLifetime {
	return &attachLifetime{client: client}
}

func (l *attachLifetime) currentSession() *attachSession {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.sess
}

func (l *attachLifetime) ensureSession() (*attachSession, error) {
	for {
		l.mtx.Lock()
		if l.sess != nil {
			sess := l.sess
			l.mtx.Unlock()
			return sess, nil
		}
		if l.initCh != nil {
			initCh := l.initCh
			l.mtx.Unlock()

			select {
			case <-l.client.ctx.Done():
				return nil, l.client.ctx.Err()
			case <-initCh:
				continue
			}
		}
		initCh := make(chan struct{})
		l.initCh = initCh
		l.mtx.Unlock()

		sess, err := l.client.openAttachSession()

		l.mtx.Lock()
		if err == nil {
			l.sess = sess
		}
		l.initCh = nil
		close(initCh)
		l.mtx.Unlock()

		if err != nil {
			return nil, err
		}
		return sess, nil
	}
}

func (l *attachLifetime) clearSession(sess *attachSession) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if l.sess == sess {
		l.sess = nil
	}
}

func (l *attachLifetime) setRelease(resourceID uint32, releaseFn func()) {
	if releaseFn == nil {
		return
	}
	sess := l.currentSession()
	if sess == nil {
		return
	}

	sess.setRelease(resourceID, releaseFn)
}

type attachPendingAcks struct {
	// mtx guards below fields.
	mtx sync.Mutex
	// nextID is the next attach correlation id.
	nextID uint32
	// pending maps attach correlation ids to waiters.
	pending map[uint32]*pendingAttach
}

func newAttachPendingAcks() *attachPendingAcks {
	return &attachPendingAcks{
		pending: make(map[uint32]*pendingAttach),
	}
}

func (p *attachPendingAcks) add() (uint32, <-chan attachResult) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.nextID++
	ch := make(chan attachResult, 1)
	p.pending[p.nextID] = &pendingAttach{ch: ch}
	return p.nextID, ch
}

func (p *attachPendingAcks) remove(attachID uint32) {
	p.mtx.Lock()
	delete(p.pending, attachID)
	p.mtx.Unlock()
}

func (p *attachPendingAcks) cancel(attachID uint32) (uint32, bool) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	pending := p.pending[attachID]
	if pending == nil {
		return 0, false
	}
	if pending.resolved {
		delete(p.pending, attachID)
		if pending.result.err == nil {
			return pending.result.resourceID, true
		}
		return 0, false
	}
	pending.canceled = true
	return 0, false
}

func (p *attachPendingAcks) complete(attachID uint32) {
	p.mtx.Lock()
	delete(p.pending, attachID)
	p.mtx.Unlock()
}

func (p *attachPendingAcks) failAll(err error) {
	p.mtx.Lock()
	pending := p.pending
	p.pending = make(map[uint32]*pendingAttach)
	p.mtx.Unlock()

	for _, entry := range pending {
		if entry.resolved {
			continue
		}
		entry.ch <- attachResult{err: err}
	}
}

func (p *attachPendingAcks) resolve(addAck *resource.ResourceAttachAddAck) (uint32, bool) {
	attachID := addAck.GetAttachId()

	p.mtx.Lock()
	pending := p.pending[attachID]
	if pending == nil {
		p.mtx.Unlock()
		return 0, false
	}
	if pending.resolved {
		p.mtx.Unlock()
		return 0, false
	}

	if pending.canceled {
		delete(p.pending, attachID)
		if addAck.GetError() == "" {
			p.mtx.Unlock()
			return addAck.GetResourceId(), true
		}
		p.mtx.Unlock()
		return 0, false
	}

	pending.resolved = true
	pending.result = attachResult{resourceID: addAck.GetResourceId()}
	if addAck.GetError() != "" {
		pending.result = attachResult{err: errors.New(addAck.GetError())}
	}

	ch := pending.ch
	result := pending.result
	p.mtx.Unlock()
	ch <- result
	return 0, false
}

func (p *attachPendingAcks) len() int {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	return len(p.pending)
}
