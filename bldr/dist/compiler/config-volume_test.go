package bldr_dist_compiler

import (
	"testing"

	"github.com/aperturerobotics/util/enabled"
)

// TestConfigVolumeMerge preserves explicit layout selection through layered
// configuration and permits a later explicit override in either direction.
func TestConfigVolumeMerge(t *testing.T) {
	// An omitted override preserves the base distribution's selected layout.
	config := &Config{EmbedNativeVolume: enabled.Enabled_ENABLE}
	config.Merge(&Config{})
	if !config.GetEmbedNativeVolume().IsEnabled(false) {
		t.Fatal("an unspecified override disabled the embedded volume")
	}

	// A platform-specific override can explicitly restore the sidecar layout.
	config.Merge(&Config{EmbedNativeVolume: enabled.Enabled_DISABLE})
	if config.GetEmbedNativeVolume().IsEnabled(false) {
		t.Fatal("explicit disable did not select a sidecar")
	}

	// A later explicit selection can return to a single executable.
	config.Merge(&Config{EmbedNativeVolume: enabled.Enabled_ENABLE})
	if !config.GetEmbedNativeVolume().IsEnabled(false) {
		t.Fatal("explicit enable did not select the embedded volume")
	}
}
