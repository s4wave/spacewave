package resource_cdn

import (
	"testing"

	"github.com/s4wave/spacewave/core/cdn"
	"github.com/sirupsen/logrus"
)

func TestRegistryAliasesShareInstance(t *testing.T) {
	// Cancel transport work while exercising real lazy instance construction.
	registry := NewRegistry(logrus.NewEntry(logrus.New()), nil)
	registry.ctxCancel()
	defer registry.Close()

	// Both public names must return one SharedObject and block-store cache.
	defaultInstance, err := registry.Lookup("")
	if err != nil {
		t.Fatal(err)
	}
	explicitInstance, err := registry.Lookup(cdn.SpaceID())
	if err != nil {
		t.Fatal(err)
	}
	if defaultInstance != explicitInstance {
		t.Fatal("default alias and explicit CDN ID created separate instances")
	}
}
