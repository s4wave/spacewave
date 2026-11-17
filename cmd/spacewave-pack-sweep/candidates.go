package main

import (
	"bufio"
	"strconv"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/packsweep"
)

// verdictNames are the names a judgment's verdict takes.
var verdictNames = []string{
	packsweep.VerdictKept.String(),
	packsweep.VerdictDead.String(),
	packsweep.VerdictLow.String(),
	packsweep.VerdictYoung.String(),
}

// candidatesFlags binds the flags of the candidate list.
func (a *Args) candidatesFlags() []cli.Flag {
	return append(a.scratchFlags(),
		&cli.StringFlag{Name: "candidates", Usage: "candidate list to write, one JSON judgment a line", Value: "candidates.jsonl", Destination: &a.Candidates},
		&cli.StringSliceFlag{Name: "verdict", Usage: "verdict a candidate carries: kept, dead, low or young; repeat for each", Value: cli.NewStringSlice(packsweep.VerdictDead.String()), Destination: &a.Verdicts},
	)
}

// runCandidates writes the last judgment's packs whose verdict is one of
// --verdict, each line the report's, and prints their count and bytes. It
// reads the scratch file alone and writes only the list, so it runs between
// any two steps and repeats as often as wanted.
func (a *Args) runCandidates(*cli.Context) error {
	// Check the verdicts named before opening anything.
	known := make(map[string]bool, len(verdictNames))
	for _, v := range verdictNames {
		known[v] = true
	}
	verdicts := make(map[string]bool)
	for _, v := range a.Verdicts.Value() {
		if !known[v] {
			return errors.Errorf("--verdict %q: want one of %v", v, verdictNames)
		}
		verdicts[v] = true
	}

	// Open the scratch file until a signal stops the step.
	ctx, cancel := signalContext()
	defer cancel()
	db, store, err := a.openScratch()
	if err != nil {
		return err
	}
	defer db.Close()

	// Write each judgment with a named verdict, counting packs and bytes.
	head, err := packsweep.ReadHead(ctx, store)
	if err != nil {
		return err
	}
	if head.JudgedAt.IsZero() {
		return errors.New("the cycle has no judgment yet; run judge first")
	}
	var packs, size uint64
	if err := writeLines(a.Candidates, func(w *bufio.Writer) error {
		var arena fastjson.Arena
		var line []byte
		return packsweep.Judgments(ctx, store, func(j *packsweep.Judgment) error {
			// Skip a judgment whose verdict was not named.
			if !verdicts[j.Verdict.String()] {
				return nil
			}

			// Count it and write its line.
			packs, size = packs+1, size+j.Size
			arena.Reset()
			line = append(j.JSON(&arena).MarshalTo(line[:0]), '\n')
			_, err := w.Write(line)
			return err
		})
	}); err != nil {
		return errors.Wrap(err, "write --candidates")
	}

	// Print the totals with the cycle and judgment they came from.
	return a.print(func(arena *fastjson.Arena) *fastjson.Value {
		// Build the totals object.
		o := arena.NewObject()
		o.Set("cycle", arena.NewNumberInt(head.Cycle))
		o.Set("judgedAt", arena.NewString(head.JudgedAt.UTC().Format(time.RFC3339Nano)))
		o.Set("packs", arena.NewNumberString(strconv.FormatUint(packs, 10)))
		o.Set("size", arena.NewNumberString(strconv.FormatUint(size, 10)))
		return o
	})
}
