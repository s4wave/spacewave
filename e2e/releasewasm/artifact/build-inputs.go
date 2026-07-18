package artifact

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"slices"

	"github.com/pkg/errors"
)

// BuildInputs identifies the compiler, mode, tools, and environment that
// determine release artifact bytes.
type BuildInputs struct {
	Compiler    string
	Mode        string
	Environment map[string]string
	Tools       map[string]string
}

func (i *BuildInputs) digest() (string, error) {
	if i == nil {
		return "", errors.New("nil release artifact build inputs")
	}
	if i.Compiler == "" {
		return "", errors.New("empty release artifact compiler")
	}
	if i.Mode == "" {
		return "", errors.New("empty release artifact build mode")
	}

	h := sha256.New()
	writeDigestField(h, "compiler", i.Compiler)
	writeDigestField(h, "mode", i.Mode)
	writeDigestMap(h, "environment", i.Environment)
	writeDigestMap(h, "tools", i.Tools)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeDigestMap(h hash.Hash, group string, values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		writeDigestField(h, group+"/"+key, values[key])
	}
}

func writeDigestField(h hash.Hash, key, value string) {
	h.Write([]byte(key))
	h.Write([]byte{0})
	h.Write([]byte(value))
	h.Write([]byte{0})
}
