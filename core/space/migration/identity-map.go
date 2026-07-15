package space_migration

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"
)

// IdentityMap contains deterministic mappings for all Space-local identities.
type IdentityMap struct {
	SpaceIDs            map[string]string
	BlockStoreIDs       map[string]string
	ObjectKeys          map[string]string
	NestedSharedObjects map[string]string
	CanvasNodes         map[string]string
	GraphIRIs           map[string]string
}

// NewIdentityMap constructs an empty identity map.
func NewIdentityMap() *IdentityMap {
	return &IdentityMap{
		SpaceIDs:            make(map[string]string),
		BlockStoreIDs:       make(map[string]string),
		ObjectKeys:          make(map[string]string),
		NestedSharedObjects: make(map[string]string),
		CanvasNodes:         make(map[string]string),
		GraphIRIs:           make(map[string]string),
	}
}

// DeterministicIdentity derives a stable destination identity for a source value.
func DeterministicIdentity(namespace, kind, source string) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{namespace, kind, source}, "\x00")))
	return hex.EncodeToString(hash[:])
}

// MapGraphIRI maps an exact Space-local graph IRI and leaves external values unchanged.
func (m *IdentityMap) MapGraphIRI(value string) string {
	if m == nil {
		return value
	}
	if mapped := m.GraphIRIs[value]; mapped != "" {
		return mapped
	}
	return value
}

// Validate checks that all identity maps are initialized and collision-free.
func (m *IdentityMap) Validate() error {
	if m == nil {
		return errors.New("identity map is required")
	}
	for name, values := range map[string]map[string]string{
		"space": m.SpaceIDs, "block-store": m.BlockStoreIDs, "object": m.ObjectKeys,
		"nested-shared-object": m.NestedSharedObjects, "canvas-node": m.CanvasNodes,
		"graph-iri": m.GraphIRIs,
	} {
		seen := make(map[string]string, len(values))
		for source, destination := range values {
			if source == "" || destination == "" {
				return errors.Errorf("%s identity mapping has an empty value", name)
			}
			if previous, exists := seen[destination]; exists && previous != source {
				return errors.Errorf("%s identity mapping collides at %s", name, destination)
			}
			seen[destination] = source
		}
	}
	return nil
}
