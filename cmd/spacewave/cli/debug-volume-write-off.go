//go:build !js

package spacewave_cli

import (
	"context"
	stderrors "errors"
	"io"
	"os"
	"slices"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/sirupsen/logrus"
)

// writeOffRefsBatch is how many refs of the list one read group holds.
const writeOffRefsBatch = 1024

// debugVolumeWriteOffArgs are the arguments of the offline hole write-off.
type debugVolumeWriteOffArgs struct {
	// refs is the file of the block refs to write off, empty to list the
	// holes only.
	refs string
	// holesOut is the optional output of every recorded hole.
	holesOut string
	// upgrade decides whether the open upgrades the volume's format.
	upgrade *FormatUpgrade
}

// BuildFlags returns the write-off flags and the shared format upgrade flag.
func (a *debugVolumeWriteOffArgs) BuildFlags() []cli.Flag {
	return append([]cli.Flag{
		&cli.StringFlag{
			Name:        "refs",
			Usage:       "file of the B58 block refs to write off, one per line, optionally followed by a tab and notes",
			Destination: &a.refs,
		},
		&cli.StringFlag{
			Name:        "holes-out",
			Usage:       "write each recorded hole's ref, a tab, and open or written-off to this file",
			Destination: &a.holesOut,
		},
	}, a.upgrade.BuildFlags()...)
}

// Run writes off the named holes of the stopped volume and prints the counts.
func (a *debugVolumeWriteOffArgs) Run(c *cli.Context) error {
	// Write off the holes of the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	res, err := a.writeOff(c.Context, logrus.NewEntry(logrus.New()), path)
	if err != nil {
		return err
	}

	// Print the counts after the volume closed.
	writeFields(c.App.Writer, res.fields(path))
	return nil
}

// volumeWriteOffResult is what a hole write-off found and wrote.
type volumeWriteOffResult struct {
	// audit is the audit that recorded the holes before the write-off.
	audit block_gc.AuditResult
	// wrote is what the write-off wrote.
	wrote block_gc.WriteOffResult
	// status is the settle status read after the write-off.
	status block_gc.SettleStatus
}

// holeOpen and holeAccepted are the second column of a --holes-out row.
const (
	holeOpen     = "open"
	holeAccepted = "written-off"
)

// writeOff records the stopped volume's holes with an audit, then writes off
// the ones the refs file names, and lists every recorded hole in the holes
// output. It writes off none when the audit left a named ref without a hole
// record, and still lists the holes then, so the operator can correct the
// list. Without a refs file it records and lists the holes only.
func (a *debugVolumeWriteOffArgs) writeOff(ctx context.Context, le *logrus.Entry, path string) (res volumeWriteOffResult, err error) {
	// Read the whole list first: the write-off checks every ref before it
	// writes one.
	if a.refs == "" && a.holesOut == "" {
		return res, errors.New("expected --refs, --holes-out or both")
	}
	var refs []*block.BlockRef
	inputs := []string{path}
	if a.refs != "" {
		if refs, err = readWriteOffRefs(ctx, a.refs); err != nil {
			return res, err
		}
		inputs = append(inputs, a.refs)
	}
	holes, closeHoles, err := openVolumeBlockLacks(a.holesOut, inputs...)
	if err != nil {
		return res, errors.Wrap(err, "holes output")
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(closeHoles(), "holes output")) }()

	// Open the stopped volume with the writer claim.
	vol, err := openStoppedVolumeStore(ctx, le, path, volumeWritable, a.upgrade.StoreConfig(nil), 0)
	if err != nil {
		return res, err
	}
	defer func() { err = stderrors.Join(err, errors.Wrap(vol.Close(), "close volume")) }()

	// Record every hole a live root reaches, the one proof that a block is
	// lost, then write off the named ones.
	if res.audit, err = vol.Audit(ctx); err != nil {
		return res, errors.Wrap(err, "record the holes")
	}
	var refused error
	if len(refs) != 0 {
		res.wrote, err = vol.WriteOffHoles(ctx, refs)
		if err != nil && !errors.Is(err, block_gc.ErrNotHole) {
			return res, err
		}
		refused = errors.Wrap(err, "the audit found this block stored, or reached by no live root; nothing was written off")
	}

	// List the holes as the write-off left them, a refused one included.
	err = vol.ScanHoles(ctx, func(ref *block.BlockRef, writtenOff bool) error {
		state := holeOpen
		if writtenOff {
			state = holeAccepted
		}
		_, err := io.WriteString(holes, ref.MarshalString()+"\t"+state+"\n")
		return err
	})
	if err = stderrors.Join(refused, errors.Wrap(err, "list the holes")); err != nil {
		return res, err
	}

	// Read what still holds the flip.
	res.status, err = vol.SettleStatus(ctx, settleStatusLimit)
	return res, err
}

