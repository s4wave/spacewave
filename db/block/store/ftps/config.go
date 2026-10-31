//go:build !tinygo

package block_store_ftps

import "crypto/tls"

// Config configures a Client.
type Config struct {
	// Addr is the server's host:port; a bare host uses port 21.
	Addr string
	// User is the login name.
	User string
	// Password is the login password.
	Password string
	// TLSConfig verifies the server; nil verifies the certificate against the
	// host name of Addr.
	TLSConfig *tls.Config
	// Conns is the number of control connections; zero uses DefaultConns.
	Conns int
}
