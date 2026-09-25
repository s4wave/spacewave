//go:build !js

package spacewave_cli

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"golang.org/x/term"
)

// s3Presets lists the endpoint presets accepted by storage add s3.
var s3Presets = []string{"aws", "r2", "b2", "minio", "custom"}

// s3AddArgs holds the flags of storage add s3.
type s3AddArgs struct {
	preset     string
	endpoint   string
	region     string
	bucket     string
	prefix     string
	accountID  string
	insecure   bool
	setDefault bool
}

// newStorageCommand builds the storage command group.
func newStorageCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	var statePath string
	var sessionIdx uint
	return &cli.Command{
		Name:  "storage",
		Usage: "manage the account's storage backends",
		Flags: clientFlags(&statePath, &sessionIdx),
		Subcommands: []*cli.Command{
			newStorageAddCommand(&statePath, &sessionIdx),
			newStorageListCommand(&statePath, &sessionIdx),
			newStorageTestCommand(&statePath, &sessionIdx),
			newStorageDefaultCommand(&statePath, &sessionIdx),
			newStorageRemoveCommand(&statePath, &sessionIdx),
		},
	}
}

// newStorageAddCommand builds the storage add command group.
func newStorageAddCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var args s3AddArgs
	return &cli.Command{
		Name:  "add",
		Usage: "add a storage backend",
		Subcommands: []*cli.Command{{
			Name:      "s3",
			Usage:     "add an S3-compatible bucket",
			ArgsUsage: "<name>",
			Description: "Reads the access key from AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY\n" +
				"(and AWS_SESSION_TOKEN when set), otherwise from standard input: the\n" +
				"access key id on the first line and the secret on the second. Saves\n" +
				"the backend only after writing, reading, and deleting a probe object.",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "preset",
					Usage:       "endpoint preset: " + strings.Join(s3Presets, ", "),
					Value:       "custom",
					Destination: &args.preset,
				},
				&cli.StringFlag{
					Name:        "bucket",
					Usage:       "bucket name",
					Required:    true,
					Destination: &args.bucket,
				},
				&cli.StringFlag{
					Name:        "region",
					Usage:       "bucket region (aws and b2 build the endpoint from it)",
					Destination: &args.region,
				},
				&cli.StringFlag{
					Name:        "endpoint",
					Usage:       "endpoint host[:port], overriding the preset",
					Destination: &args.endpoint,
				},
				&cli.StringFlag{
					Name:        "account-id",
					Usage:       "Cloudflare account id for the r2 preset",
					Destination: &args.accountID,
				},
				&cli.StringFlag{
					Name:        "prefix",
					Usage:       "object key prefix inside the bucket",
					Destination: &args.prefix,
				},
				&cli.BoolFlag{
					Name:        "insecure",
					Usage:       "connect without TLS (the minio preset sets this)",
					Destination: &args.insecure,
				},
				&cli.BoolFlag{
					Name:        "default",
					Usage:       "place new Spaces on this backend",
					Destination: &args.setDefault,
				},
				outputFlag(),
			},
			Action: func(c *cli.Context) error {
				name := c.Args().First()
				if name == "" {
					return errors.New("backend name required")
				}
				location, err := args.location()
				if err != nil {
					return err
				}
				creds, err := readS3Credentials()
				if err != nil {
					return err
				}
				return withSession(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
					resp, err := sess.AddStorageBackend(ctx, &s4wave_session.AddStorageBackendRequest{
						DisplayName: name,
						S3:          location,
						Credentials: creds,
						SetDefault:  args.setDefault,
					})
					if err != nil {
						return errors.Wrap(err, "add storage backend")
					}
					if format := c.String("output"); format == "json" || format == "yaml" {
						data, err := resp.MarshalJSON()
						if err != nil {
							return err
						}
						if err := formatOutput(data, format); err != nil {
							return err
						}
						return checkResultError(resp.GetCheck())
					}
					if resp.GetStorageBackendId() == "" {
						return checkResultError(resp.GetCheck())
					}
					os.Stdout.WriteString("added " + name + ": " + location.GetBucket() + " on " + location.GetEndpoint() + "\n")
					return nil
				})
			},
		}},
	}
}

