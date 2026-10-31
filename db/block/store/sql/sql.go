//go:build !tinygo

// Package block_store_sql stores blocks as rows of a MySQL-compatible
// database, such as MariaDB.
//
// The store only ever adds rows: a write inserts each block once, keyed by
// block store id and hash, and no operation deletes or rewrites a row. A
// database user granted SELECT and INSERT alone runs it, so the grants enforce
// that no block is lost.
package block_store_sql

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// ErrAppendOnly is returned by RmBlock: the store never deletes a block.
var ErrAppendOnly = errors.New("the SQL block store never deletes a block")

// existsBatchSize bounds the hashes one existence query names.
const existsBatchSize = 1000

// minPacketBytes is the smallest statement budget assumed when the server's
// packet limit is unreadable.
const minPacketBytes = 1 << 20

// rowOverhead is the bytes one inserted row adds to a statement beside its
// data and refs: the placeholders and the encoded store id and hash.
const rowOverhead = 512

// Config locates the database and the table the store writes.
type Config struct {
	// Host is the database host and Port its port, zero for 3306.
	Host string
	Port uint32
	// Database is the database name.
	Database string
	// User and Password authenticate to the database.
	User, Password string
	// Table is the blocks table.
	Table string
	// TLS connects with TLS.
	TLS bool
	// TLSCertSHA256 is the hex SHA-256 of the server's certificate. Set, TLS
	// trusts exactly that certificate in place of a CA and the host name.
	TLSCertSHA256 string
}

