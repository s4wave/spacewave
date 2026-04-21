package cliutil

import (
	"context"

	ucli "github.com/aperturerobotics/cli"
	"github.com/sirupsen/logrus"
)

// UtilArgs contains the utility arguments and functions.
type UtilArgs struct {
	// le is the logger entry
	le *logrus.Entry
	// ctx is the context
	ctx context.Context

	// FilePath is the file path to operate on.
	FilePath string
	// OutPath is the file path to write to.
	OutPath string

	// KeySize is the key size to generate.
	KeySize int

	// Timestamp is the input timestamp string
	Timestamp string
}

// BuildFlags attaches the flags to a flag set.
func (a *UtilArgs) BuildFlags() []ucli.Flag {
	return []ucli.Flag{}
}

// BuildCommands attaches the commands.
func (a *UtilArgs) BuildCommands() []*ucli.Command {
	return []*ucli.Command{
		{
			Name:   "ulid",
			Usage:  "generates a lowercase ulid",
			Action: a.RunULID,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "out",
					Aliases:     []string{"o"},
					Usage:       "file to store the generated ulid",
					Destination: &a.OutPath,
				},
			},
		},
		{
			Name:   "generate-private",
			Usage:  "generates a private key .pem file",
			Action: a.RunGeneratePrivate,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "out",
					Aliases:     []string{"o"},
					Usage:       "file to store pem formatted private key",
					Destination: &a.OutPath,
				},
			},
		},
		{
			Name:   "read-private",
			Usage:  "loads a private key pem and prints the peer id",
			Action: a.RunReadPrivatePeerId,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "file",
					Aliases:     []string{"f"},
					Usage:       "file to load pem formatted private key",
					Destination: &a.FilePath,
				},
			},
		},
		{
			Name:   "read-public",
			Usage:  "loads a public key pem and prints the peer id",
			Action: a.RunReadPublicPeerId,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "file",
					Aliases:     []string{"f"},
					Usage:       "file to load pem formatted public key",
					Destination: &a.FilePath,
				},
			},
		},
		{
			Name:   "derive-public",
			Usage:  "loads a private key pem and writes a public key",
			Action: a.RunDerivePublic,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "file",
					Aliases:     []string{"f"},
					Usage:       "file to load pem formatted private key",
					Destination: &a.FilePath,
				},
				&ucli.StringFlag{
					Name:        "out",
					Aliases:     []string{"o"},
					Usage:       "file to store pem formatted public key",
					Destination: &a.OutPath,
				},
			},
		},
		{
			Name:   "derive-ssh-public",
			Usage:  "loads a public or private key pem and writes a ssh public key",
			Action: a.RunDeriveSshPublic,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "file",
					Aliases:     []string{"f"},
					Usage:       "file to load pem formatted private or public key",
					Destination: &a.FilePath,
				},
				&ucli.StringFlag{
					Name:        "out",
					Aliases:     []string{"o"},
					Usage:       "file to store SSH pem formatted public key",
					Destination: &a.OutPath,
				},
			},
		},
		{
			Name:   "generate-crypto-key",
			Usage:  "generates a base64 crypto key",
			Action: a.RunGenerateCryptoKey,
			Flags: []ucli.Flag{
				&ucli.IntFlag{
					Name:        "key-size",
					Aliases:     []string{"size", "s"},
					Usage:       "bytes size of the key to generate",
					Destination: &a.KeySize,
					Value:       32,
				},
				&ucli.StringFlag{
					Name:        "out",
					Aliases:     []string{"o"},
					Usage:       "file to store base64 formatted key",
					Destination: &a.OutPath,
				},
			},
		},
		{
			Name:   "timestamp",
			Usage:  "prints a timestamp in RFC3339 format",
			Action: a.RunTimestamp,
			Flags: []ucli.Flag{
				&ucli.StringFlag{
					Name:        "timestamp",
					Aliases:     []string{"t"},
					Usage:       "RFC3339 timestamp or unix milliseconds: if empty, uses current utc time",
					Destination: &a.Timestamp,
				},
			},
		},
	}
}

// SetContext sets the context.
func (a *UtilArgs) SetContext(c context.Context) {
	a.ctx = c
}

// GetContext returns the context.
func (a *UtilArgs) GetContext() context.Context {
	if c := a.ctx; c != nil {
		return c
	}
	return context.TODO()
}

// SetLogger sets the root log entry.
func (a *UtilArgs) SetLogger(le *logrus.Entry) {
	a.le = le
}

// GetLogger returns the log entry
func (a *UtilArgs) GetLogger() *logrus.Entry {
	if le := a.le; le != nil {
		return le
	}
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(log)
}
