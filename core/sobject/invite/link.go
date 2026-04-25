package sobject_invite

import (
	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// SerializeInviteLink serializes an SOInviteMessage as a base58 string
// suitable for embedding in a URL path or query parameter.
func SerializeInviteLink(msg *sobject.SOInviteMessage) (string, error) {
	data, err := msg.MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal invite message")
	}
	return b58.Encode(data), nil
}

// DeserializeInviteLink deserializes a base58-encoded SOInviteMessage
// from a URL path or query parameter.
func DeserializeInviteLink(encoded string) (*sobject.SOInviteMessage, error) {
	data, err := b58.Decode(encoded)
	if err != nil {
		return nil, errors.Wrap(err, "decode base58 invite link")
	}
	msg := &sobject.SOInviteMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal invite message")
	}
	return msg, nil
}