// Open opens a connection pool to the database. It connects lazily, so an
// unreachable database fails the first operation instead.
func Open(conf *Config) (*sql.DB, error) {
	// Address the server, defaulting to the MySQL port.
	port := conf.Port
	if port == 0 {
		port = 3306
	}

	// Name the login and database.
	mc := mysql.NewConfig()
	mc.User, mc.Passwd = conf.User, conf.Password
	mc.Net, mc.Addr = "tcp", net.JoinHostPort(conf.Host, strconv.FormatUint(uint64(port), 10))
	mc.DBName = conf.Database

	// Bound a stalled network and let the server's packet limit apply.
	mc.Timeout = 10 * time.Second
	mc.ReadTimeout = 2 * time.Minute
	mc.WriteTimeout = 2 * time.Minute
	mc.MaxAllowedPacket = 0

	// Send each query's arguments inline, so a read is one round trip
	// instead of a prepare, an execute and a close.
	mc.InterpolateParams = true
	if conf.TLS {
		mc.TLSConfig = "true"
	}
	if conf.TLS && conf.TLSCertSHA256 != "" {
		mc.TLS = pinnedTLS(conf.Host, conf.TLSCertSHA256)
	}

	// Build the connector the pool dials through.
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, err
	}

	// Recycle connections before a hosted server's idle timeout drops them.
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	db.SetConnMaxIdleTime(time.Minute)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// EnsureTable creates the blocks table when it does not exist.
func EnsureTable(ctx context.Context, db *sql.DB, table string) error {
	_, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS "+quoteName(table)+` (
  store_id VARBINARY(255) NOT NULL,
  hash VARBINARY(128) NOT NULL,
  data LONGBLOB NOT NULL,
  refs LONGBLOB NULL,
  PRIMARY KEY (store_id, hash)
)`)
	return errors.Wrap(err, "create blocks table")
}

// Store is one block store's rows in the blocks table.
type Store struct {
	le      *logrus.Entry
	db      *sql.DB
	table   string
	storeID []byte
	// packetOnce reads the server's packet limit, which bounds a statement.
	packetOnce  sync.Once
	packetBytes int
}

// NewStore builds the store of storeID's rows in table. The store owns db and
// closes it on Close.
func NewStore(le *logrus.Entry, db *sql.DB, table, storeID string) *Store {
	return &Store{le: le, db: db, table: quoteName(table), storeID: []byte(storeID)}
}

// Close closes the connection pool.
func (s *Store) Close() {
	if err := s.db.Close(); err != nil {
		s.le.WithError(err).Warn("close SQL block store")
	}
}

// GetHashType returns 0 to use the default hash type.
func (s *Store) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *Store) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the store as the scoped read handle.
func (s *Store) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock inserts one block.
func (s *Store) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Hash the block, then insert it as a batch of one.
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

// PutBlockBatch inserts the batch in as few statements as the server's packet
// limit allows. A block already stored keeps its row. Tombstones are
// rejected. No block is reported as existing.
func (s *Store) PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) ([]bool, error) {
	// Split the batch into statements under the packet limit, one block at
	// least each.
	limit := s.statementBytes(ctx)
	var rows []any
	var size int
	flush := func() error {
		// Insert the gathered rows in one statement.
		if len(rows) == 0 {
			return nil
		}
		n := len(rows) / 4
		query := "INSERT IGNORE INTO " + s.table + " (store_id, hash, data, refs) VALUES " + // #nosec G202 -- NewStore quotes the table and values are placeholders.
			strings.TrimSuffix(strings.Repeat("(?,?,?,?),", n), ",")
		_, err := s.db.ExecContext(ctx, query, rows...)
		rows, size = rows[:0], 0
		return errors.Wrap(err, "insert blocks")
	}
	for _, entry := range batch {
		if entry.Tombstone {
			return nil, ErrAppendOnly
		}
		key, err := hashKey(entry.Ref)
		if err != nil {
			return nil, err
		}
		var refs []byte
		if entry.RefsKnown {
			if refs, err = encodeRefs(entry.Refs); err != nil {
				return nil, err
			}
		}
		rowBytes := len(entry.Data) + len(refs) + rowOverhead
		if len(rows) != 0 && size+rowBytes > limit {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		rows = append(rows, s.storeID, key, entry.Data, refs)
		size += rowBytes
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return make([]bool, len(batch)), nil
}

// statementBytes returns the bytes one insert statement may carry: half the
// server's packet limit, which leaves room for the encoding.
func (s *Store) statementBytes(ctx context.Context) int {
	s.packetOnce.Do(func() {
		var packet int64
		err := s.db.QueryRowContext(ctx, "SELECT @@max_allowed_packet").Scan(&packet)
		if err != nil || packet <= 0 {
			s.le.WithError(err).Warn("read the SQL server's packet limit")
			packet = 2 * minPacketBytes
		}
		s.packetBytes = int(max(packet/2, minPacketBytes))
	})
	return s.packetBytes
}

// GetBlock looks up a block in the store.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock returns the block with its refs. Returns nil if the block is
// not found.
func (s *Store) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Read the block's row.
	key, err := hashKey(ref)
	if err != nil {
		return nil, err
	}
	var data, refs []byte
	err = s.db.QueryRowContext(ctx, "SELECT data, refs FROM "+s.table+" WHERE store_id = ? AND hash = ?", s.storeID, key).Scan(&data, &refs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "read block")
	}

	// Decode its refs; a NULL column means they are unknown.
	stored := &block.StoredBlock{Data: data}
	if refs != nil {
		if stored.Refs, err = decodeRefs(refs); err != nil {
			return nil, err
		}
		stored.RefsKnown = true
	}
	return stored, nil
}

// GetBlockExists checks if a block exists.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	exists, err := s.GetBlockExistsBatch(ctx, []*block.BlockRef{ref})
	if err != nil {
		return false, err
	}
	return exists[0], nil
}

// GetBlockExistsBatch checks the store for each block, a thousand per query.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	exists := make([]bool, len(refs))
	for start := 0; start < len(refs); start += existsBatchSize {
		// Name the chunk's hashes and map each back to its positions.
		chunk := refs[start:min(start+existsBatchSize, len(refs))]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, s.storeID)
		at := make(map[string][]int, len(chunk))
		for i, ref := range chunk {
			key, err := hashKey(ref)
			if err != nil {
				return nil, err
			}
			if at[string(key)] == nil {
				args = append(args, key)
			}
			at[string(key)] = append(at[string(key)], start+i)
		}

		// Mark each hash the table holds.
		query := "SELECT hash FROM " + s.table + " WHERE store_id = ? AND hash IN (" + // #nosec G202 -- NewStore quotes the table and values are placeholders.
			strings.TrimSuffix(strings.Repeat("?,", len(args)-1), ",") + ")"
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, errors.Wrap(err, "check blocks")
		}
		for rows.Next() {
			var key []byte
			if err := rows.Scan(&key); err != nil {
				rows.Close()
				return nil, errors.Wrap(err, "check blocks")
			}
			for _, i := range at[string(key)] {
				exists[i] = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, errors.Wrap(err, "check blocks")
		}
	}
	return exists, nil
}

// StatBlock returns the block's metadata, or nil if the block is not found.
func (s *Store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Read the stored length of the block's data.
	key, err := hashKey(ref)
	if err != nil {
		return nil, err
	}
	var size int64
	err = s.db.QueryRowContext(ctx, "SELECT LENGTH(data) FROM "+s.table+" WHERE store_id = ? AND hash = ?", s.storeID, key).Scan(&size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "stat block")
	}
	return &block.BlockStat{Ref: ref, Size: size}, nil
}

// RmBlock returns ErrAppendOnly: the store never deletes a block.
func (s *Store) RmBlock(context.Context, *block.BlockRef) error {
	return ErrAppendOnly
}

// Sync reports always-durable: each insert commits before it returns.
func (s *Store) Sync(context.Context) (bool, error) {
	return true, nil
}

// Reclaim drops nothing and calls neither fence nor live: every block stays.
func (s *Store) Reclaim(
	context.Context,
	func(context.Context) error,
	func(context.Context, []*block.BlockRef) ([]bool, error),
) (time.Time, error) {
	return time.Time{}, nil
}

// hashKey returns the row key of a block reference.
func hashKey(ref *block.BlockRef) ([]byte, error) {
	if ref.GetEmpty() {
		return nil, block.ErrEmptyBlockRef
	}
	return ref.GetHash().MarshalVT()
}

// encodeRefs encodes the refs as a sequence of length-prefixed references. No
// refs encode as an empty, non-nil value, so a leaf differs from unknown refs.
func encodeRefs(refs []*block.BlockRef) ([]byte, error) {
	out := make([]byte, 0, len(refs)*40)
	for _, ref := range refs {
		data, err := ref.MarshalVT()
		if err != nil {
			return nil, err
		}
		out = binary.AppendUvarint(out, uint64(len(data)))
		out = append(out, data...)
	}
	return out, nil
}

// decodeRefs decodes the refs encodeRefs wrote.
func decodeRefs(data []byte) ([]*block.BlockRef, error) {
	refs := []*block.BlockRef{}
	for len(data) != 0 {
		n, w := binary.Uvarint(data)
		if w <= 0 {
			return nil, errors.New("decode block refs: truncated")
		}
		data = data[w:]
		size, err := strconv.Atoi(strconv.FormatUint(n, 10))
		if err != nil || size > len(data) {
			return nil, errors.New("decode block refs: truncated")
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(data[:size]); err != nil {
			return nil, errors.Wrap(err, "decode block refs")
		}
		refs = append(refs, ref)
		data = data[size:]
	}
	return refs, nil
}

// quoteName quotes an identifier for MySQL.
func quoteName(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// _ is a type assertion
var _ block.StoreOps = (*Store)(nil)

// pinnedTLS returns a TLS config that trusts only the certificate whose
// SHA-256 is pin, such as a server's self-signed one.
func pinnedTLS(host, pin string) *tls.Config {
	// Verify the leaf against the pin rather than a CA and the host name.
	want := strings.ToLower(strings.ReplaceAll(pin, ":", ""))
	return &tls.Config{
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // #nosec G402 -- VerifyConnection checks the pinned certificate.
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("the server sent no certificate")
			}
			sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
			if hex.EncodeToString(sum[:]) != want {
				return errors.New("the server's certificate does not match the pinned SHA-256")
			}
			return nil
		},
	}
}
