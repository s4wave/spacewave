package opfs

import (
	"testing"

	"github.com/s4wave/spacewave/db/coord"
)

func TestWriteLockNameDerivesDistinctNamesFromOnePrefix(t *testing.T) {
	prefix := "spacewave/volume-a"
	storeScope := coord.Scope{VolumeID: "volume-a", ObjectStoreID: "objects"}
	keyedScope := coord.Scope{VolumeID: "volume-a", Key: "world-1"}
	otherKeyedScope := coord.Scope{VolumeID: "volume-a", Key: "world-2"}

	storeName := writeLockName(prefix, storeScope)
	if storeName == prefix+"/coord/write" {
		t.Fatalf("object store lock name does not include its scope: %q", storeName)
	}

	keyedName := writeLockName(prefix, keyedScope)
	if keyedName == storeName {
		t.Fatalf("keyed lock name %q collides with object store lock name", keyedName)
	}
	if otherName := writeLockName(prefix, otherKeyedScope); otherName == keyedName {
		t.Fatalf("distinct keys derive one lock name %q", otherName)
	}
	if again := writeLockName(prefix, keyedScope); again != keyedName {
		t.Fatalf("same scope derives %q then %q", keyedName, again)
	}
}
