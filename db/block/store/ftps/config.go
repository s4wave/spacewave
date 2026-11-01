package block_store_ftps

import "crypto/tls"

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
}
