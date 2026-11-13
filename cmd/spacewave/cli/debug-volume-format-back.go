//go:build !js

package spacewave_cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// formatBackVersion is the format the way back writes: version 9 code opens a
// version 8 store unadopted and recounts every child list, so it reads a
// version 10 store's lists and roots and clears its other liveness records.
const formatBackVersion = 8

// debugVolumeFormatBackArgs are the arguments of the volume-format-back
// command.
type debugVolumeFormatBackArgs struct{}

// newDebugVolumeFormatBackCommand builds the command that lets the previous
// release open a stopped volume.
func newDebugVolumeFormatBackCommand() *cli.Command {
	args := &debugVolumeFormatBackArgs{}
	return &cli.Command{
		Name:      "volume-format-back",
		Usage:     "set a stopped volume's store format back so the previous release opens it",
		ArgsUsage: "<volume-file>",
		Description: fmt.Sprintf("Sets the store format of a stopped volume at version %d to %d in one write. The previous "+
			"release, which refuses version %d as newer, then opens it unadopted and recounts every child list in the "+
			"background, freeing nothing until the recount finishes. The volume must be closed by every other process.",
			store_kvkey.FormatVersion, formatBackVersion, store_kvkey.FormatVersion),
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the command's flags, of which it has none.
func (a *debugVolumeFormatBackArgs) BuildFlags() []cli.Flag {
	return nil
}

// Run sets the format of the volume at the argument's path back.
func (a *debugVolumeFormatBackArgs) Run(c *cli.Context) error {
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	return formatBack(c.Context, c.Args().First(), os.Stdout)
}

// formatBack sets the store format of the stopped volume at path from the
// current version to formatBackVersion and reports it to w. It refuses a
// volume another process has open or one at any other version.
func formatBack(ctx context.Context, path string, w io.Writer) error {
	// Open the file, refusing a missing one, which the open would create, and
	// one a daemon has open.
	if _, err := os.Stat(path); err != nil {
		return err
	}
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		return err
	}
	defer db.Close()
	shared, err := db.Shared()
	if err != nil {
		return err
	}
	if shared {
		return errors.Errorf("%s is open in another process; stop the daemon first", path)
	}

	// Start the write.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Read the version and refuse any but the current one.
	key := store_kvkey.NewDefaultKVKey().GetFormatVersionKey()
	data, found, err := tx.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		return errors.Errorf("%s has no store format version", path)
	}
	version, err := store_kvkey.ParseFormatVersion(data)
	if err != nil {
		return err
	}
	if version != store_kvkey.FormatVersion {
		return errors.Errorf("%s is at store format %d; only %d goes back", path, version, store_kvkey.FormatVersion)
	}

	// Write the old version.
	if err := tx.Set(ctx, key, store_kvkey.MarshalFormatVersion(formatBackVersion)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: store format %d set to %d\n", path, version, formatBackVersion)
	return nil
}
