//go:build !js

package spacewave_cli

import (
	"context"
	"os"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/manifoldco/promptui"
	"github.com/pkg/errors"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"golang.org/x/term"
)

// sessionPairArgs are the arguments of the session pair command.
type sessionPairArgs struct {
	statePath  string
	sessionIdx uint
	yes        bool
}

// newSessionPairCommand builds the session pair command.
func newSessionPairCommand() *cli.Command {
	args := &sessionPairArgs{}
	return &cli.Command{
		Name:   "pair",
		Usage:  "issue a code that adds this session's account to another device",
		Flags:  args.BuildFlags(),
		Action: args.Run,
	}
}

// BuildFlags returns the client flags and the approval flag.
func (a *sessionPairArgs) BuildFlags() []cli.Flag {
	return append(
		clientFlags(&a.statePath, &a.sessionIdx),
		&cli.BoolFlag{
			Name:        "yes",
			Aliases:     []string{"y"},
			Usage:       "approve without comparing the emoji; whoever enters the code first gains the account",
			Destination: &a.yes,
		},
	)
}

// Run issues a pairing code, waits for the other device to enter it, and
// approves the link once the emoji are confirmed.
func (a *sessionPairArgs) Run(c *cli.Context) error {
	// Refuse to wait for an approval no one can give.
	if !a.yes && !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("pass --yes, or run in an interactive terminal")
	}

	// Mount the selected session.
	ctx := c.Context
	outputFormat := c.String("output")
	client, err := connectDaemonFromContext(ctx, c, a.statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIndex32(a.sessionIdx))
	if err != nil {
		return err
	}
	defer sess.Release()

	// Issue the code and show it with the command the other device runs.
	resp, err := sess.GeneratePairingCode(ctx)
	if err != nil {
		return errors.Wrap(err, "generate pairing code")
	}
	code := resp.GetCode()
	if err := printSessionPairCode(code, outputFormat); err != nil {
		return err
	}

	// Approve the device that enters the code.
	remotePeerID, remoteLabel, err := a.approve(ctx, sess, outputFormat)
	if err != nil {
		return err
	}

	// Report the linked device.
	if outputFormat == "json" || outputFormat == "yaml" {
		return writePairingRecord(outputFormat, "linked", [][2]string{
			{"remotePeerId", remotePeerID},
			{"remoteLabel", remoteLabel},
		}, nil, false)
	}
	os.Stdout.WriteString("Device linked.\n\n")
	writeFields(os.Stdout, [][2]string{
		{"Device", remoteLabel},
		{"Peer", remotePeerID},
	})
	return nil
}

// approve drives the offering side of the pairing until both devices confirm.
// It returns the peer ID and label of the linked device.
func (a *sessionPairArgs) approve(ctx context.Context, sess *s4wave_session.Session, outputFormat string) (string, string, error) {
	// Watch pairing status until both devices confirm.
	watch, err := sess.WatchPairingStatus(ctx)
	if err != nil {
		return "", "", errors.Wrap(err, "watch pairing")
	}
	defer watch.Close()
	confirmed := false
	for {
		state, err := watch.Recv()
		if err != nil {
			return "", "", errors.Wrap(err, "watch pairing")
		}
		switch state.GetStatus() {
		case s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI:
			if confirmed {
				continue
			}
			approved, err := a.confirmEmoji(state, outputFormat)
			if err != nil {
				return "", "", err
			}
			if err := sess.ConfirmSASMatch(ctx, approved); err != nil {
				return "", "", errors.Wrap(err, "confirm emoji")
			}
			if !approved {
				return "", "", errors.New("pairing cancelled")
			}
			confirmed = true
		case s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED:
			if !confirmed {
				return "", "", errors.New("other device confirmed before local verification")
			}
			return state.GetRemotePeerId(), state.GetRemoteLabel(), nil
		case s4wave_session.PairingStatus_PairingStatus_PAIRING_REJECTED:
			return "", "", errors.New("pairing rejected on the other device")
		case s4wave_session.PairingStatus_PairingStatus_CONFIRMATION_TIMEOUT:
			return "", "", errors.New("the other device did not approve in time")
		case s4wave_session.PairingStatus_PairingStatus_FAILED,
			s4wave_session.PairingStatus_PairingStatus_SIGNALING_FAILED,
			s4wave_session.PairingStatus_PairingStatus_CONNECTION_TIMEOUT:
			return "", "", errors.Errorf("pairing failed: %s", state.GetErrorMessage())
		}
	}
}

// confirmEmoji shows the verification emoji and returns whether the link is
// approved: by the prompt, or by --yes.
func (a *sessionPairArgs) confirmEmoji(state *s4wave_session.WatchPairingStatusResponse, outputFormat string) (bool, error) {
	// Require the full emoji sequence before anyone compares it.
	emoji := state.GetEmoji()
	if len(emoji) != 6 {
		return false, errors.New("pairing did not provide six verification emoji")
	}
	remote := state.GetRemoteLabel()
	if remote == "" {
		remote = "the other device"
	}

	// Ask in the terminal unless --yes approved in advance.
	if !a.yes {
		os.Stdout.WriteString("Confirm these emoji match on " + remote + ":\n" + strings.Join(emoji, " ") + "\n")
		_, err := (&promptui.Prompt{Label: "Do they match", IsConfirm: true}).Run()
		return err == nil, nil
	}
	if outputFormat == "json" || outputFormat == "yaml" {
		return true, writePairingRecord(outputFormat, "verify", nil, emoji, true)
	}
	os.Stdout.WriteString("Approving " + remote + " without comparing these emoji:\n" + strings.Join(emoji, " ") + "\n")
	return true, nil
}

// printSessionPairCode shows the code and the command that consumes it.
func printSessionPairCode(code, outputFormat string) error {
	// Emit one record before the verify and linked records, or show the steps.
	if outputFormat == "json" || outputFormat == "yaml" {
		return writePairingRecord(outputFormat, "code", [][2]string{{"code", code}}, nil, true)
	}
	os.Stdout.WriteString("Pairing code: " + code + "\n\n")
	os.Stdout.WriteString("On the other device, run:\n  spacewave login pair --code " + code + "\n\n")
	os.Stdout.WriteString("Waiting for the other device...\n")
	return nil
}
