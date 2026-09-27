package plugin_host_scheduler

import (
	"strings"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
)

// manifestCandidate pairs a selectable manifest with its available execution host.
type manifestCandidate struct {
	// ref carries the canonical manifest identity and release metadata.
	ref *bldr_manifest.ManifestRef
	// host executes this manifest's platform.
	host plugin_host.PluginHost
}

// betterThan orders initial selection by platform, revision, and content identity.
func (c *manifestCandidate) betterThan(other *manifestCandidate) bool {
	// Prefer an available candidate over an empty selection.
	if other == nil {
		return true
	}

	// Prefer native desktop execution and JavaScript browser execution.
	rank := platformPreferenceRank(c.host.GetPlatformId())
	otherRank := platformPreferenceRank(other.host.GetPlatformId())
	if rank != otherRank {
		return rank < otherRank
	}

	// Prefer the latest revision on the chosen platform.
	rev := c.ref.GetMeta().GetRev()
	otherRev := other.ref.GetMeta().GetRev()
	if rev != otherRev {
		return rev > otherRev
	}

	// Resolve equal revisions deterministically before any runtime is admitted.
	if ref, otherRef := c.ref.String(), other.ref.String(); ref != otherRef {
		return ref > otherRef
	}
	return c.host.GetPlatformId() > other.host.GetPlatformId()
}

// shouldRemainCurrent keeps a still-selectable admitted generation until a
// newer revision arrives. Late same-revision variants cannot cancel its RPCs.
//
// A native build always replaces a non-native execution: native and
// JavaScript builds carry independent revision counters, so a higher
// JavaScript revision says nothing about which build is newer.
func (c *manifestCandidate) shouldRemainCurrent(best *manifestCandidate) bool {
	if c == nil {
		return false
	}
	if best == nil {
		return true
	}
	if platformPreferenceRank(best.host.GetPlatformId()) == 0 &&
		platformPreferenceRank(c.host.GetPlatformId()) != 0 {
		return false
	}
	return c.ref.GetMeta().GetRev() >= best.ref.GetMeta().GetRev()
}

// matchesState checks the execution identity while allowing a manifest's
// immutable content to move from an external bucket into local storage.
func (c *manifestCandidate) matchesState(state *executePluginArgs) bool {
	if state == nil || state.pluginHost != c.host || state.manifestSnapshot == nil {
		return false
	}
	return manifest_world.ManifestObjectRefsSameExecutable(
		state.manifestSnapshot.GetManifestRef(),
		c.ref.GetManifestRef(),
	)
}

// platformPreferenceRank prefers native desktop hosts, then JavaScript, then
// legacy browser platforms. Lower ranks are preferred.
func platformPreferenceRank(platformID string) int {
	if strings.HasPrefix(platformID, bldr_platform.PlatformID_WEB+"/") {
		return 2
	}
	if platformID == bldr_platform.PlatformID_JS {
		return 1
	}
	return 0
}
