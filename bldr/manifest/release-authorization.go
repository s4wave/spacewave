package bldr_manifest

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// ReleaseAuthorizationContext separates executable authorization from other peer signatures.
const ReleaseAuthorizationContext = "bldr/manifest 2026-10-10T16:53:45Z release authorization"

// SignReleaseAuthorization authorizes this exact Manifest root with the release key.
// Bucket locations and transforms may change without changing executable identity.
func (m *ManifestRef) SignReleaseAuthorization(key crypto.PrivKey) error {
	// Require a complete content-addressed root before signing.
	root := m.GetManifestRef().GetRootRef()
	if root.GetEmpty() {
		return errors.New("release authorization: missing manifest root")
	}
	if err := root.Validate(false); err != nil {
		return errors.Wrap(err, "release authorization: manifest root")
	}

	// Sign the root in the executable-authorization context.
	data, err := (&ReleaseAuthorization{ManifestRoot: root}).MarshalVT()
	if err != nil {
		return err
	}
	signed, err := peer.NewSignedMsg(ReleaseAuthorizationContext, key, hash.HashType_HashType_BLAKE3, data)
	if err != nil {
		return err
	}
	m.ReleaseAuthorization = signed
	return nil
}

// VerifyReleaseAuthorization requires a valid release-peer signature on this exact root.
// A valid signature from a World validator grants no executable authority.
func (m *ManifestRef) VerifyReleaseAuthorization(allowedPeers []peer.ID) error {
	// Require both the authorization and the caller's release pins.
	signed := m.GetReleaseAuthorization()
	if signed == nil {
		return errors.New("release authorization: missing authorization")
	}
	if len(allowedPeers) == 0 {
		return errors.New("release authorization: missing release peer pins")
	}

	// Authenticate the signature before trusting its signer or payload.
	_, signer, err := signed.ExtractAndVerify(ReleaseAuthorizationContext)
	if err != nil {
		return errors.Wrap(err, "release authorization: invalid signature")
	}
	if !slices.Contains(allowedPeers, signer) {
		return errors.Errorf("release authorization: signer %s is not a pinned release peer", signer.String())
	}

	// Match the authorized content identity to the selected executable root.
	authorization := &ReleaseAuthorization{}
	if err := authorization.UnmarshalVT(signed.GetData()); err != nil {
		return errors.Wrap(err, "release authorization: invalid payload")
	}
	root := authorization.GetManifestRoot()
	if root.GetEmpty() {
		return errors.New("release authorization: missing authorized manifest root")
	}
	if err := root.Validate(false); err != nil {
		return errors.Wrap(err, "release authorization: authorized manifest root")
	}
	if !root.EqualsRef(m.GetManifestRef().GetRootRef()) {
		return errors.New("release authorization: manifest root does not match authorized root")
	}
	return nil
}
