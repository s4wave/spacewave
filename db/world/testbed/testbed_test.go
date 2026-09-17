package testbed

import (
	"context"
	"testing"
)

// mustDefaultTB is a testing.TB stub recording Fatal and Cleanup calls.
type mustDefaultTB struct {
	testing.TB

	fatalCalled bool
	cleanups    []func()
}

// fatalPanic is raised by Fatal to stop MustDefault like testing.Fatal does.
type fatalPanic struct{}

func (f *mustDefaultTB) Helper() {}

func (f *mustDefaultTB) Fatal(_ ...any) {
	f.fatalCalled = true
	panic(fatalPanic{})
}

func (f *mustDefaultTB) Cleanup(fn func()) {
	f.cleanups = append(f.cleanups, fn)
}

// TestMustDefaultFatalOnConstructionError verifies construction failure fails t.
func TestMustDefaultFatalOnConstructionError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalPanic); !ok {
				panic(r)
			}
		}
	}()

	ftb := &mustDefaultTB{}
	tb := MustDefault(ftb, context.Background(), struct{}{})
	tb.Release()
	t.Error("MustDefault returned a testbed after a construction error")
}

// TestMustDefaultRegistersRelease verifies the success path registers
// tb.Release with t.Cleanup exactly once.
func TestMustDefaultRegistersRelease(t *testing.T) {
	ftb := &mustDefaultTB{}
	tb := MustDefault(ftb, context.Background())
	if tb == nil {
		t.Fatal("MustDefault returned nil testbed")
	}

	// Observe Release through a registered release function.
	released := false
	tb.AddReleaseFunc(func() { released = true })
	if len(ftb.cleanups) != 1 {
		t.Fatalf("expected 1 registered cleanup, got %d", len(ftb.cleanups))
	}
	ftb.cleanups[0]()
	if !released {
		t.Error("registered cleanup did not release the testbed")
	}
}
