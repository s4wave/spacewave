//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"golang.org/x/term"
)

// originSecretEnv names the environment variable that supplies the origin's
// secret key, matching the standard S3 client variable.
const originSecretEnv = "AWS_SECRET_ACCESS_KEY" //nolint:gosec // the name of the variable, not a credential.

// newSpaceOriginCommand builds the space origin command group.
func newSpaceOriginCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:  "origin",
		Usage: "serve a public space from your own bucket and domain",
		Subcommands: []*cli.Command{
			newSpaceOriginLinkCommand(statePath, sessionIdx),
			newSpaceOriginStatusCommand(statePath, sessionIdx),
			newSpaceOriginReleaseHostedCommand(statePath, sessionIdx),
		},
	}
}

// newSpaceOriginLinkCommand builds the space origin link subcommand.
func newSpaceOriginLinkCommand(statePath *string, sessionIdx *uint) *cli.Command {
	link := &s4wave_provider_spacewave.PublicOriginLink{}
	return &cli.Command{
		Name:  "link",
		Usage: "link an S3-compatible bucket and its public domain to a public space",
		Description: "The secret key is read from " + originSecretEnv + ", or from standard input when unset.\n" +
			"Linking proves the public URL serves the bucket, then copies the space's existing\n" +
			"packs there. Pushes move to the bucket once the copy completes.",
		ArgsUsage: "[space-id]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "endpoint",
				Usage:       "bucket S3 API endpoint, such as https://s3.us-west-004.backblazeb2.com",
				Required:    true,
				Destination: &link.Endpoint,
			},
			&cli.StringFlag{
				Name:        "region",
				Usage:       "S3 signing region",
				Required:    true,
				Destination: &link.Region,
			},
			&cli.StringFlag{
				Name:        "bucket",
				Usage:       "bucket name",
				Required:    true,
				Destination: &link.Bucket,
			},
			&cli.StringFlag{
				Name:        "access-key-id",
				Usage:       "key that can read, write and delete in the bucket",
				EnvVars:     []string{"AWS_ACCESS_KEY_ID"},
				Required:    true,
				Destination: &link.AccessKeyId,
			},
			&cli.StringFlag{
				Name:        "public-url",
				Usage:       "https URL that serves the bucket, such as https://cdn.example.com",
				Required:    true,
				Destination: &link.PublicBaseUrl,
			},
			outputFlag(),
		},
		Action: func(c *cli.Context) error {
			secret, err := readOriginSecret()
			if err != nil {
				return err
			}
			link.SecretAccessKey = secret

			return runSpaceOriginCommand(c, *statePath, *sessionIdx, func(
				ctx context.Context,
				svc s4wave_session.SRPCSpacewaveSessionResourceServiceClient,
				spaceID string,
			) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
				resp, err := svc.LinkSpacePublicOrigin(ctx, &s4wave_provider_spacewave.LinkSpacePublicOriginRequest{
					SpaceId: spaceID,
					Link:    link,
				})
				return resp.GetStatus(), errors.Wrap(err, "link public origin")
			})
		},
	}
}

// newSpaceOriginStatusCommand builds the space origin status subcommand.
func newSpaceOriginStatusCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "show where a public space's packs are served from",
		ArgsUsage: "[space-id]",
		Flags:     []cli.Flag{outputFlag()},
		Action: func(c *cli.Context) error {
			return runSpaceOriginCommand(c, *statePath, *sessionIdx, func(
				ctx context.Context,
				svc s4wave_session.SRPCSpacewaveSessionResourceServiceClient,
				spaceID string,
			) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
				resp, err := svc.GetSpacePublicOrigin(ctx, &s4wave_provider_spacewave.GetSpacePublicOriginRequest{
					SpaceId: spaceID,
				})
				return resp.GetStatus(), errors.Wrap(err, "get public origin")
			})
		},
	}
}

// newSpaceOriginReleaseHostedCommand builds the space origin release-hosted
// subcommand.
func newSpaceOriginReleaseHostedCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:  "release-hosted",
		Usage: "delete Spacewave's copies of a space that moved to its own bucket",
		Description: "Readers that still use Spacewave's CDN address stop loading the space.\n" +
			"Run this once every reader uses the linked public URL.",
		ArgsUsage: "[space-id]",
		Flags:     []cli.Flag{outputFlag()},
		Action: func(c *cli.Context) error {
			return runSpaceOriginCommand(c, *statePath, *sessionIdx, func(
				ctx context.Context,
				svc s4wave_session.SRPCSpacewaveSessionResourceServiceClient,
				spaceID string,
			) (*s4wave_provider_spacewave.PublicOriginStatus, error) {
				resp, err := svc.ReleaseSpaceHostedCopies(ctx, &s4wave_provider_spacewave.ReleaseSpaceHostedCopiesRequest{
					SpaceId: spaceID,
				})
				return resp.GetStatus(), errors.Wrap(err, "release hosted copies")
			})
		},
	}
}

