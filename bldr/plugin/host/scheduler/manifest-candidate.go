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
	// host executes this manifest's platform, or is nil for served files.
	host plugin_host.PluginHost
}

// platformID returns the platform of the candidate's manifest.
func (c *manifestCandidate) platformID() string {
	return c.ref.GetMeta().GetPlatformId()
}

// rank orders candidates a host runs by platform preference and the
// candidates only served after them. Lower ranks are preferred.
func (c *manifestCandidate) rank() int {
	return candidateRank(c.platformID(), c.host)
}

// betterThan orders initial selection by platform, revision, and content identity.
func (c *manifestCandidate) betterThan(other *manifestCandidate) bool {
	// Prefer an available candidate over an empty selection.
	if other == nil {
		return true
	}

	// Prefer native desktop execution and JavaScript browser execution.
	if rank, otherRank := c.rank(), other.rank(); rank != otherRank {
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
	return c.platformID() > other.platformID()
}

// shouldRemainCurrent keeps a still-selectable admitted generation until a
// newer revision arrives. Late same-revision variants cannot cancel its RPCs.
//
// A native build always replaces a non-native execution: native and
// JavaScript builds carry independent revision counters, so a higher
// JavaScript revision says nothing about which build is newer. Any execution
// replaces served files.
func (c *manifestCandidate) shouldRemainCurrent(best *manifestCandidate) bool {
	// An empty generation yields to any candidate and holds against none.
	if c == nil {
		return false
	}
	if best == nil {
		return true
	}

	// Replace served files and non-native execution when a better kind exists.
	if best.host != nil && c.host == nil {
		return false
	}
	if best.rank() == 0 && c.rank() != 0 {
		return false
	}

	// Otherwise only a newer revision replaces the admitted generation.
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

// candidateRank ranks a manifest for platformID by its platform preference,
// or after every platform when no host runs it.
func candidateRank(platformID string, host plugin_host.PluginHost) int {
	if host == nil {
		return 3
	}
	return platformPreferenceRank(platformID)
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
