package spacewave_chat_content

import (
	"strings"
	"unicode/utf8"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

// Validate checks the protocol type and bounded JSON body before storage.
func (e *ChatEvent) Validate() error {
	// Require a bounded UTF-8 protocol type for the chat event.
	if e.GetType() == "" || len(e.GetType()) > 255 || strings.ContainsRune(e.GetType(), 0) || !utf8.ValidString(e.GetType()) {
		return errors.New("chat event requires a bounded protocol type")
	}

	// Validate the chat event body as a bounded JSON object.
	if len(e.GetContentJson()) > 64*1024 || !utf8.ValidString(e.GetContentJson()) {
		return errors.New("chat event content exceeds its bounds or is not UTF-8")
	}
	value, err := fastjson.Parse(e.GetContentJson())
	if err != nil {
		return errors.Wrap(err, "parse chat event content")
	}
	if value.Type() != fastjson.TypeObject {
		return errors.New("chat event content must be a JSON object")
	}
	return e.GetRelation().Validate()
}

// Validate checks one optional relationship expressed in native message keys.
func (r *ChatRelation) Validate() error {
	if r == nil {
		return nil
	}
	if len(r.GetType()) > 255 || len(r.GetKey()) > 4096 || len(r.GetTargetKey()) > 4096 || len(r.GetReplyToKey()) > 4096 {
		return errors.New("chat relationship is incomplete or exceeds its bounds")
	}
	if (r.GetType() == "") != (r.GetTargetKey() == "") || r.GetTargetKey() == "" && r.GetReplyToKey() == "" {
		return errors.New("chat relationship is incomplete or exceeds its bounds")
	}
	return nil
}
