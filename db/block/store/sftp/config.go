package block_store_sftp

// Config configures a Client.
type Config struct {
	// Addr is the server's host:port; a bare host uses port 22.
	Addr string
	// User is the login name.
	User string
	// Password is the login password.
	Password string
	// HostKey pins the server's SSH host key: an authorized_keys style public
	// key line, which also limits the handshake to that key's type, or the
	// "SHA256:..." fingerprint ssh prints, which fails when the server shows
	// another of its keys. Empty trusts the
	// first key the server shows and requires the same one on every later
	// connection of the Client.
	HostKey string
	// Conns is the number of SSH connections the client holds, each carrying
	// many requests at once; zero uses DefaultConns.
	Conns int
}
