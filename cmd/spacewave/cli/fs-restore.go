//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/session"
	space_backup "github.com/s4wave/spacewave/core/space/backup"
	block_store_ftps "github.com/s4wave/spacewave/db/block/store/ftps"
	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_sftp "github.com/s4wave/spacewave/db/block/store/sftp"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// FsRestoreArgs selects one immutable object store and an entity credential.
type FsRestoreArgs struct {
	// rootFile is an existing, decrypted export-root recovery file.
	rootFile string
	// snapshot names a remote sealed index, empty for the newest.
	snapshot string
	// endpoint is the selected S3 endpoint hostname.
	endpoint string
	// region is the S3 signing region.
	region string
	// bucket is the S3 bucket.
	bucket string
	// prefix is the per-Space object prefix.
	prefix string
	// objectKey is the UnixFS object to restore.
	objectKey string
	// identity is password or a PEM entity key path.
	identity string
	// gdriveFolder is the selected Drive root folder ID.
	gdriveFolder string
	// sftpHost is the selected SFTP host and optional port.
	sftpHost string
	// ftpsHost is the selected explicit-TLS FTP host and optional port.
	ftpsHost string
	// root is the remote server's bucket directory.
	root string
	// netrc is the remote server's credential file.
	netrc string
	// hostKey pins the SFTP host identity.
	hostKey string
	// insecure connects to a local test S3 endpoint without TLS.
	insecure bool
	// verifyLFS verifies every restored file against its SHA-256 OID.
	verifyLFS bool
}

// BuildFlags builds the standalone recovery flags, without a Session selector.
func (a *FsRestoreArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "root-file", Usage: "decrypted export-root recovery file (contains decryption material)", Destination: &a.rootFile},
		&cli.StringFlag{Name: "snapshot", Usage: "sealed index object key; default is the newest complete index", Destination: &a.snapshot},
		&cli.StringFlag{Name: "s3-endpoint", Usage: "S3 endpoint hostname", Destination: &a.endpoint},
		&cli.StringFlag{Name: "s3-region", Usage: "S3 signing region", Destination: &a.region},
		&cli.StringFlag{Name: "s3-bucket", Usage: "S3 bucket", Destination: &a.bucket},
		&cli.StringFlag{Name: "gdrive-folder", Usage: "Google Drive backup root folder ID; authorizes access by device sign-in", Destination: &a.gdriveFolder},
		&cli.StringFlag{Name: "sftp-host", Usage: "SFTP host[:port]", Destination: &a.sftpHost},
		&cli.StringFlag{Name: "ftps-host", Usage: "explicit-TLS FTP host[:port]", Destination: &a.ftpsHost},
		&cli.StringFlag{Name: "root", Usage: "SFTP or FTPS backup directory", Destination: &a.root},
		&cli.StringFlag{Name: "netrc", Usage: "SFTP or FTPS credential file", Destination: &a.netrc},
		&cli.StringFlag{Name: "sftp-host-key", Usage: "pinned SFTP host public key or SHA256 fingerprint", Destination: &a.hostKey},
		&cli.StringFlag{Name: "prefix", Usage: "per-Space prefix containing packs/, entries/ and snapshots/", Required: true, Destination: &a.prefix},
		&cli.StringFlag{Name: "object", Usage: "UnixFS object key", Required: true, Destination: &a.objectKey},
		&cli.StringFlag{Name: "identity", Usage: "password to prompt for the account password, or a PEM entity key file", Destination: &a.identity},
		&cli.BoolFlag{Name: "verify-lfs", Usage: "verify every restored file against its SHA-256 LFS OID", Destination: &a.verifyLFS},
		&cli.BoolFlag{Name: "insecure", Usage: "connect to a test S3 endpoint without TLS", Destination: &a.insecure},
	}
}