// location builds the S3 location from the preset and flags.
func (a *s3AddArgs) location() (*account_settings.S3Location, error) {
	location := &account_settings.S3Location{
		Endpoint:     a.endpoint,
		Region:       a.region,
		Bucket:       a.bucket,
		ObjectPrefix: a.prefix,
		DisableSsl:   a.insecure,
	}
	switch a.preset {
	case "aws":
		if location.Region == "" {
			location.Region = "us-east-1"
		}
		if location.Endpoint == "" {
			location.Endpoint = "s3." + location.Region + ".amazonaws.com"
		}
	case "r2":
		location.Region = "auto"
		if location.Endpoint == "" {
			if a.accountID == "" {
				return nil, errors.New("the r2 preset needs --account-id or --endpoint")
			}
			location.Endpoint = a.accountID + ".r2.cloudflarestorage.com"
		}
	case "b2":
		if location.Endpoint == "" {
			if location.Region == "" {
				return nil, errors.New("the b2 preset needs --region, such as us-west-004")
			}
			location.Endpoint = "s3." + location.Region + ".backblazeb2.com"
		}
	case "minio":
		if location.Endpoint == "" {
			location.Endpoint = "127.0.0.1:9000"
		}
		location.DisableSsl = true
	case "custom":
		if location.Endpoint == "" {
			return nil, errors.New("the custom preset needs --endpoint")
		}
	default:
		return nil, errors.Errorf("unknown preset %q: use one of %s", a.preset, strings.Join(s3Presets, ", "))
	}
	return location, nil
}

// readS3Credentials reads the access key from the standard environment
// variables, or else from standard input, prompting when it is a terminal.
func readS3Credentials() (*block_store_s3.Credentials, error) {
	accessKeyID, secret := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")
	if accessKeyID != "" && secret != "" {
		return &block_store_s3.Credentials{
			AccessKeyId:     accessKeyID,
			SecretAccessKey: secret,
			Token:           os.Getenv("AWS_SESSION_TOKEN"),
		}, nil
	}

	// Prompt on a terminal without echoing the secret.
	stdin := int(os.Stdin.Fd())
	if term.IsTerminal(stdin) {
		os.Stderr.WriteString("Access key ID: ")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return nil, errors.Wrap(err, "read access key id")
		}
		os.Stderr.WriteString("Secret access key: ")
		secretData, err := term.ReadPassword(stdin)
		os.Stderr.WriteString("\n")
		if err != nil {
			return nil, errors.Wrap(err, "read secret access key")
		}
		accessKeyID, secret = strings.TrimSpace(line), string(secretData)
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			accessKeyID = strings.TrimSpace(scanner.Text())
		}
		if scanner.Scan() {
			secret = strings.TrimSpace(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, errors.Wrap(err, "read credentials")
		}
	}
	if accessKeyID == "" || secret == "" {
		return nil, errors.New("access key id and secret access key are required")
	}
	return &block_store_s3.Credentials{AccessKeyId: accessKeyID, SecretAccessKey: secret}, nil
}

// newStorageListCommand builds the storage list command.
func newStorageListCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list storage backends and the Spaces placed on each",
		Flags: []cli.Flag{outputFlag()},
		Action: func(c *cli.Context) error {
			return withSession(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
				resp, err := readStorageBackends(ctx, sess)
				if err != nil {
					return err
				}
				if format := c.String("output"); format == "json" || format == "yaml" {
					data, err := resp.MarshalJSON()
					if err != nil {
						return err
					}
					return formatOutput(data, format)
				}
				if len(resp.GetStorageBackends()) == 0 {
					os.Stdout.WriteString("no storage backends; Spaces use the account's own storage\n")
					return nil
				}
				rows := [][]string{{"NAME", "BUCKET", "ENDPOINT", "SPACES", "DEFAULT"}}
				for _, info := range resp.GetStorageBackends() {
					backend := info.GetBackend()
					isDefault := ""
					if backend.GetId() == resp.GetDefaultStorageBackendId() {
						isDefault = "yes"
					}
					rows = append(rows, []string{
						backend.GetDisplayName(),
						backend.GetS3().GetBucket(),
						backend.GetS3().GetEndpoint(),
						strconv.Itoa(len(info.GetPlacedSpaces())),
						isDefault,
					})
				}
				writeTable(os.Stdout, "", rows)
				return nil
			})
		},
	}
}

