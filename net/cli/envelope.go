package cli

import (
	"crypto/rand"
	"io"
	"math"
	"os"

	"github.com/aperturerobotics/cli"
	jsoniter "github.com/aperturerobotics/json-iterator-lite"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/keypem/keyfile"
	"github.com/sirupsen/logrus"
)

// EnvelopeArgs contains the arguments for envelope commands.
type EnvelopeArgs struct {
	// KeyPaths is the list of key file paths.
	KeyPaths cli.StringSlice
	// Context is the application context string for the envelope.
	Context string
	// Threshold is the number of shares needed minus one.
	Threshold uint
	// InputPath is the input file path (default: stdin).
	InputPath string
	// OutputPath is the output file path (default: stdout).
	OutputPath string
}

// BuildFlags returns the common CLI flags for envelope commands.
func (a *EnvelopeArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			Name:        "key",
			Aliases:     []string{"k"},
			Usage:       "path to key file (can be specified multiple times)",
			Destination: &a.KeyPaths,
			Required:    true,
		},
		&cli.StringFlag{
			Name:        "context",
			Aliases:     []string{"ctx"},
			Usage:       "application context string for the envelope",
			Value:       "bifrost/cli envelope v1",
			Destination: &a.Context,
		},
		&cli.UintFlag{
			Name:        "threshold",
			Aliases:     []string{"t"},
			Usage:       "threshold parameter (need threshold+1 shares to unseal)",
			Destination: &a.Threshold,
		},
		&cli.StringFlag{
			Name:        "input",
			Aliases:     []string{"i"},
			Usage:       "input file path (default: stdin)",
			Destination: &a.InputPath,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output file path (default: stdout)",
			Destination: &a.OutputPath,
		},
	}
}

// BuildCommands returns the envelope subcommands.
func (a *EnvelopeArgs) BuildCommands() []*cli.Command {
	return []*cli.Command{
		{
			Name:   "seal",
			Usage:  "seal data into an envelope encrypted to the given keys",
			Action: a.RunSeal,
			Flags:  a.BuildFlags(),
		},
		{
			Name:   "unseal",
			Usage:  "unseal an envelope using the given private keys",
			Action: a.RunUnseal,
			Flags: append(a.BuildFlags(), &cli.BoolFlag{
				Name:  "info",
				Usage: "print envelope info (shares needed, available) instead of payload",
			}),
		},
	}
}

// readInput reads input from the specified path or stdin.
func (a *EnvelopeArgs) readInput() ([]byte, error) {
	if a.InputPath != "" {
		return os.ReadFile(a.InputPath)
	}
	return io.ReadAll(os.Stdin)
}

// writeOutput writes output to the specified path or stdout.
func (a *EnvelopeArgs) writeOutput(data []byte) error {
	if a.OutputPath != "" {
		return os.WriteFile(a.OutputPath, data, 0o600)
	}
	_, err := os.Stdout.Write(data)
	return err
}

// loadPubKeys loads public keys from the key file paths.
func (a *EnvelopeArgs) loadPubKeys() ([]crypto.PubKey, error) {
	paths := a.KeyPaths.Value()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetOutput(io.Discard)

	keys := make([]crypto.PubKey, 0, len(paths))
	for _, path := range paths {
		// Try loading as private key first (to extract public key).
		priv, err := keyfile.OpenOrWritePrivKey(le, path)
		if err != nil {
			return nil, errors.Wrapf(err, "load key %s", path)
		}
		keys = append(keys, priv.GetPublic())
	}
	return keys, nil
}

// loadPrivKeys loads private keys from the key file paths.
func (a *EnvelopeArgs) loadPrivKeys() ([]crypto.PrivKey, error) {
	paths := a.KeyPaths.Value()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetOutput(io.Discard)

	keys := make([]crypto.PrivKey, 0, len(paths))
	for _, path := range paths {
		priv, err := keyfile.OpenOrWritePrivKey(le, path)
		if err != nil {
			// Try reading as PEM.
			dat, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, errors.Wrapf(err, "load key %s", path)
			}
			priv, readErr = keypem.ParsePrivKeyPem(dat)
			if readErr != nil || priv == nil {
				return nil, errors.Wrapf(err, "load key %s", path)
			}
		}
		keys = append(keys, priv)
	}
	return keys, nil
}

