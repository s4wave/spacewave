package gocompiler

import (
	"slices"
	"testing"
)

// TestRuntimeStartupTraceBuildTags verifies environment-gated startup trace tags.
func TestRuntimeStartupTraceBuildTags(t *testing.T) {
	t.Setenv(RuntimeStartupTraceEnv, "")
	if tags := RuntimeStartupTraceBuildTags(); len(tags) != 0 {
		t.Fatalf("startup trace tags = %v, want none", tags)
	}

	t.Setenv(RuntimeStartupTraceEnv, "1")
	if tags := RuntimeStartupTraceBuildTags(); !slices.Equal(tags, []string{RuntimeStartupTraceBuildTag}) {
		t.Fatalf("startup trace tags = %v, want %s tag", tags, RuntimeStartupTraceBuildTag)
	}
}

// TestRuntimeStartupTraceBuildTagsForWebWasm verifies unsupported compiler modes receive no hook.
func TestRuntimeStartupTraceBuildTagsForWebWasm(t *testing.T) {
	t.Setenv(RuntimeStartupTraceEnv, "1")
	if tags := RuntimeStartupTraceBuildTagsForWebWasm(false, false); len(tags) != 0 {
		t.Fatalf("native startup trace tags = %v, want none", tags)
	}
	if tags := RuntimeStartupTraceBuildTagsForWebWasm(true, true); len(tags) != 0 {
		t.Fatalf("TinyGo startup trace tags = %v, want none", tags)
	}
	if tags := RuntimeStartupTraceBuildTagsForWebWasm(true, false); !slices.Equal(tags, []string{RuntimeStartupTraceBuildTag}) {
		t.Fatalf("Go WASM startup trace tags = %v, want %s tag", tags, RuntimeStartupTraceBuildTag)
	}
}
