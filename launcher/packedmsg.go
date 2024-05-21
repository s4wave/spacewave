package bldr_launcher

import (
	"strings"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bldr/util/packedmsg"
	"github.com/aperturerobotics/util/scrub"
	"github.com/klauspost/compress/s2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/chacha20poly1305"
)

var ErrUnknownDistSigPeer = errors.New("message not signed with any recognized distribution keys")

// DecodeSignedDistConfig attempts to decode a packed DistConfig.
// The DistConfig is packed inside a SignedMsg.
// Pass a list of acceptable signature peer IDs to accept.
//
// Data is the unpacked SignedMsg.
// Returns ErrUnknownDistSigPeer if none of the peer IDs matched the message public key.
func DecodeSignedDistConfig(data []byte, allowedPeerIDs []peer.ID, projectID string) (*DistConfig, peer.ID, error) {
	signedMsg := &peer.SignedMsg{}
	if err := signedMsg.UnmarshalVT(data); err != nil {
		return nil, "", err
	}

	signerPub, _, err := signedMsg.ExtractAndVerify()
	if err != nil {
		return nil, "", err
	}
	var matchedPeerID peer.ID
	for _, peerID := range allowedPeerIDs {
		if peerID.MatchesPublicKey(signerPub) {
			matchedPeerID = peerID
			break
		}
	}
	if len(matchedPeerID) == 0 {
		return nil, "", ErrUnknownDistSigPeer
	}

	// Make it a bit harder for a would-be curious onlooker.
	// Decrypt the signed message using a deterministic key.
	cmpDistConfData, err := DecryptDistConfig(signedMsg.GetData(), signedMsg.GetFromPeerId(), signedMsg.GetSignature().GetHashType(), projectID)
	if err != nil {
		return nil, "", errors.Wrap(err, "valid signature but invalid crypt")
	}

	// Decompress
	distConfData, err := s2.Decode(nil, cmpDistConfData)
	scrub.Scrub(cmpDistConfData)
	if err != nil {
		return nil, "", errors.Wrap(err, "valid signature but invalid cmp")
	}
	defer scrub.Scrub(distConfData)

	appDistConf := &DistConfig{}
	if err := appDistConf.UnmarshalVT(distConfData); err != nil {
		return nil, "", errors.Wrap(err, "valid signature and crypt but invalid body")
	}
	if err := appDistConf.Validate(); err != nil {
		return nil, "", errors.Wrap(err, "valid signature and crypt but invalid config")
	}

	return appDistConf, matchedPeerID, nil
}

// EncodeSignedDistConfig attempts to encode a packed DistConfig.
// The DistConfig is packed inside a SignedMsg.
//
// Data is the DistConfig data.
func EncodeSignedDistConfig(peerPriv crypto.PrivKey, distConf *DistConfig) ([]byte, error) {
	data, err := distConf.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Compress
	cmp := s2.EncodeBest(nil, data)
	scrub.Scrub(data)
	data = cmp

	// Make it a bit harder for a would-be curious onlooker.
	// Encrypt the signed message using a deterministic key.
	peerID, err := peer.IDFromPrivateKey(peerPriv)
	if err != nil {
		return nil, err
	}
	peerIDString := peerID.String()
	signatureHashType := hash.HashType_HashType_SHA256
	distConfData, err := EncryptDistConfig(data, peerIDString, signatureHashType, distConf.GetProjectId())
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(distConfData)

	signedMsg := &peer.SignedMsg{
		FromPeerId: peerIDString,
		Data:       distConfData,
	}
	if err := signedMsg.Sign(peerPriv, signatureHashType); err != nil {
		return nil, err
	}

	// Marshal the signed message.
	signedMsgData, err := signedMsg.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Verify that it decodes correctly.
	outConf, outMatchedPeerID, err := DecodeSignedDistConfig(signedMsgData, []peer.ID{peerID}, distConf.GetProjectId())
	if err != nil {
		return nil, err
	}
	defer outConf.Reset()
	if !outConf.EqualVT(distConf) {
		return nil, errors.New("encoded dist config failed: parsed object mismatch")
	}
	if outMatchedPeerID.String() != peerID.String() {
		return nil, errors.New("encoded dist config failed: parsed peer id mismatch")
	}

	// Verified & done
	return signedMsgData, nil
}

