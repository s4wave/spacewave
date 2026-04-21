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

// RunGeneratePrivate runs the generate-private util command.
func (a *UtilArgs) RunGeneratePrivate(_ *cli.Context) error {
	npeer, err := peer.NewPeer(nil)
	if err != nil {
		return err
	}

	priv, err := npeer.GetPrivKey(a.GetContext())
	if err != nil {
		return err
	}

	pemd, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		return err
	}
	err = writeIfNotExists(a.OutPath, bytes.NewReader(pemd))
	if err != nil {
		return err
	}
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

// RunDerivePublic derives the public key from a private pem.
func (a *UtilArgs) RunDerivePublic(_ *cli.Context) error {
	rp, err := a.readInputFilePrivKey()
	if err != nil {
		return err
	}
	pemd, err := keypem.MarshalPubKeyPem(rp.GetPubKey())
	if err != nil {
		return err
	}
	err = writeIfNotExists(a.OutPath, bytes.NewReader(pemd))
	if err != nil {
		return err
	}
	return nil
}

// RunDerivePublic derives the ssh public key from a private or public pem.
func (a *UtilArgs) RunDeriveSshPublic(_ *cli.Context) error {
	rp, err := a.readInputFilePubKey()
	if err != nil {
		return err
	}
	pkey, err := peer_ssh.NewPublicKey(rp.GetPubKey())
	if err != nil {
		return err
	}
	dat := ssh.MarshalAuthorizedKey(pkey)
	err = writeIfNotExists(a.OutPath, bytes.NewReader(dat))
	if err != nil {
		return err
	}
	return nil
}

// RunGenerateCryptoKey runs the generate-crypto-key util command.
func (a *UtilArgs) RunGenerateCryptoKey(_ *cli.Context) error {
	keySize := a.KeySize
	if keySize == 0 {
		keySize = 32
	}

	buf := make([]byte, keySize)
	if _, err := rand.Read(buf); err != nil {
		return err
	}

	bufB64 := base64.StdEncoding.EncodeToString(buf) + "\n"
	err := writeIfNotExists(a.OutPath, bytes.NewReader([]byte(bufB64)))
	if err != nil {
		return err
	}
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

// readInputFilePrivKey reads the input file path or stdin.
func (a *UtilArgs) readInputFilePrivKey() (peer.Peer, error) {
	dat, err := a.readInputFile()
	if err != nil {
		return nil, err
	}

	key, err := keypem.ParsePrivKeyPem(dat)
	if err != nil {
		return nil, err
	}

	le := a.GetLogger()
	npeer, err := peer.NewPeer(key)
	if err != nil {
		return nil, err
	}
	le.Debugf("loaded private key: %s", npeer.GetPeerID().String())
	return npeer, nil
}

// readInputFilePubKey reads the input file path or stdin.
func (a *UtilArgs) readInputFilePubKey() (peer.Peer, error) {
	dat, err := a.readInputFile()
	if err != nil {
		return nil, err
	}

	key, err := keypem.ParsePubKeyPem(dat)
	if err != nil {
		return nil, err
	}

	le := a.GetLogger()
	npeer, err := peer.NewPeerWithPubKey(key)
	if err != nil {
		return nil, err
	}
	le.Debugf("loaded public key: %s", npeer.GetPeerID().String())
	return npeer, nil
}

func writeIfNotExists(outPath string, input io.Reader) error {
	var of *os.File
	var out io.Writer
	if outPath != "" {
		_, err := os.Stat(outPath)
		if !os.IsNotExist(err) {
			return errors.Wrap(os.ErrExist, outPath)
		}
		of, err = os.OpenFile(outPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return err
		}
		out = of
		defer of.Close()
		if pos, err := of.Seek(0, io.SeekEnd); err != nil || pos != 0 {
			if err == nil {
				// file must have existed
				return errors.Wrap(os.ErrExist, outPath)
			}
			return err
		}
	} else {
		out = os.Stdout
	}
	if _, err := io.Copy(out, input); err != nil {
		return err
	}
	if of != nil {
		return of.Close()
	}
	return nil
}
