package block_store_sql

// Config locates the database and the table the store writes.
type Config struct {
	// Host is the database host.
	Host string
	// Port is the database port, zero for 3306.
	Port uint32
	// Database is the database name.
	Database string
	// User is the database user.
	User string
	// Password authenticates User to the database.
	Password string
	// Table is the blocks table.
	Table string
	// TLS connects with TLS.
	TLS bool
	// TLSCertSHA256 is the hex SHA-256 of the server's certificate. Set, TLS
	// trusts exactly that certificate in place of a CA and the host name.
	TLSCertSHA256 string
	// MaxConns bounds the pool's open connections. Zero uses
	// DefaultMaxConns. Each block read holds one for a round trip.
	MaxConns int
}
