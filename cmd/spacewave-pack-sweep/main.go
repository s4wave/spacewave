// Command spacewave-pack-sweep proves which packs of a pack store hold no
// block a set of roots reaches, and moves them out of the reads.
//
// A cycle takes a snapshot of the store's registered packs, reading each pack
// the scratch file's catalog lacks once, walks what the roots reach through the
// catalog's refs, and judges each pack dead, low liveness, young or kept. It
// reads the store over SFTP, finding its packs through a copy of the store's
// pack index file, and keeps everything it learns in one scratch file, so a
// stopped step resumes where its last batch left it.
//
// Only quarantine, recheck, trash and remove change the store, each through
// the backend's admin service: quarantine takes the dead packs out of the
// reads, leaving each readable through the fallback, and recheck passes each
// that stayed unreached, or lifts it back. Trash marks each passed pack
// trashing and moves its entry, then its file, to the store's trash, where a
// read that still falls back to it records a miss. Remove drops each trashed
// pack no read missed from the backend's index, pointing each block row that
// named it at another pack holding the block.
package main

import (
	"os"

	"github.com/aperturerobotics/cli"
)

// main runs the command the arguments name.
func main() {
	// Describe the commands.
	args := &Args{}
	app := cli.NewApp()
	app.Name = "spacewave-pack-sweep"
	app.Usage = "prove which packs no root reaches, and move them to the trash"
	app.Commands = []*cli.Command{
		{
			Name:   "new-cycle",
			Usage:  "start a cycle, keeping the catalog of packs already read",
			Flags:  args.scratchFlags(),
			Action: args.runNewCycle,
		},
		{
			Name:   "snapshot",
			Usage:  "list the store's packs and read each the catalog lacks",
			Flags:  append(args.scratchFlags(), args.storeFlags()...),
			Action: args.runSnapshot,
		},
		{
			Name:   "walk",
			Usage:  "walk what the roots reach through the catalog, listing the holes",
			Flags:  append(args.scratchFlags(), args.walkFlags()...),
			Action: args.runWalk,
		},
		{
			Name:   "pull-log",
			Usage:  "store the write log's lines since the last pull and trim them from its directory",
			Flags:  append(args.scratchFlags(), args.pullFlags()...),
			Action: args.runPullLog,
		},
		{
			Name:   "closure",
			Usage:  "walk what the pulled log keys reach that the roots do not",
			Flags:  append(args.scratchFlags(), args.closureFlags()...),
			Action: args.runClosure,
		},
		{
			Name:   "judge",
			Usage:  "judge each snapshot pack and write the report",
			Flags:  append(args.scratchFlags(), args.judgeFlags()...),
			Action: args.runJudge,
		},
		{
			Name:   "quarantine",
			Usage:  "quarantine through the backend each pack judged dead and still unreached",
			Flags:  args.adminFlags(),
			Action: args.runQuarantine,
		},
		{
			Name:   "recheck",
			Usage:  "pass each quarantined pack that waited, with no write or read open since, and nothing reached",
			Flags:  append(args.adminFlags(), args.waitFlag()),
			Action: args.runRecheck,
		},
		{
			Name:   "trash",
			Usage:  "move each passed pack to the store's trash through the backend, entry first",
			Flags:  args.trashFlags(),
			Action: args.runTrash,
		},
		{
			Name:   "remove",
			Usage:  "have the backend remove each trashed pack no read missed, repairing the rows naming it",
			Flags:  args.removeFlags(),
			Action: args.runRemove,
		},
		{
			Name:   "status",
			Usage:  "print the cycle's state",
			Flags:  args.scratchFlags(),
			Action: args.runStatus,
		},
	}

	// Run the one named.
	if err := app.Run(os.Args); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
