package main

import (
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// copyArgs configures the version copy.
type copyArgs struct {
	// Endpoint is the S3 endpoint host both buckets are on, reached over
	// HTTPS.
	Endpoint string
	// Region is the buckets' region.
	Region string
	// From is the source bucket.
	From string
	// To is the destination bucket.
	To string
	// Prefixes are the source key prefixes.
	Prefixes cli.StringSlice
	// ToPrefix starts every destination key.
	ToPrefix string
	// KeyIDEnv names the variable holding the access key id.
	KeyIDEnv string
	// SecretEnv names the variable holding the secret access key.
	SecretEnv string
	// DryRun counts what the copy would write, writing nothing.
	DryRun bool

	// client reaches the buckets; nil dials Endpoint. Tests set it.
	client *block_store_s3.Client
}

// copyFlags binds the flags of the version copy.
func (a *Args) copyFlags() []cli.Flag {
	c := &a.versionCopy
	return []cli.Flag{
		&cli.StringFlag{Name: "s3-endpoint", Usage: "S3 endpoint host of both buckets, reached over HTTPS", Destination: &c.Endpoint, Required: true},
		&cli.StringFlag{Name: "s3-region", Usage: "the buckets' region", Destination: &c.Region},
		&cli.StringFlag{Name: "from-bucket", Usage: "source bucket", Destination: &c.From, Required: true},
		&cli.StringFlag{Name: "to-bucket", Usage: "destination bucket", Destination: &c.To, Required: true},
		&cli.StringSliceFlag{Name: "prefix", Usage: "source key prefix to copy; repeat for each", Destination: &c.Prefixes, Required: true},
		&cli.StringFlag{Name: "to-prefix", Usage: "prefix of every destination key, before the source key", Destination: &c.ToPrefix},
		&cli.StringFlag{Name: "key-id-env", Usage: "name of the variable holding the access key id, which must read the source and write the destination", Value: s3KeyIDEnv, Destination: &c.KeyIDEnv},
		&cli.StringFlag{Name: "secret-env", Usage: "name of the variable holding the secret access key", Value: s3SecretEnv, Destination: &c.SecretEnv},
		&cli.BoolFlag{Name: "dry-run", Usage: "list and count what the copy would write, writing nothing", Destination: &c.DryRun},
	}
}

// runCopyVersions copies every version and delete marker under the prefixes
// from one bucket to another, each key's oldest first, server-side, and
// prints the counts. It refuses a destination without versioning and one the
// source prefixes overlap in the same bucket, before any write.
func (a *Args) runCopyVersions(*cli.Context) error {
	// Run until a signal stops the copy.
	ctx, cancel := signalContext()
	defer cancel()
	c := &a.versionCopy

	// Reach the buckets with the key the named variables hold.
	client := c.client
	if client == nil {
		keyID, secret := os.Getenv(c.KeyIDEnv), os.Getenv(c.SecretEnv)
		if keyID == "" || secret == "" {
			return errors.New("set the variables --key-id-env and --secret-env name")
		}
		var err error
		client, err = block_store_s3.BuildClient(&block_store_s3.ClientConfig{
			Endpoint:    c.Endpoint,
			Region:      c.Region,
			Credentials: &block_store_s3.Credentials{AccessKeyId: keyID, SecretAccessKey: secret},
		})
		if err != nil {
			return err
		}
	}

	// Copy, and print the counts.
	le := logrus.NewEntry(logrus.StandardLogger()).WithFields(logrus.Fields{"from": c.From, "to": c.To, "dry_run": c.DryRun})
	le.Info("copying versions")
	res, err := block_store_s3.CopyVersions(ctx, &block_store_s3.VersionCopyOptions{
		Client:     client,
		FromBucket: c.From,
		ToBucket:   c.To,
		Prefixes:   c.Prefixes.Value(),
		ToPrefix:   c.ToPrefix,
		DryRun:     c.DryRun,
	})
	if res != nil {
		if perr := a.print(func(ar *fastjson.Arena) *fastjson.Value { return copyResultJSON(ar, res, c.DryRun) }); perr != nil && err == nil {
			err = perr
		}
	}
	if err == nil && res.Conflicts != 0 {
		err = errors.Errorf("%d keys hold another history in the destination and were left alone", res.Conflicts)
	}
	return err
}

// copyResultJSON describes a version copy's counts.
func copyResultJSON(a *fastjson.Arena, r *block_store_s3.VersionCopyResult, dryRun bool) *fastjson.Value {
	// Write the counts.
	obj := a.NewObject()
	if dryRun {
		obj.Set("dry_run", a.NewTrue())
	} else {
		obj.Set("dry_run", a.NewFalse())
	}
	for _, f := range []struct {
		name string
		n    uint64
	}{
		{"keys", r.Keys}, {"versions", r.Versions}, {"markers", r.Markers},
		{"bytes", r.Bytes}, {"largest", r.Largest}, {"skipped", r.Skipped},
		{"resumed", r.Resumed}, {"conflicts", r.Conflicts},
	} {
		obj.Set(f.name, a.NewNumberString(strconv.FormatUint(f.n, 10)))
	}

	// List the first conflicting keys.
	keys := a.NewArray()
	for i, key := range r.ConflictKeys {
		keys.SetArrayItem(i, a.NewString(key))
	}
	obj.Set("conflict_keys", keys)
	return obj
}
