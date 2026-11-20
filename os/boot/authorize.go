package boot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
	os_image "github.com/s4wave/spacewave/os/image"
)

// pinnedSigners names the data Volume's copy of the accepted signers. Once
// it exists, the card's signers file is never read again.
const pinnedSigners = "pinned-signers"

// AuthorizedRoot returns the image root the card's authorization names, once
// a pinned signer's signature over it verifies. First boot pins the card's
// signers into the data directory; every later boot verifies against that
// pin, so rewriting the card's signers after first boot authorizes nothing.
func AuthorizedRoot(card, data string) (*block.BlockRef, error) {
	// Pin the signers before trusting any authorization.
	pinned, err := PinSigners(card, data)
	if err != nil {
		return nil, err
	}

	// Verify the card's authorization against the pin alone.
	signed, err := os_image.ReadAuthorization(filepath.Join(card, os_image.CardAuthorization))
	if err != nil {
		return nil, err
	}
	root, signer, err := os_image.VerifyImageRoot(signed, pinned)
	if err != nil {
		return nil, err
	}
	println("SPACEWAVE_OS_AUTHORIZED signer=" + signer.String() + " root=" + root.MarshalString())
	return root, nil
}

// PinSigners returns the signers pinned in the data directory, first copying
// them from the card when the data directory holds none. The copy is synced
// and renamed into place, so a power cut leaves no pin or the whole pin.
func PinSigners(card, data string) ([]peer.ID, error) {
	// Return an existing pin; the card's signers no longer matter.
	path := filepath.Join(data, pinnedSigners)
	pinned, err := os_image.ReadSigners(path)
	if err == nil {
		return pinned, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	// First boot: pin the card's signers.
	signers, err := os_image.ReadSigners(filepath.Join(card, os_image.CardSigners))
	if err != nil {
		return nil, err
	}
	if err := os_image.WriteSigners(path, signers); err != nil {
		return nil, err
	}
	println("SPACEWAVE_OS_PINNED signers=" + strings.Join(peer.IDsToString(signers), ","))
	return signers, nil
}