// newStorageTestCommand builds the storage test command.
func newStorageTestCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "test",
		Usage:     "write, read, and delete a probe object in a backend",
		ArgsUsage: "<name>",
		Flags:     []cli.Flag{outputFlag()},
		Action: func(c *cli.Context) error {
			return withStorageBackend(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session, backend *account_settings.StorageBackend) error {
				resp, err := sess.CheckStorageBackend(ctx, &s4wave_session.CheckStorageBackendRequest{StorageBackendId: backend.GetId()})
				if err != nil {
					return errors.Wrap(err, "check storage backend")
				}
				if format := c.String("output"); format == "json" || format == "yaml" {
					data, err := resp.MarshalJSON()
					if err != nil {
						return err
					}
					if err := formatOutput(data, format); err != nil {
						return err
					}
					return checkResultError(resp.GetResult())
				}
				if err := checkResultError(resp.GetResult()); err != nil {
					return err
				}
				os.Stdout.WriteString(backend.GetDisplayName() + ": ok\n")
				return nil
			})
		},
	}
}

// newStorageDefaultCommand builds the storage default command.
func newStorageDefaultCommand(statePath *string, sessionIdx *uint) *cli.Command {
	var none bool
	return &cli.Command{
		Name:      "default",
		Usage:     "choose the backend new Spaces use",
		ArgsUsage: "<name>",
		Flags: []cli.Flag{&cli.BoolFlag{
			Name:        "none",
			Usage:       "return new Spaces to the account's own storage",
			Destination: &none,
		}},
		Action: func(c *cli.Context) error {
			if none {
				if c.Args().Present() {
					return errors.New("--none takes no backend name")
				}
				return withSession(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
					if err := sess.SetDefaultStorageBackend(ctx, ""); err != nil {
						return errors.Wrap(err, "clear default storage backend")
					}
					os.Stdout.WriteString("new Spaces use the account's own storage\n")
					return nil
				})
			}
			return withStorageBackend(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session, backend *account_settings.StorageBackend) error {
				if err := sess.SetDefaultStorageBackend(ctx, backend.GetId()); err != nil {
					return errors.Wrap(err, "set default storage backend")
				}
				os.Stdout.WriteString("new Spaces use " + backend.GetDisplayName() + "\n")
				return nil
			})
		},
	}
}

// newStorageRemoveCommand builds the storage remove command.
func newStorageRemoveCommand(statePath *string, sessionIdx *uint) *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "remove a backend that holds no Space",
		ArgsUsage: "<name>",
		Action: func(c *cli.Context) error {
			return withStorageBackend(c, *statePath, *sessionIdx, func(ctx context.Context, sess *s4wave_session.Session, backend *account_settings.StorageBackend) error {
				if err := sess.RemoveStorageBackend(ctx, backend.GetId()); err != nil {
					return errors.Wrap(err, "remove "+backend.GetDisplayName())
				}
				os.Stdout.WriteString("removed " + backend.GetDisplayName() + "\n")
				return nil
			})
		},
	}
}

// withSession connects to the daemon and mounts the selected session.
func withSession(
	c *cli.Context,
	statePath string,
	sessionIdx uint,
	fn func(ctx context.Context, sess *s4wave_session.Session) error,
) error {
	ctx := c.Context
	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return err
	}
	defer client.close()
	sess, err := client.mountSession(ctx, sessionIndex32(sessionIdx))
	if err != nil {
		return err
	}
	defer sess.Release()
	return fn(ctx, sess)
}

// withStorageBackend mounts the session and resolves the backend named by
// the first argument.
func withStorageBackend(
	c *cli.Context,
	statePath string,
	sessionIdx uint,
	fn func(ctx context.Context, sess *s4wave_session.Session, backend *account_settings.StorageBackend) error,
) error {
	name := c.Args().First()
	if name == "" {
		return errors.New("backend name required")
	}
	return withSession(c, statePath, sessionIdx, func(ctx context.Context, sess *s4wave_session.Session) error {
		backend, err := findStorageBackend(ctx, sess, name)
		if err != nil {
			return err
		}
		return fn(ctx, sess, backend)
	})
}

