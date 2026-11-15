package plugin_space

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// admissionGate admits the World manifests a Space names in its loads. A
// Space lists plugins any member can name, so the Space sends a World root
// only while an AdmitPluginRoot for the session peer has a value: the release,
// the development build or the device's person admitted it.
//
// The host scheduler also refuses an unadmitted root, and that is the
// enforcement; this gate decides what to request. It falls back to the
// release's plain load when no World root is admitted, withdraws a pinned
// plugin, and releases a load whose root lost admission, none of which a host
// given only unadmitted manifests can do: it can only wait.
//
// A root with no value is refused: no resolver, no record and no peer all
// refuse. A root is pending until its directive first goes idle or admits it,
// so a pass neither falls back, withdraws nor releases on a watch that has not
// answered yet. The watch
// loop goroutine owns the gate; a watch's callback only flips its flags and
// wakes the loop.
type admissionGate struct {
	// b is the bus the admission directives are added on.
	b bus.Bus
	// peerID is the session peer the device's approvals are keyed by.
	peerID string
	// wake wakes the watch loop when an admission changes.
	wake func()
	// watches is the watch of each plugin's root, by plugin ID and root.
	watches map[sourceRoot]*admissionWatch
}

// admissionWatch holds one AdmitPluginRoot directive.
type admissionWatch struct {
	// mtx guards admitted and decided.
	mtx sync.Mutex
	// admitted is set while the directive has a value.
	admitted bool
	// decided is set once the directive went idle or had a value.
	decided bool
	// used is set when the current pass asked for the root.
	used bool
	// release releases the directive and its callback.
	release func()
}

// newAdmissionGate constructs an admissionGate.
func newAdmissionGate(b bus.Bus, peerID string, wake func()) *admissionGate {
	return &admissionGate{b: b, peerID: peerID, wake: wake, watches: make(map[sourceRoot]*admissionWatch)}
}

// admitted returns, keeping their order, the refs whose roots are admitted
// for pluginID and the refs not refused: those admitted and those whose
// admission has not answered yet. A root is pending when open holds more refs
// than admitted. It starts watching each root not yet watched.
func (g *admissionGate) admitted(ctx context.Context, pluginID string, refs []*manifest.ManifestRef) (admitted, open []*manifest.ManifestRef) {
	for _, ref := range refs {
		w := g.watch(ctx, sourceRoot{pluginID: pluginID, root: manifestRoot(ref)})
		w.used = true
		w.mtx.Lock()
		ok, decided := w.admitted, w.decided
		w.mtx.Unlock()
		if ok {
			admitted = append(admitted, ref)
		}
		if ok || !decided {
			open = append(open, ref)
		}
	}
	return admitted, open
}

// watch returns the watch of key, adding its directive on first use. A root
// with no peer, no root or a directive the bus refuses is never admitted.
func (g *admissionGate) watch(ctx context.Context, key sourceRoot) *admissionWatch {
	// Reuse the watch of a root already asked about.
	if w := g.watches[key]; w != nil {
		return w
	}
	w := &admissionWatch{release: func() {}, decided: true}
	g.watches[key] = w
	if g.peerID == "" || key.root == "" {
		return w
	}

	// Ask the bus whether the root is admitted for the session peer.
	di, ref, err := g.b.AddDirective(ctx, bldr_plugin.NewAdmitPluginRoot(g.peerID, key.pluginID, key.root), nil)
	if err != nil {
		return w
	}

	// Record the first answer and each flip of admission, and wake the loop
	// to resend its loads.
	w.decided = false
	releaseState := di.AddStateCallback(func(idle bool, _ []error, vals []directive.AttachedValue) {
		// Store the new admission and wake only on a change.
		admitted := len(vals) != 0
		w.mtx.Lock()
		changed := w.admitted != admitted || (!w.decided && (idle || admitted))
		w.admitted = admitted
		w.decided = w.decided || idle || admitted
		w.mtx.Unlock()
		if changed {
			g.wake()
		}
	})
	w.release = func() {
		releaseState()
		ref.Release()
	}
	return w
}

// refused reports whether any root of refs answered that it is not admitted
// for pluginID. A root still pending is not refused.
func (g *admissionGate) refused(ctx context.Context, pluginID string, refs []*manifest.ManifestRef) bool {
	_, open := g.admitted(ctx, pluginID, refs)
	return len(open) != len(refs)
}

// prune releases the watches the pass did not ask about and clears the marks
// for the next pass.
func (g *admissionGate) prune() {
	for key, w := range g.watches {
		if !w.used {
			w.release()
			delete(g.watches, key)
			continue
		}
		w.used = false
	}
}

// release releases every watch.
func (g *admissionGate) release() {
	for _, w := range g.watches {
		w.release()
	}
	clear(g.watches)
}

// manifestRoot returns the root a scheduler asks admission for.
func manifestRoot(ref *manifest.ManifestRef) string {
	return ref.GetManifestRef().GetRootRef().GetHash().MarshalString()
}

// artifactKeys returns the sorted artifact keys of refs, identifying a load.
func artifactKeys(refs []*manifest.ManifestRef) []string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, manifest.NewManifestArtifactKey(ref.GetManifestRef()))
	}
	slices.Sort(keys)
	return keys
}
