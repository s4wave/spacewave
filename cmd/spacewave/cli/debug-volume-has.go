//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// newDebugVolumeHasCommand builds the offline block lookup command.
func newDebugVolumeHasCommand() *cli.Command {
	return &cli.Command{
		Name:      "volume-has",
		Usage:     "report which of a list of block IDs a volume file stores",
		ArgsUsage: "<volume-file> <id-file>",
		Description: "Reads block IDs in their base58 form, one per line, from the ID file, or from standard input " +
			"when it is -, and looks each up in one read snapshot of an s4db volume file. It changes no key or value and " +
			"may run beside the daemon that has the volume open, but the file is not left byte for byte the same: the " +
			"open takes a reader slot and the close may release freed space. Copy the file first to keep it unchanged. " +
			"Each output line is \"present\" or \"absent\" and the ID; the counts follow on standard error. Blank " +
			"lines and lines starting with # are skipped.",
		Action: func(c *cli.Context) error {
			// Check the arguments.
			if c.NArg() != 2 {
				return errors.New("expected the volume file and the ID file")
			}

			// Read the IDs from the file, or from standard input.
			var ids io.Reader = os.Stdin
			if name := c.Args().Get(1); name != "-" {
				f, err := os.Open(name)
				if err != nil {
					return err
				}
				defer f.Close()
				ids = f
			}

			// Look them up and report the counts.
			present, absent, err := runDebugVolumeHas(c.Context, c.Args().Get(0), ids, os.Stdout)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "%d present, %d absent\n", present, absent)
			return nil
		},
	}
}

// runDebugVolumeHas looks up each block ID read from ids in one read snapshot
// of the volume file at path, writing a line for each to w. It returns the
// number present and absent.
func runDebugVolumeHas(ctx context.Context, path string, ids io.Reader, w io.Writer) (present, absent int, err error) {
	// Open the file beside any daemon that has it open, refusing a missing
	// file, which the open would create, and without compaction, which
	// would move values.
	if _, err := os.Stat(path); err != nil {
		return 0, 0, err
	}
	db, err := s4db.Open(path, s4db.Options{RelocateBudget: -1})
	if err != nil {
		return 0, 0, fmt.Errorf("open volume: %w", err)
	}
	defer db.Close()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Discard()

	// Refuse format 0, which keys each block by its base58 text.
	keys := store_kvkey.NewDefaultKVKey()
	data, found, err := tx.Get(ctx, keys.GetFormatVersionKey())
	if err != nil {
		return 0, 0, err
	}
	var version uint64
	if found {
		if version, err = store_kvkey.ParseFormatVersion(data); err != nil {
			return 0, 0, err
		}
	}
	if version == 0 || version > store_kvkey.FormatVersion {
		return 0, 0, fmt.Errorf("lookup needs store format version 1 to %d, found %d", store_kvkey.FormatVersion, version)
	}

	// Look up each ID in the snapshot.
	scanner := bufio.NewScanner(ids)
	for line := 1; scanner.Scan(); line++ {
		// Parse the ID, skipping blank lines and comments.
		id := strings.TrimSpace(scanner.Text())
		if id == "" || strings.HasPrefix(id, "#") {
			continue
		}
		ref, err := block.UnmarshalBlockRefB58(id)
		if err != nil {
			return present, absent, fmt.Errorf("line %d: %w", line, err)
		}
		refKey, err := ref.MarshalKey()
		if err != nil {
			return present, absent, fmt.Errorf("line %d: %w", line, err)
		}

		// Report whether the volume stores its block.
		ok, err := tx.Exists(ctx, keys.GetBlockKey(refKey))
		if err != nil {
			return present, absent, err
		}
		state := "absent"
		if ok {
			state = "present"
			present++
		} else {
			absent++
		}
		if _, err := fmt.Fprintln(w, state, id); err != nil {
			return present, absent, err
		}
	}
	return present, absent, scanner.Err()
}
