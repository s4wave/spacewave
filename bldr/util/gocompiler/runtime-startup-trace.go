package gocompiler

import "os"

const (
	// RuntimeStartupTraceEnv enables the opt-in browser runtime startup trace.
	RuntimeStartupTraceEnv = "BLDR_RUNTIME_STARTUP_TRACE"
	// RuntimeStartupTraceBuildTag includes the browser startup trace hook.
	RuntimeStartupTraceBuildTag = "bldr_startup_trace"
)

// RuntimeStartupTraceBuildTags returns the opt-in startup trace build tag.
func RuntimeStartupTraceBuildTags() []string {
	if os.Getenv(RuntimeStartupTraceEnv) == "" {
		return nil
	}
	return []string{RuntimeStartupTraceBuildTag}
}

// RuntimeStartupTraceBuildTagsForWebWasm returns the opt-in startup trace tag
// only for the supported non-TinyGo browser compiler.
func RuntimeStartupTraceBuildTagsForWebWasm(isWebPlatform, useTinygo bool) []string {
	if !isWebPlatform || useTinygo {
		return nil
	}
	return RuntimeStartupTraceBuildTags()
}
