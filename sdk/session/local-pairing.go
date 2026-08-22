package s4wave_session

import (
	"bytes"
	"compress/flate"
	"io"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// localPairingMessage is the generated-message contract shared by pairing
// offers and answers.
type localPairingMessage[T any] interface {
	*T
	MarshalVT() ([]byte, error)
	UnmarshalVT([]byte) error
}

// encodeLocalPairing serializes, DEFLATE-compresses, and base58-encodes a
// pairing message.
func encodeLocalPairing[M localPairingMessage[T], T any](label string, msg M) (string, error) {
	data, err := msg.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal "+label)
	}
	compressed, err := compressLocalPairingPayload(data)
	if err != nil {
		return "", errors.Wrap(err, "compress "+label)
	}
	return b58.Encode(compressed), nil
}

// decodeLocalPairing decodes a base58 string into a pairing message.
func decodeLocalPairing[M localPairingMessage[T], T any](label, encoded string) (T, error) {
	var msg T
	compressed, err := b58.Decode(encoded)
	if err != nil {
		return msg, errors.Wrap(err, "base58 decode")
	}
	data, err := decompressLocalPairingPayload(compressed)
	if err != nil {
		return msg, errors.Wrap(err, "decompress "+label)
	}
	if err := M(&msg).UnmarshalVT(data); err != nil {
		return msg, errors.Wrap(err, "unmarshal "+label)
	}
	return msg, nil
}

// EncodeLocalPairingOffer serializes, DEFLATE-compresses, and base58-encodes
// an offer.
func EncodeLocalPairingOffer(offer *LocalPairingOffer) (string, error) {
	return encodeLocalPairing[*LocalPairingOffer]("offer", offer)
}

// DecodeLocalPairingOffer decodes a base58 string into a LocalPairingOffer.
func DecodeLocalPairingOffer(encoded string) (*LocalPairingOffer, error) {
	msg, err := decodeLocalPairing[*LocalPairingOffer]("offer", encoded)
	return &msg, err
}

// EncodeLocalPairingAnswer serializes, DEFLATE-compresses, and base58-encodes
// an answer.
func EncodeLocalPairingAnswer(answer *LocalPairingAnswer) (string, error) {
	return encodeLocalPairing[*LocalPairingAnswer]("answer", answer)
}

// DecodeLocalPairingAnswer decodes a base58 string into a LocalPairingAnswer.
func DecodeLocalPairingAnswer(encoded string) (*LocalPairingAnswer, error) {
	msg, err := decodeLocalPairing[*LocalPairingAnswer]("answer", encoded)
	return &msg, err
}

// ParsePeerID parses the peer ID from a LocalPairingOffer.
func (o *LocalPairingOffer) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(o.GetPeerId())
}

// ParsePeerID parses the peer ID from a LocalPairingAnswer.
func (a *LocalPairingAnswer) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(a.GetPeerId())
}

// compressLocalPairingPayload compresses a payload with DEFLATE.
func compressLocalPairingPayload(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return nil, errors.Wrap(err, "construct flate writer")
	}
	if _, err := w.Write(data); err != nil {
		return nil, errors.Wrap(err, "write flate data")
	}
	if err := w.Close(); err != nil {
		return nil, errors.Wrap(err, "close flate writer")
	}
	return buf.Bytes(), nil
}

// decompressLocalPairingPayload decompresses a DEFLATE payload.
func decompressLocalPairingPayload(data []byte) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	decoded, err := io.ReadAll(r)
	closeErr := r.Close()
	if err != nil {
		if closeErr != nil {
			return nil, errors.Wrapf(err, "read flate data (close error: %v)", closeErr)
		}
		return nil, errors.Wrap(err, "read flate data")
	}
	if closeErr != nil {
		return nil, errors.Wrap(closeErr, "close flate reader")
	}
	return decoded, nil
}
