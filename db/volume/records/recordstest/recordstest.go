// Package recordstest checks a records.Store against its contract, so native
// tests and browser fixtures run the same checks on every implementation.
package recordstest

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/records"
)

// Check runs the contract checks on an empty store.
func Check(ctx context.Context, s records.Store) error {
	// A commit applies its ops in order.
	ops := []records.Op{
		{Key: []byte("a/1"), Value: []byte("one")},
		{Key: []byte("a/2"), Value: []byte("two")},
		{Key: []byte("a/3"), Value: []byte("three")},
		{Key: []byte("a/2"), Delete: true},
		{Key: []byte("b"), Value: []byte("bee")},
		{Key: []byte("b"), Value: []byte("B")},
		{Key: []byte{0xff, 0xff}, Value: []byte("high")},
		{Key: []byte{0xff, 0xff, 0x00}, Value: []byte("higher")},
	}
	if err := s.Commit(ctx, ops, true); err != nil {
		return err
	}

	// Get returns each value and nil for an absent key.
	keys := [][]byte{[]byte("a/1"), []byte("a/2"), []byte("b"), []byte("missing")}
	values, err := s.Get(ctx, keys)
	if err != nil {
		return err
	}
	want := [][]byte{[]byte("one"), nil, []byte("B"), nil}
	if !slices.EqualFunc(values, want, bytesEqual) {
		return errors.Errorf("get %q, want %q", values, want)
	}

	// Has reports presence.
	has, err := s.Has(ctx, keys)
	if err != nil {
		return err
	}
	if !slices.Equal(has, []bool{true, false, true, false}) {
		return errors.Errorf("has %v", has)
	}

	// Scan visits a prefix in key order, including a prefix of all 0xff.
	if err := expectScan(ctx, s, []byte("a/"), "a/1=one", "a/3=three"); err != nil {
		return err
	}
	if err := expectScan(ctx, s, []byte{0xff, 0xff}, "\xff\xff=high", "\xff\xff\x00=higher"); err != nil {
		return err
	}
	if err := expectScan(ctx, s, nil, "a/1=one", "a/3=three", "b=B", "\xff\xff=high", "\xff\xff\x00=higher"); err != nil {
		return err
	}

	// A relaxed commit is visible to the next call, and an empty commit
	// succeeds.
	if err := s.Commit(ctx, []records.Op{{Key: []byte("a/1"), Delete: true}}, false); err != nil {
		return err
	}
	if err := s.Commit(ctx, nil, true); err != nil {
		return err
	}
	return expectScan(ctx, s, []byte("a/"), "a/3=three")
}

// expectScan checks the records a prefix scan visits.
func expectScan(ctx context.Context, s records.Store, prefix []byte, want ...string) error {
	var got []string
	err := s.Scan(ctx, prefix, func(key, value []byte) error {
		got = append(got, string(key)+"="+string(value))
		return nil
	})
	if err != nil {
		return err
	}
	if !slices.Equal(got, want) {
		return errors.Errorf("scan %q: %q, want %q", prefix, got, want)
	}
	return nil
}

// bytesEqual compares two values, distinguishing nil from empty.
func bytesEqual(a, b []byte) bool {
	return (a == nil) == (b == nil) && bytes.Equal(a, b)
}
