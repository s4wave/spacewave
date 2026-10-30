//go:build !tinygo

package block_store_ftp

import "crypto/tls"

// Config locates the server and the directory the stores write under.
type Config struct {
	// Host is the server host.
	Host string
	// Port is the server port, zero for 21.
	Port uint32
	// User is the login user.
	User string
	// Password is the login password.
	Password string
	// Root is the directory that holds the stores, such as "/spacewave". A
	// relative directory is under the user's login directory.
	Root string
	// TLSConfig overrides the TLS settings, such as the root CAs of a test
	// server. Nil verifies Host against the system roots.
	TLSConfig *tls.Config
	// MaxConns bounds the open connections, zero for 8.
	MaxConns int
}
