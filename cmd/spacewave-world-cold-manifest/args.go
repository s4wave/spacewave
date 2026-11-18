//go:build !js && !tinygo && !wasip1

package main

import (
	"bufio"
	"encoding/hex"
	stderrors "errors"
	"io"
	"iter"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// Args names the local export, its sorted key stream and a new manifest file.
type Args struct {
	// Source is the mirror directory containing XNN/packs and XNN/entries.
	Source string
	// Keys lists the exported SHA-256 digests or encoded pack keys in hex order.
	Keys string
	// Output is the path of the new manifest protobuf.
	Output string
}

// BuildFlags binds the local inputs. The command accepts no bucket credentials.
func (a *Args) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "source", Usage: "local directory mirroring the cold prefix", Required: true, Destination: &a.Source},
		&cli.StringFlag{Name: "keys", Usage: "strictly sorted exported keys, one SHA-256 digest or encoded pack key in hex per line", Required: true, Destination: &a.Keys},
		&cli.StringFlag{Name: "output", Usage: "new manifest file (must not already exist)", Required: true, Destination: &a.Output},
	}
}

// Run verifies the export and publishes its manifest only on success.
func (a *Args) Run(c *cli.Context) error {
	// Stream the independently exported key list into the local verifier.
	ctx, cancel := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	keys, err := os.Open(a.Keys)
	if err != nil {
		return err
	}
	defer keys.Close() // Read-only export file.
	manifest, err := block_store_s3.BuildColdPackManifest(ctx, a.Source, exportKeys(keys))
	if err != nil {
		return err
	}

	// Check cancellation before the only output write.
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := manifest.MarshalVT()
	if err != nil {
		return err
	}
	return publishManifest(a.Output, data)
}

// exportKeys decodes the input without holding the exported key set in memory.
func exportKeys(r io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		// Decode one key at a time. The verifier enforces strict canonical order.
		scanner := bufio.NewScanner(r)
		for line := 1; scanner.Scan(); line++ {
			// A bare digest uses the same hash encoding the pack writer uses.
			text := scanner.Bytes()
			key, err := hex.DecodeString(string(text))
			if err != nil {
				yield(nil, errors.Wrapf(err, "exported key at line %d is not hex", line))
				return
			}
			if len(key) == 32 {
				key = packfile.BlockKey(hash.NewHash(hash.RecommendedHashType, key))
			}
			if !yield(key, nil) {
				return
			}
		}

		// A partial read is never a successful end of the export.
		if err := scanner.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// publishManifest exposes the complete file atomically without replacing an old one.
func publishManifest(path string, data []byte) (err error) {
	// Stage in the target directory so publishing cannot cross file systems.
	f, err := os.CreateTemp(filepath.Dir(path), ".cold-manifest-*")
	if err != nil {
		return err
	}
	defer func() { err = stderrors.Join(err, os.Remove(f.Name())) }()

	// Fence the complete contents; on failure the destination stays absent.
	if _, err := f.Write(data); err != nil {
		return stderrors.Join(err, f.Close())
	}
	if err := f.Sync(); err != nil {
		return stderrors.Join(err, f.Close())
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Link fails if a manifest already exists, rather than replacing it.
	return os.Link(f.Name(), path)
}
