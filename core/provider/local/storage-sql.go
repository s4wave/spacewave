package provider_local

import (
	"context"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sql "github.com/s4wave/spacewave/db/block/store/sql"
	"github.com/sirupsen/logrus"
)

// CheckSQLLocation creates the location's blocks table when missing, then
// writes and reads back a probe block, and classifies the first failure.
func CheckSQLLocation(
	ctx context.Context,
	location *account_settings.SqlLocation,
	creds *block_store_s3.Credentials,
) *block_store_s3.CheckResult {
	return block_store_sql.Check(ctx, sqlConfig(location, creds))
}

// buildSQLBlockStore opens the block store's rows in the location's blocks
// table, creating the table when missing.
func buildSQLBlockStore(
	ctx context.Context,
	le *logrus.Entry,
	location *account_settings.SqlLocation,
	blockStoreID string,
	creds *block_store_s3.Credentials,
) (backendStore, error) {
	// Open the database and create its blocks table if missing.
	conf := sqlConfig(location, creds)
	db, err := block_store_sql.Open(conf)
	if err != nil {
		return nil, err
	}
	if err := block_store_sql.EnsureTable(ctx, db, conf.Table); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Scope the store to this block store's rows.
	return block_store_sql.NewStore(le, db, conf.Table, blockStoreID), nil
}

// buildSQLFallbackStore opens the block store's rows in the location's blocks
// table for reads. It neither connects nor creates the table, so an
// unreachable database fails the reads that reach it, not the mount.
func buildSQLFallbackStore(
	le *logrus.Entry,
	location *account_settings.SqlLocation,
	blockStoreID string,
	creds *block_store_s3.Credentials,
) (backendStore, error) {
	// Open the lazy connection pool without touching the database.
	conf := sqlConfig(location, creds)
	db, err := block_store_sql.Open(conf)
	if err != nil {
		return nil, err
	}
	return block_store_sql.NewStore(le, db, conf.Table, blockStoreID), nil
}

// sqlConfig returns the store config of the location with the credentials,
// whose access key id is the user and secret access key the password.
func sqlConfig(location *account_settings.SqlLocation, creds *block_store_s3.Credentials) *block_store_sql.Config {
	return &block_store_sql.Config{
		Host:     location.GetHost(),
		Port:     location.GetPort(),
		Database: location.GetDatabase(),
		User:     creds.GetAccessKeyId(),
		Password: creds.GetSecretAccessKey(),
		Table:    location.BlocksTable(),
		TLS:      location.GetTls(),

		TLSCertSHA256: location.GetTlsCertSha256(),
		MaxConns:      int(sqlMaxConns.Load()),
	}
}
