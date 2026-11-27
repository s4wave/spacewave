package os_image

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/peer"
)

const (
	// CardSigners identifies the card file listing the accepted signers, one
	// peer ID per line. First boot pins it. Later boots never read it.
	CardSigners = "signers"
	// AuthorizationFile identifies the file holding an image authorization as
	// packedmsg text: the sign Step's output, which install publishes into
	// the image store beside the root it authorizes.
	AuthorizationFile = "authorization"
	// CardGrant identifies the card file holding the bootstrap grant, an
	// untargeted Space invite, as packedmsg text.
	CardGrant = "grant"
)

// WriteAuthorization writes the image authorization signed to path as the
// packedmsg text ReadAuthorization reads.
func WriteAuthorization(path string, signed *peer.SignedMsg) error {
	data, err := signed.MarshalVT()
	if err != nil {
		return err
	}
	return writePacked(path, data)
}

// InstallCard writes an installable card into dir: the Space's accepted
// signers and its bootstrap grant. It writes nothing unless a listed signer
// authorized exactly image, its root and its transform, and the grant can
// enroll a key made on the machine, so a card that could never boot or enroll is refused at install
// rather than at first boot. The caller then publishes the authorization into
// the image store, last, so a card cut short at install authorizes nothing.
func InstallCard(dir string, signers []peer.ID, signed *peer.SignedMsg, grant *sobject.SOInviteMessage, image *bucket.ObjectRef, now time.Time) error {
	// Verify the authorization against the signers the card will pin.
	authorized, _, err := VerifyImage(signed, signers)
	if err != nil {
		return err
	}
	if !authorized.EqualVT(image) {
		return errors.Errorf("image authorization: authorizes root %s, not the image's root %s",
			authorized.GetRootRef().MarshalString(), image.GetRootRef().MarshalString())
	}

	// Refuse a grant no Device key can redeem: forged, bound to one peer, or expired.
	if _, err := grant.VerifyTransportPeer(); err != nil {
		return errors.Wrap(err, "bootstrap grant")
	}
	if grant.GetTargetPeerId() != "" {
		return errors.Errorf("bootstrap grant: bound to peer %s, but each Device makes its own key", grant.GetTargetPeerId())
	}
	if expires := grant.GetExpiresAt(); expires != nil && !expires.AsTime().After(now) {
		return errors.Errorf("bootstrap grant: expired at %s", expires.AsTime().Format(time.RFC3339))
	}

	// Write the signers and the grant.
	grantData, err := grant.MarshalVT()
	if err != nil {
		return err
	}
	if err := WriteSigners(filepath.Join(dir, CardSigners), signers); err != nil {
		return err
	}
	return writePacked(filepath.Join(dir, CardGrant), grantData)
}

// ReadAuthorization reads the packedmsg image authorization at path.
func ReadAuthorization(path string) (*peer.SignedMsg, error) {
	// Decode the packed text. The caller verifies the signed message.
	data, err := readPacked(path)
	if err != nil {
		return nil, errors.Wrap(err, "image authorization")
	}
	signed, err := peer.UnmarshalSignedMsg(data)
	if err != nil {
		return nil, errors.Wrap(err, "image authorization")
	}
	return signed, nil
}

// ReadGrant reads the packedmsg bootstrap grant at path. The caller verifies
// it before presenting it.
func ReadGrant(path string) (*sobject.SOInviteMessage, error) {
	// Decode the packed text into the invite message.
	data, err := readPacked(path)
	if err != nil {
		return nil, errors.Wrap(err, "bootstrap grant")
	}
	grant := &sobject.SOInviteMessage{}
	if err := grant.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "bootstrap grant")
	}
	return grant, nil
}

// readPacked reads the packedmsg text at path and returns its message bytes.
func readPacked(path string) ([]byte, error) {
	// Decode the packed text, which carries its own checksum.
	text, err := os.ReadFile(path) //nolint:gosec // The boot and install read their own card.
	if err != nil {
		return nil, err
	}
	data, ok := packedmsg.DecodePackedMessage(strings.TrimSpace(string(text)))
	if !ok {
		return nil, errors.New("not a packed message")
	}
	return data, nil
}

// writePacked writes data to path as one line of packedmsg text.
func writePacked(path string, data []byte) error {
	return writeFileSync(path, []byte(packedmsg.EncodePackedMessage(data)+"\n"))
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
