package provider_local

import (
	"crypto/ecdh"
	"crypto/sha256"
	"sort"

	"github.com/pkg/errors"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/extra25519"
)

// sasEmojiTable is the 64-emoji lookup table for SAS verification.
var sasEmojiTable = [64]string{
	// Animals
	"\U0001F436", // dog
	"\U0001F431", // cat
	"\U0001F42D", // mouse
	"\U0001F439", // hamster
	"\U0001F430", // rabbit
	"\U0001F43B", // bear
	"\U0001F43C", // panda
	"\U0001F428", // koala
	"\U0001F42F", // tiger
	"\U0001F981", // lion
	"\U0001F42E", // cow
	"\U0001F437", // pig
	"\U0001F438", // frog
	"\U0001F435", // monkey
	"\U0001F414", // chicken
	"\U0001F427", // penguin
	// Nature
	"\U0001F333", // tree
	"\U0001F335", // cactus
	"\U0001F337", // tulip
	"\U0001F339", // rose
	"\U0001F33B", // sunflower
	"\U0001F340", // four leaf clover
	"\U0001F341", // maple leaf
	"\U0001F344", // mushroom
	// Food
	"\U0001F34E", // apple
	"\U0001F34A", // orange
	"\U0001F34B", // lemon
	"\U0001F34D", // pineapple
	"\U0001F347", // grapes
	"\U0001F349", // watermelon
	"\U0001F353", // strawberry
	"\U0001F352", // cherries
	// Objects
	"\U0001F3B5", // music note
	"\U0001F3B8", // guitar
	"\U0001F3BA", // trumpet
	"\U0001F3B2", // dice
	"\U0001F3AF", // target
	"\U0001F3C6", // trophy
	"\U0001F451", // crown
	"\U0001F48E", // gem
	// Weather/Space
	"\u2B50",     // star
	"\U0001F319", // crescent moon
	"\u2600",     // sun
	"\u26A1",     // lightning
	"\U0001F308", // rainbow
	"\u2744",     // snowflake
	"\U0001F525", // fire
	"\U0001F4A7", // droplet
	// Symbols
	"\u2764",     // heart
	"\U0001F49C", // purple heart
	"\U0001F499", // blue heart
	"\U0001F49A", // green heart
	"\U0001F4AB", // dizzy (star)
	"\U0001F44D", // thumbs up
	"\U0001F44E", // thumbs down
	"\U0001F44B", // wave
	// Vehicles
	"\U0001F680", // rocket
	"\U0001F6F8", // flying saucer
	"\U0001F3A0", // carousel
	"\u26F5",     // sailboat
	"\U0001F697", // car
	"\U0001F682", // locomotive
	"\U0001F6B2", // bicycle
	"\u2708",     // airplane
}

// DeriveSASEmoji derives a 6-emoji SAS verification sequence from
// the local session's private key and the remote peer's public key.
// Both sides compute the same sequence if keys are authentic.
func DeriveSASEmoji(
	localPriv bifrost_crypto.PrivKey,
	remotePub bifrost_crypto.PubKey,
	localPeerID peer.ID,
	remotePeerID peer.ID,
) ([]string, error) {
	// Convert local Ed25519 private key to X25519.
	edPriv, ok := localPriv.(*bifrost_crypto.Ed25519PrivateKey)
	if !ok {
		return nil, errors.New("local key must be Ed25519")
	}
	edPrivStd := edPriv.GetStdKey()
	curvePriv := extra25519.PrivateKeyToCurve25519(edPrivStd)
	ecdhPriv, err := ecdh.X25519().NewPrivateKey(curvePriv[:32])
	if err != nil {
		return nil, errors.Wrap(err, "create X25519 private key")
	}

	// Convert remote Ed25519 public key to X25519.
	edPub, ok := remotePub.(*bifrost_crypto.Ed25519PublicKey)
	if !ok {
		return nil, errors.New("remote key must be Ed25519")
	}
	edPubStd := edPub.GetStdKey()
	curvePub, valid := extra25519.PublicKeyToCurve25519(edPubStd)
	if !valid {
		return nil, errors.New("remote public key cannot be converted to X25519")
	}
	ecdhPub, err := ecdh.X25519().NewPublicKey(curvePub)
	if err != nil {
		return nil, errors.Wrap(err, "create X25519 public key")
	}

	// ECDH shared secret.
	shared, err := ecdhPriv.ECDH(ecdhPub)
	if err != nil {
		return nil, errors.Wrap(err, "ECDH key agreement")
	}

	// Sort peer IDs lexicographically by raw bytes.
	peerIDs := []peer.ID{localPeerID, remotePeerID}
	sort.Slice(peerIDs, func(i, j int) bool {
		return string(peerIDs[i]) < string(peerIDs[j])
	})

	// Hash: SHA-256(sharedSecret || sorted[0] || sorted[1]).
	h := sha256.New()
	h.Write(shared)
	h.Write([]byte(peerIDs[0]))
	h.Write([]byte(peerIDs[1]))
	digest := h.Sum(nil)

	// Map first 6 bytes to emoji.
	emoji := make([]string, 6)
	for i := range 6 {
		emoji[i] = sasEmojiTable[digest[i]%64]
	}

	return emoji, nil
}
