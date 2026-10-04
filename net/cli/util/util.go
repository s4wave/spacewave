package cliutil

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	util_ulid "github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	peer_ssh "github.com/s4wave/spacewave/net/peer/ssh"
	"github.com/s4wave/spacewave/net/util/confparse"
	"golang.org/x/crypto/ssh"
)

// RunULID runs the ulid util command.
func (a *UtilArgs) RunULID(_ *cli.Context) error {
	return writeIfNotExists(a.OutPath, bytes.NewReader([]byte(util_ulid.NewULID()+"\n")))
}

// RunTimestamp runs the timestamp util command.
func (a *UtilArgs) RunTimestamp(_ *cli.Context) error {
	var ts *timestamppb.Timestamp
	if a.Timestamp != "" {
		var err error
		ts, err = confparse.ParseTimestamp(a.Timestamp)
		if err != nil {
			return err
		}
	} else {
		ts = timestamppb.Now()
	}

	formatted := ts.AsRFC3339() + "\n"
	return writeIfNotExists(a.OutPath, bytes.NewReader([]byte(formatted)))
}

// RunGeneratePrivate writes a newly generated peer private key.
func (a *UtilArgs) RunGeneratePrivate(_ *cli.Context) error {
	// Generate a peer identity for the output key.
	npeer, err := peer.NewPeer(nil)
	if err != nil {
		return err
	}

	// Read the peer's private key bytes.
	priv, err := npeer.GetPrivKey(a.GetContext())
	if err != nil {
		return err
	}

	// Encode the private key as PEM.
	pemd, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return err
	}

	// Write the PEM file without replacing an existing destination.
	err = writeIfNotExists(a.OutPath, bytes.NewReader(pemd))
	if err != nil {
		return err
	}

	// Report the generated peer identity.
	le := a.GetLogger()
	le.Infof("generated private key: %s", npeer.GetPeerID().String())
	return nil
}

// RunReadPublicPeerId loads a public key and prints the peer ID.
func (a *UtilArgs) RunReadPublicPeerId(_ *cli.Context) error {
	rp, err := a.readInputFilePubKey()
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(rp.GetPeerID().String() + "\n")
	return err
}

// RunReadPrivatePeerId loads a private key and prints the peer ID.
func (a *UtilArgs) RunReadPrivatePeerId(_ *cli.Context) error {
	rp, err := a.readInputFilePrivKey()
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(rp.GetPeerID().String() + "\n")
	return err
}

// RunDerivePublic writes a public-key PEM derived from a private PEM.
func (a *UtilArgs) RunDerivePublic(_ *cli.Context) error {
	// Load the input private key.
	rp, err := a.readInputFilePrivKey()
	if err != nil {
		return err
	}

	// Encode its public key as PEM.
	pemd, err := keypem.MarshalPubKeyPem(rp.GetPubKey())
	if err != nil {
		return err
	}

	// Write the derived public key without replacing existing output.
	err = writeIfNotExists(a.OutPath, bytes.NewReader(pemd))
	if err != nil {
		return err
	}
	return nil
}

// RunDeriveSshPublic writes an authorized SSH public key from a peer key.
func (a *UtilArgs) RunDeriveSshPublic(_ *cli.Context) error {
	// Load the input public key.
	rp, err := a.readInputFilePubKey()
	if err != nil {
		return err
	}

	// Convert the peer public key to SSH format.
	pkey, err := peer_ssh.NewPublicKey(rp.GetPubKey())
	if err != nil {
		return err
	}

	// Encode the SSH key in authorized_keys format.
	dat := ssh.MarshalAuthorizedKey(pkey)

	// Write the encoded key without replacing existing output.
	err = writeIfNotExists(a.OutPath, bytes.NewReader(dat))
	if err != nil {
		return err
	}
	return nil
}

// RunGenerateCryptoKey writes a random cryptographic key as base64.
func (a *UtilArgs) RunGenerateCryptoKey(_ *cli.Context) error {
	// Resolve the requested key size or use the default.
	keySize := a.KeySize
	if keySize == 0 {
		keySize = 32
	}

	// Generate cryptographically random key bytes.
	buf := make([]byte, keySize)
	if _, err := rand.Read(buf); err != nil {
		return err
	}

	// Encode and write the key without replacing existing output.
	bufB64 := base64.StdEncoding.EncodeToString(buf) + "\n"
	err := writeIfNotExists(a.OutPath, bytes.NewReader([]byte(bufB64)))
	if err != nil {
		return err
	}

	// Report the generated key size.
	le := a.GetLogger()
	le.Infof("generated crypto key of length %v", keySize)
	return nil
}

// readInputFile reads the input file path or stdin.
func (a *UtilArgs) readInputFile() ([]byte, error) {
	if fp := a.FilePath; fp != "" {
		return os.ReadFile(fp)
	}

	return io.ReadAll(os.Stdin)
}

// readInputFilePrivKey reads and parses a private key from the configured input.
func (a *UtilArgs) readInputFilePrivKey() (peer.Peer, error) {
	// Read private-key bytes from the configured file or standard input.
	dat, err := a.readInputFile()
	if err != nil {
		return nil, err
	}

	// Parse the private-key PEM.
	key, err := keypem.ParsePrivKeyPem(dat)
	if err != nil {
		return nil, err
	}

	// Construct and report the peer identity.
	le := a.GetLogger()
	npeer, err := peer.NewPeer(key)
	if err != nil {
		return nil, err
	}
	le.Debugf("loaded private key: %s", npeer.GetPeerID().String())
	return npeer, nil
}

// readInputFilePubKey reads and parses a public key from the configured input.
func (a *UtilArgs) readInputFilePubKey() (peer.Peer, error) {
	// Read public-key bytes from the configured file or standard input.
	dat, err := a.readInputFile()
	if err != nil {
		return nil, err
	}

	// Parse the public-key PEM.
	key, err := keypem.ParsePubKeyPem(dat)
	if err != nil {
		return nil, err
	}

	// Construct and report the peer identity.
	le := a.GetLogger()
	npeer, err := peer.NewPeerWithPubKey(key)
	if err != nil {
		return nil, err
	}
	le.Debugf("loaded public key: %s", npeer.GetPeerID().String())
	return npeer, nil
}

// writeIfNotExists writes input to a new file or to standard output.
func writeIfNotExists(outPath string, input io.Reader) error {
	// Initialize the selected output destination.
	var of *os.File
	var out io.Writer
	if outPath != "" {
		// Reject a destination that already exists.
		_, err := os.Stat(outPath)
		if !os.IsNotExist(err) {
			return errors.Wrap(os.ErrExist, outPath)
		}

		// Open the new destination and retain its cleanup.
		of, err = os.OpenFile(outPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		out = of
		defer of.Close()

		// Require the newly opened destination to be empty.
		if pos, err := of.Seek(0, io.SeekEnd); err != nil || pos != 0 {
			if err == nil {
				// The destination contained data before this write.
				return errors.Wrap(os.ErrExist, outPath)
			}
			return err
		}
	} else {
		out = os.Stdout
	}

	// Copy the generated value to the selected output.
	if _, err := io.Copy(out, input); err != nil {
		return err
	}

	// Close a file destination after the write.
	if of != nil {
		return of.Close()
	}
	return nil
}
