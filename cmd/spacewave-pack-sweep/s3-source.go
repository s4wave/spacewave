package main

import (
	"context"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

const (
	// s3AccessKeyEnv names the variable holding the S3 access key id.
	s3AccessKeyEnv = "AWS_ACCESS_KEY_ID"
	// s3SecretKeyEnv names the variable holding the S3 secret key.
	s3SecretKeyEnv = "AWS_SECRET_ACCESS_KEY" //nolint:gosec // the name of a variable, not a key.
	// s3TokenEnv names the variable holding an S3 session token, if any.
	s3TokenEnv = "AWS_SESSION_TOKEN" //nolint:gosec // the name of a variable, not a token.
)

// errReadOnly refuses a change to a store the snapshot only reads.
var errReadOnly = errors.New("the S3 source is read-only: the snapshot changes nothing in the bucket")

// s3Flags binds the flags that name an S3 bucket as the snapshot's source.
func (a *Args) s3Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "s3-endpoint", Usage: "S3 endpoint host, or an http:// or https:// origin, read instead of SFTP; keys come from " + s3AccessKeyEnv + " and " + s3SecretKeyEnv, Destination: &a.S3Endpoint},
		&cli.StringFlag{Name: "s3-region", Usage: "S3 region of the bucket", Destination: &a.S3Region},
		&cli.StringFlag{Name: "s3-bucket", Usage: "S3 bucket holding the prefix", Destination: &a.S3Bucket},
	}
}

// usesS3 reports whether the flags name an S3 source.
func (a *Args) usesS3() bool {
	return a.S3Endpoint != "" || a.S3Bucket != ""
}

// dialS3 reaches the bucket the S3 flags name through a client that refuses
// every change, so a snapshot over S3 only reads.
func (a *Args) dialS3() (block_store_s3.ObjectStore, string, func(), error) {
	// Refuse a source named twice or by halves.
	if a.SFTPAddr != "" {
		return nil, "", nil, errors.New("name the store by --sftp-addr or by --s3-endpoint, not both")
	}
	if a.S3Endpoint == "" || a.S3Bucket == "" {
		return nil, "", nil, errors.New("an S3 source needs --s3-endpoint and --s3-bucket")
	}
	access, secret := os.Getenv(s3AccessKeyEnv), os.Getenv(s3SecretKeyEnv)
	if access == "" || secret == "" {
		return nil, "", nil, errors.Errorf("an S3 source needs %s and %s set", s3AccessKeyEnv, s3SecretKeyEnv)
	}

	// Build the client, plain HTTP only for an http:// origin.
	host, plain := strings.CutPrefix(a.S3Endpoint, "http://")
	host = strings.TrimSuffix(strings.TrimPrefix(host, "https://"), "/")
	client, err := block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:   host,
		Region:     a.S3Region,
		DisableSsl: plain,
		Credentials: &block_store_s3.Credentials{
			AccessKeyId:     access,
			SecretAccessKey: secret,
			Token:           os.Getenv(s3TokenEnv),
		},
	})
	if err != nil {
		return nil, "", nil, err
	}
	return readOnlyObjects{ObjectStore: client}, a.S3Bucket, func() {}, nil
}

// readOnlyObjects serves the reads of an object store and refuses its writes
// and deletes. It is no ObjectMover, so the steps that move packs refuse it.
type readOnlyObjects struct {
	block_store_s3.ObjectStore
}

// PutObject refuses the write.
func (readOnlyObjects) PutObject(context.Context, string, string, []byte, string) error {
	return errReadOnly
}

// DeleteObject refuses the delete.
func (readOnlyObjects) DeleteObject(context.Context, string, string) error {
	return errReadOnly
}

// checkSFTP refuses a snapshot that names neither source in full, since the
// snapshot's SFTP flags are optional beside the S3 ones.
func (a *Args) checkSFTP() error {
	if a.SFTPAddr == "" || a.SFTPUser == "" || a.SFTPHostKey == "" || a.SFTPRoot == "" {
		return errors.New("name the store by --sftp-addr, --sftp-user, --sftp-host-key and --sftp-root, or by --s3-endpoint and --s3-bucket")
	}
	return nil
}
