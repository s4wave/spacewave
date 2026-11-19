//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// debugVolumeAuditWalkArgs are the arguments of the streaming audit walk.
type debugVolumeAuditWalkArgs struct {
	// scratch is the path of the walk's scratch store.
	scratch string
	// run names the walk, so a second run refuses a scratch store in use.
	run string
	// delay is how long a hole candidate waits before the walk rereads it.
	delay time.Duration
}

// BuildFlags returns the audit walk flags.
func (a *debugVolumeAuditWalkArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "scratch",
			Usage:       "the path of the walk's scratch store, created if missing and removed when the walk finishes",
			Destination: &a.scratch,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "run",
			Usage:       "the name of the walk; a scratch store holding another run's progress is refused",
			Value:       "walk",
			Destination: &a.run,
		},
		&cli.DurationFlag{
			Name:        "delay",
			Usage:       "how long a hole candidate waits before the walk rereads it; negative rereads at once",
			Value:       -1,
			Destination: &a.delay,
		},
	}
}

// Run walks the stopped volume and prints what the walk found.
func (a *debugVolumeAuditWalkArgs) Run(c *cli.Context) error {
	// Walk the volume at the argument's path.
	if c.NArg() != 1 {
		return errors.New("expected one volume file argument")
	}
	path := c.Args().First()
	le := logrus.NewEntry(logrus.New())
	res, err := auditWalkVolume(c.Context, le, path, a.scratch, block_gc.WalkOptions{Run: a.run, Delay: a.delay, Log: le})
	if err != nil {
		return err
	}

	// Print the findings.
	writeFields(os.Stdout, auditWalkFields(path, res))
	return nil
}

// auditWalkVolume walks the stopped volume at path read-only with its
// scratch store at scratch, which it removes once the walk finishes. A walk
// that fails keeps the scratch store, so the same run resumes it.
func auditWalkVolume(ctx context.Context, le *logrus.Entry, path, scratch string, opts block_gc.WalkOptions) (*block_gc.WalkResult, error) {
	// Open the stopped volume read-only and the scratch store.
	vol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
	if err != nil {
		return nil, err
	}
	defer vol.Close()
	store, err := openWalkScratch(ctx, scratch)
	if err != nil {
		return nil, err
	}

	// Walk, and remove the scratch store once the walk finishes.
	res, err := vol.AuditWalk(ctx, store, opts)
	if cerr := store.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	return res, removeWalkScratch(scratch)
}

// openWalkScratch creates or reopens the scratch store of an audit walk at
// path. Its cache and log are bounded, and its commits never wait on the
// disk: a process crash loses none, and a power loss that loses or corrupts
// the store costs the walk a restart with a new scratch store.
func openWalkScratch(ctx context.Context, path string) (store_kvtx_sqlite.Store, error) {
	return store_kvtx_sqlite.OpenWithPragmas(ctx, path, "walk", store_kvtx_sqlite.Pragmas{
		Sync:               store_kvtx_sqlite.SyncOff,
		CacheSize:          -2048,
		DisableMmap:        true,
		TempStore:          1,
		WithoutRowID:       true,
		WALCheckpointBytes: 8 << 20,
	})
}

// removeWalkScratch removes the scratch store at path with its log files.
func removeWalkScratch(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// auditWalkFields formats an audit walk in the command's field table.
func auditWalkFields(path string, res *block_gc.WalkResult) [][2]string {
	// List the counts.
	n := res.Counts
	fields := [][2]string{
		{"Volume", path},
		{"Adoption", res.Adoption.String()},
		{"Roots", strconv.FormatUint(n.GetRoots(), 10)},
		{"Child lists", strconv.FormatUint(n.GetLists(), 10)},
		{"Edges", strconv.FormatUint(n.GetEdges(), 10)},
		{"Checked blocks", strconv.FormatUint(n.GetChecked(), 10)},
		{"Undercounted blocks", strconv.FormatUint(n.GetUndercount(), 10)},
		{"Wide blocks whose namers moved, unchecked", strconv.FormatUint(n.GetWide(), 10)},
		{"Declined, the sweep's or counts not in force", strconv.FormatUint(n.GetDeclined(), 10)},
		{"Named without a child list", strconv.FormatUint(n.GetUnknown(), 10)},
		{"Hole candidates", strconv.FormatUint(n.GetCandidates(), 10)},
		{"Listed without bytes", strconv.FormatUint(n.GetListedAbsent(), 10)},
		{"Log entries copied", strconv.FormatUint(n.GetTailPairs(), 10)},
		{"Settled after the delay", strconv.FormatUint(n.GetSettled(), 10)},
		{"Unconfirmed after the delay", strconv.FormatUint(n.GetUnconfirmed(), 10)},
		{"Unconfirmed, a held root reaches", strconv.FormatUint(n.GetUnconfirmedHeld(), 10)},
		{"Unconfirmed, only lapsed roots reach", strconv.FormatUint(n.GetUnconfirmedLapsed(), 10)},
		{"Unconfirmed, no root reaches", strconv.FormatUint(n.GetUnconfirmedUnrooted(), 10)},
		{"Started", res.Started.Format(time.RFC3339)},
		{"Resumes", strconv.FormatUint(uint64(res.Resumes), 10)},
		{"Restarted a stale or unreadable run", strconv.FormatBool(res.Restarted)},
	}

	// Give the longest view of the Volume in each phase.
	for _, phase := range []block_gc.WalkPhase{
		block_gc.WalkPhase_WALK_PHASE_ROOTS,
		block_gc.WalkPhase_WALK_PHASE_LISTS,
		block_gc.WalkPhase_WALK_PHASE_CHECK,
		block_gc.WalkPhase_WALK_PHASE_CONFIRM,
	} {
		if d, ok := res.LongestView[phase]; ok {
			fields = append(fields, [2]string{"Longest view, " + phase.String(), d.String()})
		}
	}

	// Name a sample of the undercounted, listed-without-bytes and unconfirmed
	// blocks.
	fields = appendRefSample(fields, "Undercount sample", res.UndercountSample)
	fields = appendRefSample(fields, "Listed without bytes sample", res.ListedAbsentSample)
	for _, h := range res.UnconfirmedSample {
		fields = append(fields, [2]string{"Unconfirmed sample", h.Ref.MarshalString() + " " + h.Class.String()})
	}
	return fields
}

// newDebugVolumeAuditWalkCommand returns the streaming audit walk command.
func newDebugVolumeAuditWalkCommand() *cli.Command {
	args := &debugVolumeAuditWalkArgs{}
	return &cli.Command{
		Name:      "volume-audit-walk",
		Usage:     "walk a stopped volume's named roots and child lists in bounded slices and report holes and undercounts",
		ArgsUsage: "<volume-file>",
		Description: "Opens the volume read-only and writes nothing to it. It records the walk's edges and progress in a " +
			"separate scratch store, so its memory stays bounded at any volume size, and an interrupted walk resumes " +
			"under the same --run. It reports each undercounted block, whose counted holders are fewer than the edges " +
			"that still name it, and each named block absent without a hole record, rereading those after --delay and " +
			"classing what is still absent by whether a held root, only a lapsed root, or no root reaches it. It removes " +
			"the scratch store once the walk finishes. Run it only on a stopped volume: a block named by more than 4,096 " +
			"edges is checked across several views, and any write between them leaves it unchecked and counted wide. " +
			"Undercounts are checked only on an adopted volume; on any other, each listed block is counted declined.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}