// findStorageBackend resolves a backend by display name or id.
func findStorageBackend(ctx context.Context, sess *s4wave_session.Session, name string) (*account_settings.StorageBackend, error) {
	resp, err := readStorageBackends(ctx, sess)
	if err != nil {
		return nil, err
	}
	for _, info := range resp.GetStorageBackends() {
		backend := info.GetBackend()
		if backend.GetDisplayName() == name || backend.GetId() == name {
			return backend, nil
		}
	}
	return nil, errors.Errorf("no storage backend named %q", name)
}

// readStorageBackends reads the current storage backends from the watch.
func readStorageBackends(ctx context.Context, sess *s4wave_session.Session) (*s4wave_session.WatchStorageBackendsResponse, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm, err := sess.WatchStorageBackends(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "watch storage backends")
	}
	resp, err := strm.Recv()
	if err != nil {
		return nil, errors.Wrap(err, "read storage backends")
	}
	return resp, nil
}

// readSpaceStorage reads where a Space's blocks are stored, or nil when the
// session's provider does not report it.
func readSpaceStorage(ctx context.Context, sess *s4wave_session.Session, spaceID string) *s4wave_session.WatchSpaceStorageResponse {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm, err := sess.WatchSpaceStorage(ctx, spaceID)
	if err != nil {
		return nil
	}
	resp, err := strm.Recv()
	if err != nil {
		return nil
	}
	return resp
}

// formatSpaceStorage describes where a Space's blocks are stored and what
// remains to upload.
func formatSpaceStorage(storage *s4wave_session.WatchSpaceStorageResponse) string {
	name := storage.GetStorageBackendName()
	if name == "" {
		return "account storage"
	}
	pending := storage.GetPendingBlocks()
	if pending == 0 {
		return name + ", uploaded"
	}
	blocks := " blocks"
	if pending == 1 {
		blocks = " block"
	}
	desc := name + ", " + strconv.FormatInt(pending, 10) + blocks + " (" +
		formatByteCount(storage.GetPendingBytes()) + ") waiting to upload"
	if uploadErr := storage.GetUploadError(); uploadErr != "" {
		desc += ": " + uploadErr
	}
	return desc
}

// formatByteCount renders a byte count with a binary unit.
func formatByteCount(size int64) string {
	if size < 1024 {
		return strconv.FormatInt(size, 10) + " B"
	}
	value := float64(size) / 1024
	for _, unit := range []string{"KiB", "MiB", "GiB"} {
		if value < 1024 {
			return strconv.FormatFloat(value, 'f', 1, 64) + " " + unit
		}
		value /= 1024
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " TiB"
}

// checkResultError explains a failed connectivity check, or returns nil.
func checkResultError(result *block_store_s3.CheckResult) error {
	var cause string
	switch result.GetOutcome() {
	case block_store_s3.CheckOutcome_CHECK_OUTCOME_OK:
		return nil
	case block_store_s3.CheckOutcome_CHECK_OUTCOME_UNREACHABLE:
		cause = "cannot reach the endpoint"
	case block_store_s3.CheckOutcome_CHECK_OUTCOME_CREDENTIALS_REJECTED:
		cause = "the endpoint rejected the access key"
	case block_store_s3.CheckOutcome_CHECK_OUTCOME_ACCESS_DENIED:
		cause = "the access key cannot write to this bucket"
	case block_store_s3.CheckOutcome_CHECK_OUTCOME_BUCKET_NOT_FOUND:
		cause = "the bucket does not exist"
	case block_store_s3.CheckOutcome_CHECK_OUTCOME_WRONG_REGION:
		cause = "the bucket is in a different region"
	default:
		cause = "the check failed"
	}
	if detail := result.GetDetail(); detail != "" {
		cause += ": " + detail
	}
	return errors.New("storage check failed: " + cause)
}
