package bldr_dist

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// PluginHostObjectKey scopes cached plugin selections to the distribution's
// embedded manifest World, platform and channel. Artifact revision counters are
// local to a producer and cannot establish compatibility across installations.
func PluginHostObjectKey(meta *DistMeta) (string, error) {
	// The embedded metadata is immutable and contains the complete distribution identity.
	data, err := meta.MarshalVT()
	if err != nil {
		return "", err
	}
	identity, err := hash.Sum(hash.RecommendedHashType, data)
	if err != nil {
		return "", err
	}
	return "plugin-host/" + identity.MarshalString(), nil
}

// ParsePluginHostObjectKey reads the distribution identity from a plugin cache key.
func ParsePluginHostObjectKey(key string) (*hash.Hash, error) {
	// Cut the plugin-host prefix and require it to be present.
	id, ok := strings.CutPrefix(key, "plugin-host/")
	if !ok {
		return nil, errors.New("invalid distribution plugin-host object key")
	}

	// Parse and validate the distribution identity from the remaining text.
	identity := &hash.Hash{}
	if err := identity.ParseFromB58(id); err != nil {
		return nil, err
	}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	return identity, nil
}
