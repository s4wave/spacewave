//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/sirupsen/logrus"
)

// debugVolumeAuditArgs are the arguments of the offline liveness audit.
type debugVolumeAuditArgs struct{}

// BuildFlags returns the audit flags.
func (a *debugVolumeAuditArgs) BuildFlags() []cli.Flag {
	return nil
}

// Run audits the stopped volume and prints what the walk found.
func (a *debugVolumeAuditArgs) Run(c *cli.Context) error {
	// Audit the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	res, err := auditVolume(c.Context, logrus.NewEntry(logrus.New()), path)
	if err != nil {
		return err
	}

	// Print the findings.
	writeFields(os.Stdout, auditFields(path, res))
	return nil
}

// auditVolume audits the stopped volume at path.
func auditVolume(ctx context.Context, le *logrus.Entry, path string) (block_gc.AuditResult, error) {
	// Open the stopped volume and audit it.
	vol, err := openStoppedVolume(ctx, le, path)
	if err != nil {
		return block_gc.AuditResult{}, err
	}
	defer vol.Close()
	return vol.Audit(ctx)
}

// auditFields formats an audit in the command's field table.
func auditFields(path string, res block_gc.AuditResult) [][2]string {
	// List the counts.
	fields := [][2]string{
		{"Volume", path},
		{"Stored blocks", strconv.FormatUint(res.Stored, 10)},
		{"Reached blocks", strconv.Itoa(len(res.Mark.Live))},
		{"Reached without a child list", strconv.Itoa(res.Mark.Unknown)},
		{"Holes under a live root", strconv.Itoa(res.Mark.Absent)},
		{"Holes recorded", strconv.FormatUint(res.Holes, 10)},
		{"Leaked", strconv.FormatUint(res.Leaked, 10)},
		{"Unlisted garbage", strconv.FormatUint(res.Unlisted, 10)},
		{"Live blocks queued to free", strconv.FormatUint(res.LiveQueued, 10)},
		{"Undercounted blocks", strconv.FormatUint(res.Undercount, 10)},
		{"Cascade records less their count", strconv.FormatInt(res.CascadeMiscount, 10)},
	}

	// Name a sample of the blocks behind each finding.
	fields = appendRefSample(fields, "Live queued sample", res.LiveQueuedSample)
	fields = appendRefSample(fields, "Undercount sample", res.UndercountSample)
	fields = appendRefSample(fields, "Unlisted sample", res.UnlistedSample)
	return appendRefSample(fields, "Listless reached sample", res.Mark.UnknownSample)
}

// debugVolumeAdoptArgs are the arguments of the offline adoption.
type debugVolumeAdoptArgs struct{}

// BuildFlags returns the adopt flags.
func (a *debugVolumeAdoptArgs) BuildFlags() []cli.Flag {
	return nil
}

// Run adopts the stopped volume and prints the counts. A refusal prints the
// counts it reached before returning its error.
func (a *debugVolumeAdoptArgs) Run(c *cli.Context) error {
	// Adopt the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	res, err := adoptVolume(c.Context, logrus.NewEntry(logrus.New()), path)

	// Print the counts, then name the step a refusal waits on.
	writeFields(os.Stdout, adoptFields(path, res))
	switch {
	case errors.Is(err, block_gc.ErrAdoptUnverified):
		return errors.Wrap(err, "rebuild the child lists with debug ref-repair first")
	case errors.Is(err, block_gc.ErrAdoptUnknown):
		return errors.Wrap(err, "write the missing child lists with debug ref-repair, then adopt again")
	}
	return err
}

// adoptVolume adopts the stopped volume at path. A refusal returns the counts
// the adoption reached with its error.
func adoptVolume(ctx context.Context, le *logrus.Entry, path string) (block_gc.AdoptResult, error) {
	// Open the stopped volume and adopt it.
	vol, err := openStoppedVolume(ctx, le, path)
	if err != nil {
		return block_gc.AdoptResult{}, err
	}
	defer vol.Close()
	return vol.Adopt(ctx)
}

// adoptFields formats an adoption in the command's field table.
func adoptFields(path string, res block_gc.AdoptResult) [][2]string {
	// List the counts.
	fields := [][2]string{
		{"Volume", path},
		{"Already adopted", strconv.FormatBool(res.Already)},
		{"Stored blocks", strconv.FormatUint(res.Blocks, 10)},
		{"Named roots", strconv.FormatUint(res.Roots, 10)},
		{"Blocks with a count record", strconv.FormatUint(res.Counted, 10)},
		{"Blocks queued to free", strconv.FormatUint(res.Queued, 10)},
		{"Holders naming a listless block", strconv.FormatUint(res.Unknown, 10)},
	}

	// Name a sample of the listless blocks.
	return appendRefSample(fields, "Listless sample", res.UnknownSample)
}

// appendRefSample appends one labeled row per ref.
func appendRefSample(fields [][2]string, label string, refs []*block.BlockRef) [][2]string {
	// Add a row per ref.
	for _, ref := range refs {
		fields = append(fields, [2]string{label, ref.MarshalString()})
	}
	return fields
}

// newDebugVolumeAuditCommand returns the offline liveness audit command.
func newDebugVolumeAuditCommand() *cli.Command {
	args := &debugVolumeAuditArgs{}
	return &cli.Command{
		Name:      "volume-audit",
		Usage:     "walk a stopped volume from its named roots and compare the walk with its counts",
		ArgsUsage: "<volume-file>",
		Description: "Walks every block a named root reaches through child lists in one snapshot, and compares the walk " +
			"with the counts and the free queue the collector keeps. Live blocks queued to free and undercounted blocks " +
			"are errors in the counts that would free a live block; leaked and unlisted blocks are garbage the collector " +
			"does not free. It frees nothing, but records each hole under a live root that no record names yet, so a " +
			"later put of that block counts it as stored. It keeps the reached set in memory, so run it on a copy host. " +
			"Run it only on a stopped volume.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// newDebugVolumeAdoptCommand returns the offline adoption command.
func newDebugVolumeAdoptCommand() *cli.Command {
	args := &debugVolumeAdoptArgs{}
	return &cli.Command{
		Name:      "volume-adopt",
		Usage:     "count the holders of every block of a stopped, unadopted volume and mark it adopted",
		ArgsUsage: "<volume-file>",
		Description: "Counts each block's holders from the volume's named roots and child lists alone, writes the count " +
			"records and the free queue, and marks the volume adopted, after which its collector nets the journal and, " +
			"with frees on, frees what no root reaches. It refuses while the child lists come from a store format " +
			"before 7 until debug ref-repair verifies them, and refuses when a holder names a block stored without a " +
			"child list. An adopted volume reports Already adopted and changes nothing. Adopt a volume only once every " +
			"writer that uses it holds what it needs under named roots, and run it only on a stopped volume.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
