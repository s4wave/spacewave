package os_image

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

const (
	// CardSigners names the card file listing the accepted signers, one peer
	// ID per line. First boot pins it; later boots never read it.
	CardSigners = "signers"
	// CardAuthorization names the card file holding the image authorization
	// as packedmsg text.
	CardAuthorization = "authorization"
)

// WriteCard writes the signers file and the authorization of root, signed by
// key, into the card directory dir. The signer set is key's peer alone.
func WriteCard(dir string, key crypto.PrivKey, root *block.BlockRef) error {
	// Sign before writing anything, so a refused root leaves no card files.
	signed, err := SignImageRoot(key, root)
	if err != nil {
		return err
	}
	data, err := signed.MarshalVT()
	if err != nil {
		return err
	}
	signer, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return err
	}

	// Write the signers, then the authorization that only they can verify.
	if err := WriteSigners(filepath.Join(dir, CardSigners), []peer.ID{signer}); err != nil {
		return err
	}
	return writeFileSync(filepath.Join(dir, CardAuthorization), []byte(packedmsg.EncodePackedMessage(data)+"\n"))
}

// ReadAuthorization reads the packedmsg image authorization at path.
func ReadAuthorization(path string) (*peer.SignedMsg, error) {
	// Decode the packed text, which carries its own checksum.
	text, err := os.ReadFile(path) //nolint:gosec // The boot reads its own card.
	if err != nil {
		return nil, errors.Wrap(err, "image authorization")
	}
	data, ok := packedmsg.DecodePackedMessage(strings.TrimSpace(string(text)))
	if !ok {
		return nil, errors.New("image authorization: not a packed message")
	}

	// Unmarshal the signed message; the caller verifies it.
	signed, err := peer.UnmarshalSignedMsg(data)
	if err != nil {
		return nil, errors.Wrap(err, "image authorization")
	}
	return signed, nil
}

// ReadSigners reads a signers file: one peer ID per line, at least one.
func ReadSigners(path string) ([]peer.ID, error) {
	// Read the whole file, which holds a few short lines.
	text, err := os.ReadFile(path) //nolint:gosec // The boot reads its own card and Volume.
	if err != nil {
		return nil, errors.Wrap(err, "signers")
	}

	// Parse each non-empty line as a peer ID.
	var ids []peer.ID
	for line := range strings.Lines(string(text)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id, err := peer.IDB58Decode(line)
		if err != nil {
			return nil, errors.Wrapf(err, "signers: %q", line)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.Errorf("signers: %s names no signer", path)
	}
	return ids, nil
}

// WriteSigners writes ids to path, one per line, and syncs it before it
// returns. It writes a temporary file and renames it over path, so a power
// cut leaves either no file or the whole list.
func WriteSigners(path string, ids []peer.ID) error {
	// Refuse an empty set, which would authorize nothing and read back as an error.
	if len(ids) == 0 {
		return errors.New("signers: no signer to write")
	}
	var text strings.Builder
	for _, id := range ids {
		text.WriteString(id.String())
		text.WriteByte('\n')
	}
	return writeFileSync(path, []byte(text.String()))
}

// writeFileSync replaces path with data through a synced temporary file and
// syncs the directory, so the rename survives a power cut.
func writeFileSync(path string, data []byte) error {
	// Write and sync the temporary file beside path.
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // The caller names its own card or Volume file.
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	// Rename it over path and sync the directory entry.
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	if err := dir.Close(); syncErr == nil {
		syncErr = err
	}
	return syncErr
}
