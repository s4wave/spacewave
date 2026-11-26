package provider_local

import (
	"context"
	"errors"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/sirupsen/logrus"
)

// peerCause is why a peer read failed while its caller waited.
type peerCause int

const (
	// peerError is a read a peer answered with an error: its own lookup of
	// the block failed.
	peerError peerCause = iota
	// peerTimeout is a read whose bound ended with a request outstanding.
	peerTimeout
	// peerNoSession is a read that found no open peer session to ask, or
	// whose session closed under the request.
	peerNoSession
	// peerExchange is a read whose exchange with a peer failed, such as a
	// failed send, a reset stream or a block that failed to verify, which
	// says nothing of whether the peer holds the block.
	peerExchange
	// peerDenied is a read a peer rejected for the requester's read
	// authority, which says nothing of whether the peer holds the block.
	peerDenied
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
	case peerExchange:
		return "exchange"
	case peerDenied:
		return "denied"
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
	case errors.Is(err, dex_solicit.ErrPeerDenied):
		return peerDenied
	case errors.Is(err, dex_solicit.ErrNoPeerSession):
		return peerNoSession
	case errors.Is(err, context.DeadlineExceeded):
		return peerTimeout
	default:
		return peerExchange
	}
}

// peerRefused reports whether err proves that the peers cannot serve the
// block: every failure it carries is a peer's own error answer, beside the
// block.ErrUnavailable that wraps them. A denial, a failed exchange, a closed
// session or a timeout from any peer leaves it unproven, since that peer may
// hold it: a denial answers the requester's authority, which a grant on the
// same session restores.
func peerRefused(err error) bool {
	// Walk err's tree: every leaf must be the wrapper or a refusal, and one
	// must be a refusal.
	refused := false
	var walk func(error) bool
	walk = func(err error) bool {
		switch e := err.(type) {
		case interface{ Unwrap() []error }:
			for _, inner := range e.Unwrap() {
				if !walk(inner) {
					return false
				}
			}
			return true
		case interface{ Unwrap() error }:
			return walk(e.Unwrap())
		}
		switch {
		case errors.Is(err, dex_solicit.ErrPeerError):
			refused = true
			return true
		case errors.Is(err, block.ErrUnavailable):
			return true
		}
		return false
	}
	return walk(err) && refused
}

// peerSiteDepth bounds the stack walk that names a failed read's reader.
const peerSiteDepth = 64

// peerFailSeen holds the refs whose failed peer read this process has
// warned of, the last seenRefsCap of them, for every World in the process.
var peerFailSeen = newSeenRefs()

// mainModule is the import path of the binary's main module, or "" when the
// binary carries no build info.
var mainModule = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Path
})

// warnPeerFailed warns the first time this process fails a peer read of ref
// while its caller waited. The warning carries the ref, the cause and two
// function names from the reader's stack: site, the innermost frame in the
// binary's main module, which names the application code that asked, and
// root, the outermost frame walked, which names the goroutine the
// read ran in. It runs only on the failure path, and walks the stack only for
// a block's first failure.
func warnPeerFailed(le *logrus.Entry, ref *block.BlockRef, cause peerCause) {
	key := ref.MarshalString()
	if !peerFailSeen.add(key) {
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
// returns the innermost frame in the main module, and the outermost frame
// other than the runtime's. Either is "-" when the walk found none.
func readerSite() (site, root string) {
	// Capture the stack above warnPeerFailed.
	var pcs [peerSiteDepth]uintptr
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs[:])])
	mod := mainModule()

	// Keep the innermost main module frame and the outermost frame.
	site, root = "-", "-"
	for {
		frame, more := frames.Next()
		fn := frame.Function
		if fn != "" && !strings.HasPrefix(fn, "runtime.") {
			root = fn
			if site == "-" && inModule(fn, mod) {
				site = fn
			}
		}
		if !more {
			return site, root
		}
	}
}

// inModule reports whether the function fn is in the module whose path is
// mod, which holds the binary's package main as well as mod's own packages.
func inModule(fn, mod string) bool {
	// Accept package main, then any package under mod.
	if strings.HasPrefix(fn, "main.") {
		return true
	}
	rest, ok := strings.CutPrefix(fn, mod)
	return ok && mod != "" && (strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "."))
}
