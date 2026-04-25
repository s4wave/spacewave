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

// EncodeLocalPairingOffer serializes, DEFLATE-compresses, and base58-encodes
// an offer.
func EncodeLocalPairingOffer(offer *LocalPairingOffer) (string, error) {
	data, err := offer.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal offer")
	}
	compressed, err := compressLocalPairingPayload(data)
	if err != nil {
		return "", errors.Wrap(err, "compress offer")
	}
	return b58.Encode(compressed), nil
}

// DecodeLocalPairingOffer decodes a base58 string into a LocalPairingOffer.
func DecodeLocalPairingOffer(encoded string) (*LocalPairingOffer, error) {
	compressed, err := b58.Decode(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "base58 decode")
	}
	data, err := decompressLocalPairingPayload(compressed)
	if err != nil {
		return nil, errors.Wrap(err, "decompress offer")
	}
	offer := &LocalPairingOffer{}
	if err := offer.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal offer")
	}
	return offer, nil
}

// EncodeLocalPairingAnswer serializes, DEFLATE-compresses, and base58-encodes
// an answer.
func EncodeLocalPairingAnswer(answer *LocalPairingAnswer) (string, error) {
	data, err := answer.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal answer")
	}
	compressed, err := compressLocalPairingPayload(data)
	if err != nil {
		return "", errors.Wrap(err, "compress answer")
	}
	return b58.Encode(compressed), nil
}

// ParsePeerID parses the peer ID from a LocalPairingOffer.
func (o *LocalPairingOffer) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(o.GetPeerId())
}

// ParsePeerID parses the peer ID from a LocalPairingAnswer.
func (a *LocalPairingAnswer) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(a.GetPeerId())
}

// DecodeLocalPairingAnswer decodes a base58 string into a LocalPairingAnswer.
func DecodeLocalPairingAnswer(encoded string) (*LocalPairingAnswer, error) {
	compressed, err := b58.Decode(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "base58 decode")
	}
	data, err := decompressLocalPairingPayload(compressed)
	if err != nil {
		return nil, errors.Wrap(err, "decompress answer")
	}
	answer := &LocalPairingAnswer{}
	if err := answer.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal answer")
	}
	return answer, nil
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
