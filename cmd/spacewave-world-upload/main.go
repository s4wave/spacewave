// Command spacewave-world-upload uploads a directory of packs in the flat
// adopt layout ({id} and {id}.entry, as spacewave-world-repack writes it) to
// an S3-compatible bucket, with PUT requests alone.
//
// For each id, in order, it PUTs the pack at {prefix}packs/{id} and, only once
// that PUT returns 200, the entry at {prefix}entries/{id}. It stops at the
// first failure: a non-200 status or a failed request ends the run, with no
// retry and no further object.
//
// Before the first request it checks the whole plan locally: every pack has
// its entry and every entry its pack, each entry names its pack's file and
// size, each id is the v1 id of its pack's bytes, each id is in the
// --allow-list, and each object key matches
// ^{prefix}(packs|entries)/pfv1_[base58]+$.
//
// Each PUT carries Content-MD5 and x-amz-checksum-sha256, so the service
// verifies the body, and is signed with SigV4 by this package. The command
// sends no other method: it never reads, lists or deletes an object.
//
// Each object put appends one fsynced line to the --manifest TSV:
//
//	time  key  size  sha256-hex  md5-hex  status
//
// With --resume, an id whose pack and entry lines both hold status 200, with
// the size and sha256 of the local files, is skipped.
//
// The --creds file must be a regular file of mode 0600 owned by the user. It
// holds two key=value lines; blank lines and lines starting with # are
// ignored:
//
//	access_key_id=...
//	secret_access_key=...
//
// Its contents are never printed.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// defaultResourceID is the resource the pack ids are scoped to, as the
// re-pack tool names them.
const defaultResourceID = "spacewave-world-repack"

func main() {
	// Declare the command and its flags.
	o := &options{Log: os.Stdout}
	app := cli.NewApp()
	app.Name = "spacewave-world-upload"
	app.Usage = "PUT adopt-layout packs and entries to an S3 bucket, pack before entry, verified, with no other method"
	app.Flags = []cli.Flag{
		&cli.StringFlag{Name: "dir", Usage: "directory in the adopt layout, {id} and {id}.entry; only read", Destination: &o.Dir, Required: true},
		&cli.StringFlag{Name: "endpoint", Usage: "https URL of the S3 endpoint, path-style", Destination: &o.Endpoint, Required: true},
		&cli.StringFlag{Name: "region", Usage: "SigV4 region of the endpoint", Destination: &o.Region, Required: true},
		&cli.StringFlag{Name: "bucket", Usage: "bucket name", Destination: &o.Bucket, Required: true},
		&cli.StringFlag{Name: "prefix", Usage: "key prefix, ending in /; objects go to {prefix}packs/{id} and {prefix}entries/{id}", Destination: &o.Prefix, Required: true},
		&cli.StringFlag{Name: "creds", Usage: "file of mode 0600 owned by the user, with access_key_id= and secret_access_key= lines; not needed with --dry-run", Destination: &o.CredsPath},
		&cli.StringFlag{Name: "manifest", Usage: "output TSV, one fsynced line per object put; must be absent or empty unless --resume", Destination: &o.Manifest, Required: true},
		&cli.StringFlag{Name: "allow-list", Usage: "file of the ids allowed, one per line, the re-pack's output list; any other id in the plan is refused", Destination: &o.AllowList, Required: true},
		&cli.StringFlag{Name: "ids", Usage: "file of ids to upload, one per line, in that order; default every id of --dir, byte-sorted", Destination: &o.IDs},
		&cli.StringFlag{Name: "resource-id", Usage: "resource the v1 pack ids are scoped to", Value: defaultResourceID, Destination: &o.ResourceID},
		&cli.BoolFlag{Name: "resume", Usage: "skip ids whose pack and entry are both in the manifest with status 200", Destination: &o.Resume},
		&cli.BoolFlag{Name: "dry-run", Usage: "check everything and print the plan; send no request", Destination: &o.DryRun},
	}
	app.Action = func(*cli.Context) error {
		_, err := run(o)
		return err
	}

	// Run it; any error exits non-zero.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
