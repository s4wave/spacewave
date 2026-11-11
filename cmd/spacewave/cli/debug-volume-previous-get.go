//go:build !js

package spacewave_cli

import (
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/s4db/s4dbv1"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// debugVolumePreviousGetArgs are the arguments of the previous-format lookup.
type debugVolumePreviousGetArgs struct {
	// blocks are the base58 block ids to look up.
	blocks cli.StringSlice
}

// BuildFlags returns the lookup flags.
func (a *debugVolumePreviousGetArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:        "block",
			Usage:       "a base58 block id to look up under each key form a store has used",
			Destination: &a.blocks,
		},
	}
}

// previousGetKey is one key to look up and how to label it.
type previousGetKey struct {
	// label names the key in the output.
	label string
	// form is the key form: text, or the block key's encoding.
	form string
	// key is the stored key.
	key []byte
}

// Run opens the file read-only, prints its checkpoint, and looks up each key.
func (a *debugVolumePreviousGetArgs) Run(c *cli.Context) error {
	// Build the keys: each argument as text, and each block under the key of
	// its ref bytes (store format 1 on) and of its base58 text (format 0).
	if c.NArg() < 1 {
		return errors.New("expected a volume file argument and keys")
	}
	var keys []previousGetKey
	for _, k := range c.Args().Tail() {
		keys = append(keys, previousGetKey{label: k, form: "text", key: []byte(k)})
	}
	kvkey := store_kvkey.NewDefaultKVKey()
	for _, id := range a.blocks.Value() {
		ref, err := b58.Decode(id)
		if err != nil {
			return errors.Wrapf(err, "block %s", id)
		}
		keys = append(keys,
			previousGetKey{label: id, form: "ref bytes", key: kvkey.GetBlockKey(ref)},
			previousGetKey{label: id, form: "base58", key: kvkey.GetBlockKey([]byte(id))},
		)
	}

	// Open read-only and print the checkpoint before the first lookup
	// replays the log after it.
	r, err := s4dbv1.Open(c.Args().First())
	if err != nil {
		return err
	}
	defer r.Close()
	cp := r.Checkpoint()
	writeFields(os.Stdout, [][2]string{
		{"Checkpoint commit", strconv.FormatUint(cp.Seq, 10)},
		{"Checkpoint keys", strconv.FormatUint(cp.Keys, 10)},
		{"Tree pages", strconv.FormatUint(cp.TreePages, 10)},
		{"First log run bytes", strconv.FormatUint(cp.LogRunBytes, 10)},
	})
	if len(keys) == 0 {
		return nil
	}

	// Look up each key.
	rows := [][]string{{"KEY", "FORM", "FOUND", "BYTES"}}
	for _, k := range keys {
		v, ok, err := r.Get(k.key)
		if err != nil {
			return errors.Wrapf(err, "get %s", k.label)
		}
		n := ""
		if ok {
			n = strconv.Itoa(len(v))
		}
		rows = append(rows, []string{k.label, k.form, strconv.FormatBool(ok), n})
	}
	os.Stdout.WriteString("\n")
	writeTable(os.Stdout, "", rows)
	return nil
}

// newDebugVolumePreviousGetCommand returns the previous-format lookup command.
func newDebugVolumePreviousGetCommand() *cli.Command {
	args := &debugVolumePreviousGetArgs{}
	return &cli.Command{
		Name:      "volume-previous-get",
		Usage:     "look up keys and blocks in a previous-format volume file without changing it",
		ArgsUsage: "<volume-file> [key...]",
		Description: "Opens a volume file the previous store engine wrote only for reading, never converting it, and " +
			"prints its checkpoint. Then looks up each key argument as text and each --block under both block key " +
			"forms, printing whether the file holds it and the value's size. The first lookup replays the log after " +
			"the checkpoint; each lookup then reads one page per tree level. It may run beside other readers; a " +
			"writer or a conversion of the file refuses it.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
