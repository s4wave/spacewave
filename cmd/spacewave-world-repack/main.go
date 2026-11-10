// Command spacewave-world-repack writes the blocks of one or more pack sets
// in the flat adopt layout ({id} and {id}.entry, as hyperline-backend
// world-adopt-packs reads) into fewer, larger packs in a fresh directory in
// the same layout.
//
// The inputs are only read. Every block is verified against its ref as it is
// read, and a ref held by several input packs is written once. The packs are
// written by the PackStore's own packing code, writer.PackBlocks, and named
// by identity.BuildPackID from their bytes, so the same inputs give the same
// output bytes. After writing, the output is read back from disk and proven
// to hold exactly the input's blocks, each once, with every entry matching
// its pack. A slice plan groups the output packs into upload slices.
package main

import (
	"math"
	"os"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
)

// maxPackMiB is the largest --pack-mib.
const maxPackMiB = 128

// defaultMaxBlocks is the default block limit of an output pack. Its index
// tail is about 2 MB, which a reader loads whole for existence checks, and it
// stays under the PackStore's batch limit of 33,288 blocks at 128 MiB.
const defaultMaxBlocks = 32768

// defaultResourceID is the resource the pack ids are scoped to.
const defaultResourceID = "spacewave-world-repack"

// args are the command's flags.
type args struct {
	in                []string
	out, resourceID   string
	createdAt         string
	packMiB, maxBlock uint64
	sliceGB           float64
	allowExtra        bool
}

func main() {
	// Declare the command and its flags.
	a := &args{}
	app := cli.NewApp()
	app.Name = "spacewave-world-repack"
	app.Usage = "re-pack adopt-layout pack sets into fewer, larger packs, deterministically, and prove the output equal"
	app.Flags = []cli.Flag{
		&cli.StringSliceFlag{Name: "in", Usage: "input directory in the adopt layout, {id} and {id}.entry; repeat for several, merged into one id-sorted list; only read", Required: true},
		&cli.StringFlag{Name: "out", Usage: "output directory; must not exist or be empty", Destination: &a.out, Required: true},
		&cli.Uint64Flag{Name: "pack-mib", Usage: "largest output pack file, in MiB, at most 128", Value: maxPackMiB, Destination: &a.packMiB},
		&cli.Uint64Flag{Name: "max-blocks", Usage: "most blocks in one output pack; a pack is cut at this or --pack-mib, whichever comes first; 0 is no limit", Value: defaultMaxBlocks, Destination: &a.maxBlock},
		&cli.Float64Flag{Name: "slice-gb", Usage: "most GB (10^9 bytes) of files, pack plus entry, in one slice of the printed slice plan", Value: 1.0, Destination: &a.sliceGB},
		&cli.BoolFlag{Name: "allow-extra", Usage: "skip input files that are not pack and entry pairs (.part files, subdirectories, symlinks, strays), counted; a pack without its entry or an entry without its pack always fails", Destination: &a.allowExtra},
		&cli.StringFlag{Name: "resource-id", Usage: "resource the v1 pack ids are scoped to", Value: defaultResourceID, Destination: &a.resourceID},
		&cli.StringFlag{Name: "created-at", Usage: "RFC 3339 created-at of every output entry; default the latest created-at of the input entries", Destination: &a.createdAt},
	}
	app.Action = func(c *cli.Context) error {
		a.in = c.StringSlice("in")
		return a.run()
	}

	// Run it; any error, a failed proof included, exits non-zero.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// run checks the flags and runs the re-pack.
func (a *args) run() error {
	// Refuse a pack size past the limit and an empty slice.
	if a.packMiB == 0 || a.packMiB > maxPackMiB {
		return errors.Errorf("--pack-mib must be 1 to %d", maxPackMiB)
	}
	if math.IsNaN(a.sliceGB) || math.IsInf(a.sliceGB, 0) || a.sliceGB <= 0 || uint64(a.sliceGB*1e9) == 0 {
		return errors.New("--slice-gb must be a finite number above 0")
	}

	// Build the options and run.
	o := &options{
		In:         a.in,
		Out:        a.out,
		PackBytes:  int64(a.packMiB) << 20, //nolint:gosec // at most 128.
		MaxBlocks:  a.maxBlock,
		SliceBytes: uint64(a.sliceGB * 1e9),
		ResourceID: a.resourceID,
		AllowExtra: a.allowExtra,
		Log:        os.Stdout,
	}
	if a.createdAt != "" {
		t, err := time.Parse(time.RFC3339Nano, a.createdAt)
		if err != nil {
			return errors.Wrap(err, "--created-at")
		}
		o.CreatedAt = &t
	}
	_, err := run(o)
	return err
}
