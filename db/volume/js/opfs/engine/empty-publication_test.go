//go:build !js

package engine

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"
)

// TestOpenRemovesEmptyIntentWithoutLosingData proves interrupted intent creation is harmless.
func TestOpenRemovesEmptyIntentWithoutLosingData(t *testing.T) {
	ctx := t.Context()
	backend := newDiskBackend(t)
	engine, err := Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(ctx, []*Record{{Key: []byte("saved"), Value: []byte("durable data")}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if err := backend.Write(ctx, "intent", nil); err != nil {
		t.Fatal(err)
	}

	engine, err = Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	value, found, _, err := engine.Get(ctx, []byte("saved"))
	if err != nil || !found || string(value) != "durable data" {
		t.Fatalf("saved value = %q, %t, %v", value, found, err)
	}
	if _, err := backend.Read(ctx, "intent", 0, readAll); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty intent remains after recovery: %v", err)
	}
	if err := engine.Apply(ctx, []*Record{{Key: []byte("after"), Value: []byte("new write")}}); err != nil {
		t.Fatal(err)
	}
	value, found, _, err = engine.Get(ctx, []byte("after"))
	if err != nil || !found || string(value) != "new write" {
		t.Fatalf("new value = %q, %t, %v", value, found, err)
	}
}

// TestOpenIgnoresEmptyInitialRoot proves interrupted descriptor creation can restart.
func TestOpenIgnoresEmptyInitialRoot(t *testing.T) {
	ctx := t.Context()
	backend := newDiskBackend(t)
	if err := backend.Write(ctx, "root-0", nil); err != nil {
		t.Fatal(err)
	}

	engine, err := Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	if err := engine.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	value, found, _, err := engine.Get(ctx, []byte("key"))
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("new value = %q, %t, %v", value, found, err)
	}
}

// TestOpenRepairsEmptyInitialIdentity proves interrupted identity creation can restart.
func TestOpenRepairsEmptyInitialIdentity(t *testing.T) {
	ctx := t.Context()
	backend := newDiskBackend(t)
	if err := backend.Write(ctx, "identity", nil); err != nil {
		t.Fatal(err)
	}

	engine, err := Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	identity, err := backend.Read(ctx, "identity", 0, readAll)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identity, []byte("immutable-opfs-3\n")) {
		t.Fatalf("identity = %q", identity)
	}
	if err := engine.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	value, found, _, err := engine.Get(ctx, []byte("key"))
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("new value = %q, %t, %v", value, found, err)
	}
}

// TestOpenRejectsNonemptyMalformedIntent preserves committed-data corruption checks.
func TestOpenRejectsNonemptyMalformedIntent(t *testing.T) {
	ctx := t.Context()
	backend := newDiskBackend(t)
	engine, err := Open(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Apply(ctx, []*Record{{Key: []byte("saved"), Value: []byte("durable data")}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	malformed := []byte("nonempty malformed intent")
	if err := backend.Write(ctx, "intent", malformed); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(ctx, backend); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("open error = %v, want %v", err, ErrCorrupt)
	}
	data, err := backend.Read(ctx, "intent", 0, readAll)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, malformed) {
		t.Fatalf("malformed intent changed to %q", data)
	}
}
