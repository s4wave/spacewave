package sobject

import (
	"bytes"
	"sync"

	"github.com/s4wave/spacewave/net/crypto"
)

// maxKeyringGrants bounds the grants a keyring holds. A shared object holds
// one local grant per key epoch, so the bound is reached only by a long epoch
// history; the keyring then starts over.
const maxKeyringGrants = 64

// SOGrantKeyring decrypts the local peer's grants for the states of one shared
// object and remembers each decrypted grant. Decrypting a grant converts the
// peer's key and runs a scalar multiplication, so the snapshots of one shared
// object share a keyring instead of decrypting the same grant for each new
// state. A grant is immutable while held: state copies share it. A separately
// decoded grant is reused only when its bytes match. The keyring holds the
// transform keys of its grants for its lifetime. It also remembers the last
// checkpoint body it decoded, since the checkpoint changes only at a
// checkpoint while the state changes with every operation. It is safe for
// concurrent use.
type SOGrantKeyring struct {
	// sharedObjectID is the shared object the grants are bound to.
	sharedObjectID string
	// mtx guards inners and checkpoint.
	mtx sync.Mutex
	// inners holds each decrypted grant by its encrypted inner data.
	inners map[string]keyringGrant
	// checkpoint is the last checkpoint body decoded, or empty.
	checkpoint keyringCheckpoint
}

// keyringCheckpoint is a checkpoint body with its state data decoded.
type keyringCheckpoint struct {
	// data is the encoded body of the checkpoint, which states share and
	// never modify.
	data []byte
	// inner is the body of data with its state data decoded, which callers
	// never modify.
	inner *SOCheckpointInner
}

// keyringGrant is a grant with the body its decryption returned.
type keyringGrant struct {
	// grant is the grant that was decrypted.
	grant *SOGrant
	// inner is the decrypted body of grant, which callers never modify.
	inner *SOGrantInner
}

// NewSOGrantKeyring returns a keyring for the grants of sharedObjectID.
func NewSOGrantKeyring(sharedObjectID string) *SOGrantKeyring {
	return &SOGrantKeyring{sharedObjectID: sharedObjectID}
}

// Inner returns the decrypted body of grant, which is addressed to privKey.
// The caller must not modify the result.
func (k *SOGrantKeyring) Inner(grant *SOGrant, privKey crypto.PrivKey) (*SOGrantInner, error) {
	// Reuse the body of a grant with the same bytes.
	key := string(grant.GetInnerData())
	k.mtx.Lock()
	defer k.mtx.Unlock()
	if held, ok := k.inners[key]; ok && held.grant.EqualVT(grant) {
		return held.inner, nil
	}

	// Decrypt the grant and remember its body.
	inner, err := grant.DecryptInnerData(privKey, k.sharedObjectID)
	if err != nil {
		return nil, err
	}
	if k.inners == nil || len(k.inners) >= maxKeyringGrants {
		k.inners = make(map[string]keyringGrant)
	}
	k.inners[key] = keyringGrant{grant: grant, inner: inner}
	return inner, nil
}

// Checkpoint returns the body of checkpoint with its state data decoded by
// decode, reusing the body of the last checkpoint with the same encoded body.
// decode may call Inner. The caller must not modify the result.
//
// Cost: on a reuse, one comparison of the encoded body; otherwise one decode
// of the body and its state data.
func (k *SOGrantKeyring) Checkpoint(checkpoint *SOCheckpoint, decode func(*SOCheckpointInner) (*SOCheckpointInner, error)) (*SOCheckpointInner, error) {
	// Reuse the body of a checkpoint with the same bytes.
	data := checkpoint.GetInner()
	k.mtx.Lock()
	held := k.checkpoint
	k.mtx.Unlock()
	if held.inner != nil && bytes.Equal(held.data, data) {
		return held.inner, nil
	}

	// Decode it outside the lock, which decode takes through Inner, and
	// remember it.
	inner, err := checkpoint.UnmarshalInner()
	if err != nil {
		return nil, err
	}
	inner, err = decode(inner)
	if err != nil {
		return nil, err
	}
	k.mtx.Lock()
	k.checkpoint = keyringCheckpoint{data: data, inner: inner}
	k.mtx.Unlock()
	return inner, nil
}
