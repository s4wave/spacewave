// Command spacewave-world-upload uploads a directory of packs in the flat
// adopt layout ({id} and {id}.entry, as spacewave-world-repack writes it) to
// an S3-compatible bucket, with PUT requests alone.
//
// For each id, in order, it PUTs the pack at {prefix}packs/{id} and, only once
// that PUT returns 200, the entry at {prefix}entries/{id}. It stops at the
// first failure: a non-200 status, a 200 whose echoed sha256 is not the one
// sent, or a failed request ends the run, with no retry and no further
// object.
//
// Before the first request it checks the whole plan locally: every pack has
// its entry and every entry its pack, each entry names its pack's file,
// size and block count, holds the v1 bloom filter of the pack's keys, each
// id is the v1 id of its pack's bytes, each id is in the --allow-list, and
// each object key matches ^{prefix}(packs|entries)/pfv1_[base58]+$.
//
// Each PUT carries Content-MD5 and x-amz-checksum-sha256, so the service
// verifies the body, and is signed with SigV4 by this package. With
// --if-none-match, the default, it also carries a signed If-None-Match: *,
// so the service refuses with 412 to overwrite a key that exists; a 412
// stops the run. The command sends no other method: it never reads, lists
// or deletes an object.
//
// The run holds an exclusive flock on the --manifest from start to end, and
// refuses to start when another run holds it. Each object put appends one
// fsynced line to the manifest TSV:
//
//	time  key  size  sha256-hex  md5-hex  status
//
// The status is the response code, or 200-badsum for a 200 whose echoed
// sha256 is not the one sent. With --resume, a pack or entry whose line
// holds status 200, with the size and sha256 of the local file, is not put
// again; an id whose pack and entry both are is skipped. A torn last line,
// one without its newline, is dropped with a warning on stderr and cut off
// the file.
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
	o := &options{Log: os.Stdout, Warn: os.Stderr}
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
		&cli.BoolFlag{Name: "resume", Usage: "do not put again a pack or entry the manifest records with status 200", Destination: &o.Resume},
		&cli.BoolFlag{Name: "if-none-match", Usage: "send a signed If-None-Match: * on every PUT, so a key the bucket holds is not overwritten; a 412 stops the run", Value: true, Destination: &o.IfNoneMatch},
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
