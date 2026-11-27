//go:build !js

package spacewave_cli

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

// spaceOrderArgs are the arguments of the space order command.
type spaceOrderArgs struct {
	// statePath is the daemon state path of the space command group.
	statePath *string
	// sessionIdx is the session index of the space command group.
	sessionIdx *uint
	// All prints every operation, not only those this build cannot decode.
	All bool
	// Output is the output format: text, or json for one message per line.
	Output string
}

// newSpaceOrderCommand builds the space order subcommand.
func newSpaceOrderCommand(statePath *string, sessionIdx *uint) *cli.Command {
	args := &spaceOrderArgs{statePath: statePath, sessionIdx: sessionIdx}
	return &cli.Command{
		Name:      "order",
		Usage:     "inspect the operations of the space's order this build cannot decode",
		ArgsUsage: "<space-id>",
		Description: "Walks this device's agreed order of the space from its " +
			"checkpoint to the head, read only, and prints each operation " +
			"whose World operation body this build cannot decode, or that the " +
			"saved replay cursor refused as an unknown operation type, then " +
			"the counts of the whole order by body kind. It mounts the " +
			"space's shared object without its World and writes nothing.",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the command's flags.
func (a *spaceOrderArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:        "all",
			Usage:       "print every operation of the order",
			Destination: &a.All,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output format (text, or json for one message per line)",
			Value:       "text",
			Destination: &a.Output,
		},
	}
}

// Run inspects the order and prints it as it streams.
func (a *spaceOrderArgs) Run(c *cli.Context) error {
	// Take the space and the output format.
	if c.Args().Len() != 1 {
		return errors.New("usage: space order <space-id>")
	}
	if a.Output != "text" && a.Output != "json" {
		return errors.Errorf("unknown output format %q", a.Output)
	}

	// Mount the selected session.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, *a.statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIndex32(*a.sessionIdx))
	if err != nil {
		return err
	}
	defer sess.Release()

	// Resolve and mount the Space's shared object, without its body.
	spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
	if err != nil {
		return err
	}
	soSvc, soRelease, err := client.mountSharedObject(ctx, sess, spaceID)
	if err != nil {
		return err
	}
	defer soRelease()

	// Print each message as it arrives, ending at the summary.
	strm, err := soSvc.InspectSharedObjectOrder(ctx, &s4wave_sobject.InspectSharedObjectOrderRequest{All: a.All})
	if err != nil {
		return errors.Wrap(err, "inspect shared object order")
	}
	defer strm.Close()
	w := os.Stdout
	for {
		msg, err := strm.Recv()
		if err != nil {
			return errors.Wrap(err, "inspect shared object order")
		}
		if a.Output == "json" {
			data, err := msg.MarshalJSON()
			if err != nil {
				return err
			}
			if _, err := w.Write(append(data, '\n')); err != nil {
				return err
			}
		} else if op := msg.GetOperation(); op != nil {
			printOrderOperation(w, op)
		}
		if summary := msg.GetSummary(); summary != nil {
			if a.Output == "text" {
				printOrderInspection(w, spaceID, summary)
			}
			return nil
		}
	}
}

// printOrderOperation prints one operation of the order on one line.
func printOrderOperation(w io.Writer, op *space_world.OrderOperation) {
	// Describe what this build reads of the operation.
	var read string
	switch r := op.GetRead().(type) {
	case *space_world.OrderOperation_Body:
		read = orderKindName(r.Body.GetKind())
	case *space_world.OrderOperation_Unreadable:
		fields := make([]string, len(r.Unreadable.GetFields()))
		for i, f := range r.Unreadable.GetFields() {
			fields[i] = strconv.FormatUint(uint64(f), 10)
		}
		read = "unreadable fields=" + strings.Join(fields, ",")
	case *space_world.OrderOperation_Empty:
		read = "empty"
	case *space_world.OrderOperation_Invalid:
		read = "invalid: " + r.Invalid.GetError()
	case *space_world.OrderOperation_Refused:
		read = "refused: " + r.Refused.GetReason()
	case *space_world.OrderOperation_Acknowledgment:
		read = "acknowledgment"
	}

	// Print the position, author, hash and saved outcome beside the read
	// result.
	line := fmt.Sprintf("%d\t%s\tnonce %d\t%s\t%s", op.GetPosition(), op.GetPeerId(), op.GetNonce(), hex.EncodeToString(op.GetHash()), read)
	if reason := op.GetSavedReason(); reason != "" {
		line += "\tsaved: " + reason
	}
	fmt.Fprintln(w, line)
}

// printOrderInspection prints the counts of the whole order.
func printOrderInspection(w io.Writer, spaceID string, s *space_world.OrderInspection) {
	// Count the order by body kind.
	pairs := [][2]string{
		{"Space", spaceID},
		{"Checkpoint", strconv.FormatUint(s.GetCheckpointHeight(), 10)},
		{"Operations", strconv.FormatUint(uint64(s.GetOperations()), 10)},
	}
	for _, k := range s.GetKinds() {
		pairs = append(pairs, [2]string{"  " + orderKindName(k.GetKind()), strconv.FormatUint(uint64(k.GetCount()), 10)})
	}

	// Count operations with no body this build decodes, and the saved
	// cursor.
	pairs = append(pairs,
		[2]string{"Unreadable", strconv.FormatUint(uint64(s.GetUnreadable()), 10)},
		[2]string{"Empty", strconv.FormatUint(uint64(s.GetEmpty()), 10)},
		[2]string{"Invalid", strconv.FormatUint(uint64(s.GetInvalid()), 10)},
		[2]string{"Refused", strconv.FormatUint(uint64(s.GetRefused()), 10)},
		[2]string{"Acknowledgments", strconv.FormatUint(uint64(s.GetAcknowledgments()), 10)},
	)
	if cur := s.GetCursor(); cur != nil {
		pairs = append(pairs,
			[2]string{"Saved cursor", fmt.Sprintf("height %d, %d outcomes", cur.GetHeight(), cur.GetOutcomes())},
			[2]string{"Saved unknown type", strconv.FormatUint(uint64(cur.GetUnknownOperationType()), 10)},
		)
	} else {
		pairs = append(pairs, [2]string{"Saved cursor", "none"})
	}
	writeFields(w, pairs)
}

// orderKindName returns the short name of a WorldOp body kind.
func orderKindName(kind space_world.WorldOpKind) string {
	return strings.ToLower(strings.TrimPrefix(kind.String(), "WORLD_OP_KIND_"))
}
