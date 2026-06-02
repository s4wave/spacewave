//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strconv"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	world_block "github.com/s4wave/spacewave/db/world/block"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

func newSpaceWorldCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var spaceID string
	return &cli.Command{
		Name:  "world",
		Usage: "inspect world storage and changelog state",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "space-id",
				Aliases:     []string{"space"},
				Usage:       "space ID or name (auto-detected if only one space)",
				EnvVars:     []string{"SPACEWAVE_SPACE"},
				Destination: &spaceID,
			},
		},
		Subcommands: []*cli.Command{
			buildSpaceWorldChangelogCommand(statePath, sessionIdx, &spaceID),
			buildSpaceWorldRollbackPlanCommand(statePath, sessionIdx, &spaceID),
		},
	}
}

func buildSpaceWorldChangelogCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var limit uint64 = 20
	var afterSeqno uint64
	var showChanges bool
	return &cli.Command{
		Name:  "changelog",
		Usage: "list recent world changelog entries",
		Flags: []cli.Flag{
			&cli.Uint64Flag{Name: "limit", Usage: "maximum changelog entries to read", Value: limit, Destination: &limit},
			&cli.Uint64Flag{Name: "after-seqno", Usage: "only include entries after this seqno", Destination: &afterSeqno},
			&cli.BoolFlag{Name: "show-changes", Usage: "print individual changes under each entry", Destination: &showChanges},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			engine, cleanup, sid, err := mountSpaceWorldEngine(ctx, c, *statePath, *sessionIdx, *spaceID)
			if err != nil {
				return err
			}
			defer cleanup()

			snapshot, err := engine.GetWorldRootSnapshot(ctx)
			if err != nil {
				return errors.Wrap(err, "get world root snapshot")
			}
			entries, err := world_block.ReadChangeLogEntries(ctx, engine.AccessWorldState, world_block.ChangeLogReadOptions{
				Limit:      limit,
				AfterSeqno: afterSeqno,
			})
			if err != nil {
				return errors.Wrap(err, "read changelog")
			}

			w := os.Stdout
			writeFields(w, [][2]string{
				{"Space", sid},
				{"Current Seqno", strconv.FormatUint(snapshot.GetSeqno(), 10)},
				{"Entries", strconv.Itoa(len(entries))},
			})
			if len(entries) == 0 {
				return nil
			}
			rows := [][]string{{"SEQNO", "TYPE", "CHANGES", "UNDO DATA", "FIRST KEY"}}
			for _, entry := range entries {
				rows = append(rows, []string{
					strconv.FormatUint(entry.Seqno, 10),
					entry.ChangeType.String(),
					strconv.Itoa(len(entry.Changes)),
					changeLogEntryUndoDataStatus(entry),
					firstChangeKey(entry),
				})
			}
			writeTable(w, "", rows)

			if showChanges {
				for _, entry := range entries {
					w.WriteString("\nseqno " + strconv.FormatUint(entry.Seqno, 10) + " " + entry.ChangeType.String() + "\n")
					changeRows := [][]string{{"IDX", "TYPE", "KEY", "DETAIL", "UNDO DATA"}}
					for i, change := range entry.Changes {
						changeRows = append(changeRows, []string{
							strconv.Itoa(i),
							change.GetChangeType().String(),
							change.GetKey(),
							changeDetail(change),
							worldChangeUndoDataStatus(change),
						})
					}
					writeTable(w, "  ", changeRows)
				}
			}
			return nil
		},
	}
}

func buildSpaceWorldRollbackPlanCommand(statePath *string, sessionIdx *uint, spaceID *string) *cli.Command {
	var targetSeqno uint64
	var limit uint64 = 1000
	var showChanges bool
	return &cli.Command{
		Name:  "rollback-plan",
		Usage: "dry-run changelog rollback feasibility after a target seqno",
		Flags: []cli.Flag{
			&cli.Uint64Flag{Name: "target-seqno", Usage: "target seqno before changes to revert", Destination: &targetSeqno, Required: true},
			&cli.Uint64Flag{Name: "limit", Usage: "maximum changelog entries to inspect", Value: limit, Destination: &limit},
			&cli.BoolFlag{Name: "show-changes", Usage: "print individual changes needing attention", Destination: &showChanges},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context
			engine, cleanup, sid, err := mountSpaceWorldEngine(ctx, c, *statePath, *sessionIdx, *spaceID)
			if err != nil {
				return err
			}
			defer cleanup()

			snapshot, err := engine.GetWorldRootSnapshot(ctx)
			if err != nil {
				return errors.Wrap(err, "get world root snapshot")
			}
			if targetSeqno >= snapshot.GetSeqno() {
				return errors.Errorf("target seqno %d must be below current seqno %d", targetSeqno, snapshot.GetSeqno())
			}
			entries, err := world_block.ReadChangeLogEntries(ctx, engine.AccessWorldState, world_block.ChangeLogReadOptions{
				Limit:      limit,
				AfterSeqno: targetSeqno,
			})
			if err != nil {
				return errors.Wrap(err, "read changelog")
			}

			stats := summarizeRollbackPlan(entries)
			w := os.Stdout
			writeFields(w, [][2]string{
				{"Space", sid},
				{"Current Seqno", strconv.FormatUint(snapshot.GetSeqno(), 10)},
				{"Target Seqno", strconv.FormatUint(targetSeqno, 10)},
				{"Entries Inspected", strconv.Itoa(len(entries))},
				{"Changes Inspected", strconv.Itoa(stats.total)},
				{"Complete Undo Data", strconv.Itoa(stats.complete)},
				{"Missing Undo Data", strconv.Itoa(stats.missing)},
				{"Partial Semantics", strconv.Itoa(stats.partial)},
				{"Exact Root Rollback", "not exposed by current remote API"},
			})
			if limit != 0 && uint64(len(entries)) == limit && entries[len(entries)-1].Seqno > targetSeqno+1 {
				w.WriteString("warning: inspection hit --limit before reaching target seqno\n")
			}
			if stats.missing == 0 && stats.partial == 0 {
				w.WriteString("result: changelog entries carry complete payload for a forward revert plan\n")
			} else {
				w.WriteString("result: changelog entries are not sufficient for an exact forward revert plan\n")
			}

			if showChanges || stats.missing != 0 || stats.partial != 0 {
				rows := [][]string{{"SEQNO", "TYPE", "KEY", "STATUS"}}
				for _, entry := range entries {
					for _, change := range entry.Changes {
						status := worldChangeUndoDataStatus(change)
						if showChanges || status != "complete" {
							rows = append(rows, []string{
								strconv.FormatUint(entry.Seqno, 10),
								change.GetChangeType().String(),
								change.GetKey(),
								status,
							})
						}
					}
				}
				if len(rows) > 1 {
					writeTable(w, "", rows)
				}
			}
			return nil
		},
	}
}