// runSpaceOriginCommand mounts the selected session, resolves the Space
// argument, runs call against the session's cloud service and prints the
// returned origin status.
func runSpaceOriginCommand(
	c *cli.Context,
	statePath string,
	sessionIdx uint,
	call func(context.Context, s4wave_session.SRPCSpacewaveSessionResourceServiceClient, string) (*s4wave_provider_spacewave.PublicOriginStatus, error),
) error {
	// Connect to the daemon.
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()

	// Mount the selected session.
	sess, err := client.mountSession(ctx, sessionIndex32(sessionIdx))
	if err != nil {
		return err
	}
	defer sess.Release()

	// Resolve the Space and the session's cloud service.
	spaceID, err := client.resolveSpaceID(ctx, sess, c.Args().First())
	if err != nil {
		return err
	}
	sessionClient, err := sess.GetResourceRef().GetClient()
	if err != nil {
		return errors.Wrap(err, "session client")
	}
	svc := s4wave_session.NewSRPCSpacewaveSessionResourceServiceClient(sessionClient)

	// Run the call and print the origin it reports.
	status, err := call(ctx, svc, spaceID)
	if err != nil {
		return err
	}
	switch c.String("output") {
	case "json", "yaml":
		data, err := status.MarshalJSON()
		if err != nil {
			return err
		}
		return formatOutput(data, c.String("output"))
	default:
		printPublicOriginStatus(spaceID, status)
		return nil
	}
}

// readOriginSecret returns the origin secret key from the environment, or from
// standard input: a hidden prompt on a terminal, otherwise the piped text.
func readOriginSecret() (string, error) {
	// Prefer the environment, which keeps the secret out of the terminal.
	if secret := os.Getenv(originSecretEnv); secret != "" {
		return secret, nil
	}

	// Prompt without echo when a person is typing.
	if term.IsTerminal(int(os.Stdin.Fd())) {
		os.Stderr.WriteString("Secret access key: ")
		data, err := term.ReadPassword(int(os.Stdin.Fd()))
		os.Stderr.WriteString("\n")
		if err != nil {
			return "", errors.Wrap(err, "read secret access key")
		}
		return requireOriginSecret(string(data))
	}

	// Read a piped secret.
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", errors.Wrap(err, "read secret access key")
	}
	return requireOriginSecret(string(data))
}

// requireOriginSecret trims secret and rejects an empty one.
func requireOriginSecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errors.New("secret access key is empty; set " + originSecretEnv + " or pipe it on standard input")
	}
	return secret, nil
}

// printPublicOriginStatus prints a Space's origin status to stdout.
func printPublicOriginStatus(spaceID string, status *s4wave_provider_spacewave.PublicOriginStatus) {
	// Name the origin; an unlinked Space shows nothing more.
	fields := [][2]string{{"Space", spaceID}}
	switch status.GetState() {
	case s4wave_provider_spacewave.PublicOriginState_PublicOriginState_COPYING:
		fields = append(fields, [2]string{"Origin", "copying existing packs"})
	case s4wave_provider_spacewave.PublicOriginState_PublicOriginState_ACTIVE:
		fields = append(fields, [2]string{"Origin", "active"})
	default:
		fields = append(fields, [2]string{"Origin", "Spacewave CDN"})
		writeFields(os.Stdout, fields)
		return
	}
	fields = append(fields,
		[2]string{"Public URL", status.GetPublicBaseUrl()},
		[2]string{"Bucket", status.GetBucket() + " at " + status.GetEndpoint()},
		[2]string{"Packs copied", strconv.FormatUint(uint64(status.GetPacksCopied()), 10) + " of " + strconv.FormatUint(uint64(status.GetPacksTotal()), 10)},
	)
	if status.GetHostedCopies() {
		fields = append(fields, [2]string{"Hosted copies", "kept on Spacewave's CDN"})
	} else {
		fields = append(fields, [2]string{"Hosted copies", "released"})
	}
	if copyErr := status.GetCopyError(); copyErr != "" {
		fields = append(fields, [2]string{"Copy error", copyErr + " (retrying)"})
	}
	writeFields(os.Stdout, fields)
}
