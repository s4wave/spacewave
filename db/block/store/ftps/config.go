package block_store_ftps

import (
	"crypto/tls"
	"time"
)

// Config configures a Client.
type Config struct {
	// Addr is the server's host:port; a bare host uses port 21.
	Addr string
	// Logins are the users the Client logs in as, at least one. A server that
	// limits connections per user allows that many more with each login; the
	// pool spreads its sessions across them evenly.
	Logins []Login
	// TLSConfig verifies the server; nil verifies the certificate against the
	// host name of Addr.
	TLSConfig *tls.Config
	// MaxSessions is the most sessions open at once from this client across
	// all logins, spares included; zero uses DefaultMaxSessions. The pool
	// holds one fewer, so a spare has room.
	MaxSessions int
	// ConnsPerLogin is the most sessions open at once on one login, counting
	// the spares dialed ahead of range reads; zero uses DefaultConnsPerLogin.
	ConnsPerLogin int
	// NoFullRetry makes a dial that the server answers with its session limit
	// fail at once instead of waiting and dialing again, for a caller that
	// must stop on that answer.
	NoFullRetry bool
	// CloseOnDone closes a session's control and data connections when the
	// context of the call using it ends first, so a stalled command does not
	// keep its session open. The session's token is freed only once its
	// sockets are confirmed closed. The data connections are then dialed by
	// the client and wrapped in TLS without session resumption checks beyond
	// the shared cache.
	CloseOnDone bool
	// ShutTimeout bounds the wait for the server's reply at a transfer's
	// end; zero is 30 seconds.
	ShutTimeout time.Duration
	// CloseConfirmWait bounds the wait under CloseOnDone for the server to
	// answer a closed session with its own close; zero is 10 seconds. A
	// session it does not answer in time keeps its token for good, since the
	// server may still count it.
	CloseConfirmWait time.Duration
}