func mountSpaceWorldEngine(
	ctx context.Context,
	c *cli.Context,
	statePath string,
	sessionIdx uint,
	spaceID string,
) (*sdk_engine.SDKEngine, func(), string, error) {
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return nil, nil, "", err
	}

	sess, err := client.mountSession(ctx, uint32(sessionIdx))
	if err != nil {
		client.close()
		return nil, nil, "", err
	}

	sid, err := client.resolveSpaceID(ctx, sess, spaceID)
	if err != nil {
		sess.Release()
		client.close()
		return nil, nil, "", err
	}
	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, sid)
	if err != nil {
		sess.Release()
		client.close()
		return nil, nil, "", err
	}

	engine, engineCleanup, err := client.accessWorldEngine(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		client.close()
		return nil, nil, "", err
	}
	cleanup := func() {
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
	}
	return engine, cleanup, sid, nil
}

type rollbackPlanStats struct {
	total    int
	complete int
	missing  int
	partial  int
}

func summarizeRollbackPlan(entries []*world_block.ChangeLogEntry) rollbackPlanStats {
	var stats rollbackPlanStats
	for _, entry := range entries {
		for _, change := range entry.Changes {
			stats.total++
			switch worldChangeUndoDataStatus(change) {
			case "complete":
				stats.complete++
			case "partial: object revision decrement is not exposed":
				stats.partial++
			default:
				stats.missing++
			}
		}
	}
	return stats
}

func changeLogEntryUndoDataStatus(entry *world_block.ChangeLogEntry) string {
	var missing, partial int
	for _, change := range entry.Changes {
		switch worldChangeUndoDataStatus(change) {
		case "complete":
		case "partial: object revision decrement is not exposed":
			partial++
		default:
			missing++
		}
	}
	if missing != 0 {
		return "missing"
	}
	if partial != 0 {
		return "partial"
	}
	return "complete"
}

func worldChangeUndoDataStatus(change *world_block.WorldChange) string {
	if change == nil {
		return "missing: nil change"
	}
	switch change.GetChangeType() {
	case world_block.WorldChangeType_WorldChange_OBJECT_SET:
		if change.GetPrevObjectRef().GetEmpty() {
			if change.GetObjectRef().GetEmpty() {
				return "missing: object_ref"
			}
			return "complete"
		}
		if change.GetObjectRef().GetEmpty() {
			return "missing: object_ref"
		}
		return "complete"
	case world_block.WorldChangeType_WorldChange_OBJECT_DELETE:
		if change.GetPrevObjectRef().GetEmpty() {
			return "missing: prev_object_ref"
		}
		return "complete"
	case world_block.WorldChangeType_WorldChange_OBJECT_RENAME:
		if change.GetKey() == "" {
			return "missing: key"
		}
		if change.GetNewKey() == "" {
			return "missing: new_key"
		}
		return "complete"
	case world_block.WorldChangeType_WorldChange_OBJECT_INC_REV:
		return "partial: object revision decrement is not exposed"
	case world_block.WorldChangeType_WorldChange_GRAPH_SET,
		world_block.WorldChangeType_WorldChange_GRAPH_DELETE:
		if change.GetQuad() == nil {
			return "missing: quad"
		}
		return "complete"
	default:
		return "missing: unsupported change type"
	}
}

func firstChangeKey(entry *world_block.ChangeLogEntry) string {
	for _, change := range entry.Changes {
		if key := change.GetKey(); key != "" {
			return key
		}
	}
	return ""
}

func changeDetail(change *world_block.WorldChange) string {
	switch change.GetChangeType() {
	case world_block.WorldChangeType_WorldChange_OBJECT_RENAME:
		return change.GetNewKey()
	case world_block.WorldChangeType_WorldChange_GRAPH_SET,
		world_block.WorldChangeType_WorldChange_GRAPH_DELETE:
		if change.GetQuad() != nil {
			return change.GetQuad().String()
		}
	}
	return ""
}