// readWriteOffRefs reads the refs a write-off names from the file at path,
// which must name at least one.
func readWriteOffRefs(ctx context.Context, path string) ([]*block.BlockRef, error) {
	// Read every row's ref.
	input, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "open refs")
	}
	var refs []*block.BlockRef
	err = streamVolumeBlockRefs(ctx, input, writeOffRefsBatch, func(group []*block.BlockRef) error {
		refs = append(refs, slices.Clone(group)...)
		return nil
	})
	if err = stderrors.Join(err, errors.Wrap(input.Close(), "close refs")); err != nil {
		return nil, err
	}

	// Refuse an empty list, which would only run the audit.
	if len(refs) == 0 {
		return nil, errors.New("the refs file names no block")
	}
	return refs, nil
}

// fields formats the write-off in the command's field table.
func (r *volumeWriteOffResult) fields(path string) [][2]string {
	fields := [][2]string{
		{"Volume", path},
		{"Holes under a live root", strconv.Itoa(r.audit.Mark.Absent)},
		{"Holes the audit recorded", strconv.FormatUint(r.audit.Holes, 10)},
		{"Holes written off", strconv.FormatUint(r.wrote.WrittenOff, 10)},
		{"Named holes that ended first", strconv.FormatUint(r.wrote.Ended, 10)},
		{"Recorded holes", settleCount(r.status.Holes, r.status.HolesMore)},
		{"Holes not written off", settleCount(r.status.OpenHoles, r.status.HolesMore)},
	}
	return appendRefSample(fields, "Open hole sample", r.status.HoleSample)
}

// newDebugVolumeWriteOffCommand returns the offline hole write-off command.
func newDebugVolumeWriteOffCommand(upgrade *FormatUpgrade) *cli.Command {
	args := &debugVolumeWriteOffArgs{upgrade: upgrade}
	return &cli.Command{
		Name:      "volume-write-off",
		Usage:     "accept named lost blocks of a stopped volume as holes the settle may pass",
		ArgsUsage: "<volume-file>",
		Description: "A hole is a block the volume lost with its child list while a live root still reaches it. A recorded " +
			"hole holds the settle, so the collector frees nothing until a put stores the block again. This command " +
			"walks the volume as debug volume-audit does, records each hole, and writes off the ones --refs names: " +
			"a written-off hole no longer holds the settle. The first sweep after it then deletes what the volume " +
			"stores only below a written-off hole, since no walk can reach it without the lost block, so keep a copy " +
			"of the volume until that content is counted. A later put of the block from its content ends the hole " +
			"and holds the block's children. It writes off nothing when a named ref is not a hole. Refill what a " +
			"copy still holds with debug volume-refill first. With --holes-out it lists every recorded hole as " +
			"open or written-off, in the row format --refs reads, and with --holes-out alone it records and lists " +
			"the holes and writes none off. It keeps the reached set in memory. Run it only on a stopped volume.\n\n" +
			"For example, run spacewave debug volume-write-off --holes-out holes.tsv volume.s4db to list the holes, " +
			"then spacewave debug volume-write-off --refs lost.tsv volume.s4db",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