// Run restores into a new directory without opening any account or device state.
func (a *FsRestoreArgs) Run(c *cli.Context) error {
	// Validate the new destination and require exactly one storage location.
	if c.NArg() != 1 {
		return errors.New("one new destination directory is required")
	}
	destination := c.Args().First()
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return errors.New("restore destination already exists; choose a new directory")
	}
	locations := 0
	for _, selected := range []bool{a.endpoint != "", a.gdriveFolder != "", a.sftpHost != "", a.ftpsHost != ""} {
		if selected {
			locations++
		}
	}
	if locations != 1 {
		return errors.New("choose exactly one of --s3-endpoint, --gdrive-folder, --sftp-host or --ftps-host")
	}

	// Read only the entity credential the person explicitly selected.
	identity := &session.EntityCredential{}
	var root *s4wave_world.WorldRootSnapshot
	if a.rootFile != "" {
		if a.snapshot != "" || a.identity != "" {
			return errors.New("choose --root-file or a sealed --snapshot with --identity")
		}
		data, err := os.ReadFile(a.rootFile)
		if err != nil {
			return err
		}
		root = &s4wave_world.WorldRootSnapshot{}
		if err := root.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "read decrypted recovery root")
		}
	} else if a.identity == "password" {
		password, err := readSecret("Account password")
		if err != nil {
			return err
		}
		identity.Credential = &session.EntityCredential_Password{Password: password}
	} else {
		if a.identity == "" {
			return errors.New("choose --identity=password or --identity=PEM_FILE")
		}
		pem, err := os.ReadFile(a.identity)
		if err != nil {
			return err
		}
		identity.Credential = &session.EntityCredential_PemPrivateKey{PemPrivateKey: pem}
	}
	objects, bucket, release, err := a.openObjects(c.Context)
	if err != nil {
		return err
	}
	defer release()

	// Recover directly from immutable remote packs, with no local mutable index.
	prefix := strings.Trim(a.prefix, "/") + "/"
	verified, err := space_backup.Restore(c.Context, space_backup.RestoreOptions{
		Log: logrus.NewEntry(logrus.New()), Objects: objects, Bucket: bucket, Prefix: prefix,
		Root: root, Snapshot: a.snapshot, Identity: identity, Object: a.objectKey, Destination: destination, VerifyLFS: a.verifyLFS,
	})
	if err != nil {
		return err
	}
	message := "Restored " + destination
	if a.verifyLFS {
		message += "; verified " + strconv.Itoa(verified) + " LFS files"
	}
	_, err = os.Stdout.WriteString(message + "\n")
	return err
}

// openObjects authenticates only the selected object store and never a Space.
func (a *FsRestoreArgs) openObjects(ctx context.Context) (block_store_s3.ObjectStore, string, func(), error) {
	// Drive authorizes into memory only: a restore creates no local credential file.
	if a.gdriveFolder != "" {
		conf, err := block_store_gdrive.OAuthConfig()
		if err != nil {
			return nil, "", nil, err
		}
		credentials, err := block_store_gdrive.Authorize(ctx, conf, func(uri, code string) error {
			_, err := os.Stdout.WriteString("Open " + uri + "\nEnter code: " + code + "\nWaiting for Google Drive sign-in…\n")
			return err
		})
		if err != nil {
			return nil, "", nil, err
		}
		httpClient, err := block_store_gdrive.AuthorizedHTTP(ctx, conf, credentials)
		if err != nil {
			return nil, "", nil, err
		}
		client, err := block_store_gdrive.NewClient(httpClient, a.gdriveFolder, "")
		return client, "", func() {}, err
	}

	// S3 credentials use the established environment or terminal input path.
	if a.endpoint != "" {
		credentials, err := readS3Credentials()
		if err != nil {
			return nil, "", nil, err
		}
		client, err := block_store_s3.BuildClient(&block_store_s3.ClientConfig{
			Endpoint: a.endpoint, Region: a.region, Credentials: credentials, DisableSsl: a.insecure,
		})
		return client, a.bucket, func() {}, err
	}

	// FTP and SSH use the same host-local netrc convention as the storage backend.
	host := a.ftpsHost
	if a.sftpHost != "" {
		host = a.sftpHost
	}
	machine := host
	if name, _, err := net.SplitHostPort(host); err == nil {
		machine = name
	}

	// Open the selected protocol with its existing netrc credential convention.
	logins, err := block_store_ftps.ReadNetrc(a.netrc, machine)
	if err != nil {
		return nil, "", nil, err
	}
	if a.sftpHost != "" {
		if _, _, err := net.SplitHostPort(host); err != nil {
			host = net.JoinHostPort(host, "22")
		}
		client, err := block_store_sftp.NewClient(block_store_sftp.Config{
			Addr: host, User: logins[0].User, Password: logins[0].Password, HostKey: a.hostKey,
		})
		if err != nil {
			return nil, "", nil, err
		}
		return client, a.root, client.Close, nil
	}
	client := block_store_ftps.NewClient(block_store_ftps.Config{Addr: host, Logins: logins})
	return client, a.root, client.Close, nil
}

// newFsRestoreCommand builds one standalone restore path for every object store.
func newFsRestoreCommand() *cli.Command {
	args := &FsRestoreArgs{}
	return &cli.Command{Name: "restore", Usage: "restore an encrypted Space backup into a new directory", ArgsUsage: "NEW_DIRECTORY",
		Description: "Needs the backup location, its storage authorization, and an account password or PEM entity key.\n" +
			"Needs no device, running daemon, or Session state. Drive receives only ciphertext.",
		Flags: args.BuildFlags(), Action: args.Run}
}