// RunSeal seals data into an envelope.
func (a *EnvelopeArgs) RunSeal(_ *cli.Context) error {
	pubKeys, err := a.loadPubKeys()
	if err != nil {
		return err
	}

	payload, err := a.readInput()
	if err != nil {
		return errors.Wrap(err, "read input")
	}
	if len(payload) == 0 {
		return errors.New("input is empty")
	}

	// Build grant configs: one grant per key, one share each.
	grants := make([]*envelope.EnvelopeGrantConfig, len(pubKeys))
	for i := range pubKeys {
		grants[i] = &envelope.EnvelopeGrantConfig{
			ShareCount:     1,
			KeypairIndexes: []uint32{uint32(i)}, //nolint:gosec // i bounded by pubKeys slice length
		}
	}

	if a.Threshold > math.MaxUint32 {
		return errors.New("threshold exceeds maximum value")
	}

	env, err := envelope.BuildEnvelope(
		rand.Reader,
		a.Context,
		payload,
		pubKeys,
		&envelope.EnvelopeConfig{
			Threshold:    uint32(a.Threshold), //nolint:gosec // bounds checked above
			GrantConfigs: grants,
		},
	)
	if err != nil {
		return errors.Wrap(err, "build envelope")
	}

	data, err := env.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal envelope")
	}

	return a.writeOutput(data)
}

// RunUnseal unseals an envelope.
func (a *EnvelopeArgs) RunUnseal(c *cli.Context) error {
	privKeys, err := a.loadPrivKeys()
	if err != nil {
		return err
	}

	data, err := a.readInput()
	if err != nil {
		return errors.Wrap(err, "read input")
	}

	env := &envelope.Envelope{}
	if err := env.UnmarshalVT(data); err != nil {
		return errors.Wrap(err, "unmarshal envelope")
	}

	payload, result, err := envelope.UnlockEnvelope(a.Context, env, privKeys)
	if err != nil {
		return errors.Wrap(err, "unlock envelope")
	}

	if c.Bool("info") {
		dat, err := marshalEnvelopeInfoJSON(env, result)
		if err != nil {
			return err
		}
		dat = append(dat, '\n')
		return a.writeOutput(dat)
	}

	if !result.GetSuccess() {
		return errors.Errorf(
			"insufficient shares: have %d, need %d",
			result.GetSharesAvailable(),
			result.GetSharesNeeded(),
		)
	}

	return a.writeOutput(payload)
}

// marshalEnvelopeInfoJSON marshals envelope unlock info as JSON.
func marshalEnvelopeInfoJSON(
	env *envelope.Envelope,
	result *envelope.EnvelopeUnlockResult,
) ([]byte, error) {
	s := jsoniter.NewStream(nil, 256, 2)
	s.WriteObjectStart()
	s.WriteObjectField("success")
	s.WriteBool(result.GetSuccess())
	s.WriteMore()
	s.WriteObjectField("shares_available")
	s.WriteUint32(result.GetSharesAvailable())
	s.WriteMore()
	s.WriteObjectField("shares_needed")
	s.WriteUint32(result.GetSharesNeeded())
	s.WriteMore()
	s.WriteObjectField("unlocked_grants")
	s.WriteArrayStart()
	for i, idx := range result.GetUnlockedGrantIndexes() {
		if i != 0 {
			s.WriteMore()
		}
		s.WriteUint32(idx)
	}
	s.WriteArrayEnd()
	s.WriteMore()
	s.WriteObjectField("total_grants")
	s.WriteInt(len(env.GetGrants()))
	s.WriteMore()
	s.WriteObjectField("total_keypairs")
	s.WriteInt(len(env.GetKeypairs()))
	s.WriteMore()
	s.WriteObjectField("threshold")
	s.WriteUint32(env.GetThreshold())
	s.WriteMore()
	s.WriteObjectField("envelope_id")
	s.WriteString(env.GetEnvelopeId())
	s.WriteObjectEnd()
	if s.Error != nil {
		return nil, s.Error
	}
	return s.Buffer(), nil
}
