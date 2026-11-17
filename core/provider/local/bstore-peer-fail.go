package provider_local

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/sirupsen/logrus"
)

// peerCause is why a peer read failed while its caller waited.
type peerCause int

const (
	// peerError is a read a peer answered with an error, or whose exchange
	// with a peer failed.
	peerError peerCause = iota
	// peerTimeout is a read whose bound ended with a request outstanding.
	peerTimeout
	// peerNoSession is a read that found no open peer session to ask, or
	// whose session closed under the request.
	peerNoSession
	// peerCauseCount is the number of causes.
	peerCauseCount
)

// String returns the cause's name in logs and metric labels.
func (c peerCause) String() string {
	switch c {
	case peerTimeout:
		return "timeout"
	case peerNoSession:
		return "no_session"
	default:
		return "peer_error"
	}
}

// peerFailCause returns the cause of one failed attempt to read from peers.
// A peer's own error outranks a closed session, which outranks a timeout, so
// a fanout over several peers reports the most telling of their failures.
func peerFailCause(err error) peerCause {
	switch {
	case errors.Is(err, dex_solicit.ErrPeerError):
		return peerError
	case errors.Is(err, dex_solicit.ErrNoPeerSession):
		return peerNoSession
	case errors.Is(err, context.DeadlineExceeded):
		return peerTimeout
	default:
		return peerError
	}
}

// peerSiteDepth bounds the stack walk that names a failed read's reader.
const peerSiteDepth = 64

// peerFailSeen holds the refs whose failed peer read this process has
// warned of, keyed by their marshaled string. It grows by one entry per
// distinct block that fails, which stays small while peers serve the World.
var peerFailSeen sync.Map

// warnPeerFailed warns the first time this process fails a peer read of ref
// while its caller waited. The warning carries the ref, the cause and two
// function names from the reader's stack: site, the innermost frame outside
// Spacewave and the standard library, which names the application code that
// asked, and root, the outermost frame walked, which names the goroutine the
// read ran in. It runs only on the failure path, and walks the stack only for
// a block's first failure.
func warnPeerFailed(le *logrus.Entry, ref *block.BlockRef, cause peerCause) {
	key := ref.MarshalString()
	if _, loaded := peerFailSeen.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	site, root := readerSite()
	le.WithField("ref", key).
		WithField("cause", cause.String()).
		WithField("site", site).
		WithField("root", root).
		Warn("peer read failed; the block stays unserved")
}

// readerSite walks at most peerSiteDepth frames of the caller's stack and
// returns the innermost frame outside Spacewave and the standard library,
// and the outermost frame other than the runtime's. Either is "-" when the
// walk found none.
func readerSite() (site, root string) {
	var pcs [peerSiteDepth]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs[:])])
	site, root = "-", "-"
	for {
		frame, more := frames.Next()
		fn := frame.Function
		if fn != "" && !strings.HasPrefix(fn, "runtime.") {
			root = fn
			if site == "-" && appFrame(fn) {
				site = fn
			}
		}
		if !more {
			return site, root
		}
	}
}

// appFrame reports whether the function fn is outside Spacewave and the
// standard library, whose import paths start with no domain.
func appFrame(fn string) bool {
	if strings.HasPrefix(fn, "github.com/s4wave/spacewave/") {
		return false
	}
	host, _, found := strings.Cut(fn, "/")
	return found && strings.Contains(host, ".")
}