// deriveDistConfigKey derives the 32-byte encrypt key and nonce for DistConfig.
func deriveDistConfigKey(senderPeerID string, signatureHashType hash.HashType, projectID string) (encKey []byte, nonce []byte, err error) {
	var out [chacha20poly1305.KeySize]byte
	material := []byte(strings.Join([]string{senderPeerID, signatureHashType.String(), projectID}, "---COMBUSTIBLE LEMON---"))
	blake3.DeriveKey(
		"bldr/app/dist-config 2024-05-21T07:07:33.279912Z",
		material,
		out[:],
	)
	var nonceOut [chacha20poly1305.NonceSizeX]byte
	blake3.DeriveKey(
		"bldr/app/dist-config 2024-05-21T07:07:51.952028Z",
		material,
		nonceOut[:],
	)
	scrub.Scrub(material[:])
	return out[:], nonceOut[:], nil
}

// DecryptDistConfig decrypts an encrypted DistConfig.
func DecryptDistConfig(data []byte, senderPeerID string, signatureHashType hash.HashType, projectID string) ([]byte, error) {
	key, nonce, err := deriveDistConfigKey(senderPeerID, signatureHashType, projectID)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(key)
	defer scrub.Scrub(nonce)

	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	return cipher.Open(nil, nonce, data, []byte(senderPeerID+"/"+projectID))
}

// EncryptDistConfig encrypts an DistConfig body for SignedMsg.
func EncryptDistConfig(data []byte, senderPeerID string, signatureHashType hash.HashType, projectID string) ([]byte, error) {
	key, nonce, err := deriveDistConfigKey(senderPeerID, signatureHashType, projectID)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(key)
	defer scrub.Scrub(nonce)

	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	return cipher.Seal(nil, nonce, data, []byte(senderPeerID+"/"+projectID)), nil
}

// FindDistConfigUpdate attempts to find a valid signed packedmsg with a rev greater than the given.
//
// Skips any configurations with a different project id.
// Returns nil, nil, nil if none found with rev higher than given.
// le can be nil to disable logging
func FindDistConfigUpdate(le *logrus.Entry, currRev uint64, data []byte, distPeerIDs []peer.ID, projectID string) (*DistConfig, string, peer.ID, error) {
	// replace the breaks we add in the dist server
	dataStr := strings.TrimSpace(string(data))
	dataStr = strings.ReplaceAll(dataStr, "<br/>", "\n")

	// find the updated config in the body
	var updatedAppDistConf *DistConfig
	var updatedAppDistConfMsg string
	var updatedAppDistConfPeer peer.ID
	packedMsgs, packedMsgsSrc := packedmsg.FindPackedMessages(dataStr)
	if len(packedMsgs) == 0 {
		return nil, "", "", nil
	}
	for _, msgp := range packedMsgs {
		msg := msgp
		defer scrub.Scrub(msg)
	}
	for i, msg := range packedMsgs {
		distConf, matchedPeerID, err := DecodeSignedDistConfig(msg, distPeerIDs, projectID)
		if err != nil {
			le.WithError(err).Warn("skipping invalid dist config packedmsg")
			continue
		}

		le := le
		if le != nil {
			if currRev != 0 {
				le = le.WithField("curr-rev", currRev)
			}
			le = le.
				WithField("rev", distConf.GetRev()).
				WithField("signer", matchedPeerID.String())
		}
		if distConf.GetProjectId() != projectID {
			if le != nil {
				le.Debugf("found valid app dist config but for different project: %s != expected %s", distConf.GetProjectId(), projectID)
			}
			continue
		}
		if updatedAppDistConf.GetRev() < distConf.GetRev() && currRev < distConf.GetRev() {
			updatedAppDistConf = distConf
			updatedAppDistConfMsg = packedMsgsSrc[i]
			updatedAppDistConfPeer = matchedPeerID
			if le != nil {
				le.Info("found valid app dist config")
			}
		} else {
			if le != nil {
				le.Debug("found valid but older app dist config")
			}
		}
	}

	return updatedAppDistConf, updatedAppDistConfMsg, updatedAppDistConfPeer, nil
}

// ParseDistConfigPackedMsg parses a packedmsg with an app dist config.
// Returns an error if no valid messages were found.
// Skips any messages for a different project id.
// Returns the config, the encoded config, the peer that signed the config, and any error.
// le can be nil to disable logging
func ParseDistConfigPackedMsg(le *logrus.Entry, data []byte, distPeerIDs []peer.ID, projectID string) (*DistConfig, string, peer.ID, error) {
	conf, confMsg, confPeer, err := FindDistConfigUpdate(le, 0, data, distPeerIDs, projectID)
	if err != nil {
		return nil, "", "", err
	}
	if conf == nil {
		return nil, "", "", errors.New("no valid app dist config found")
	}
	return conf, confMsg, confPeer, err
}
