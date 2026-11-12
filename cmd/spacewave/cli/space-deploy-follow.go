package spacewave_cli

import (
	"context"
	"io"
	"strconv"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/bucket"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
)

// swapWaitingNextStep says how a swap of pluginID waiting on the running
// worker ends. The listing cannot name the host kind: js plugins run both in
// browser workers and in the desktop quickjs host. A worker that never answers
// nor exits stays until the plugin is restarted.
func swapWaitingNextStep(pluginID string) string {
	return "it swaps when the worker answers or exits (a browser worker exits when its page reloads);" +
		" if it does neither, run spacewave plugin restart " + pluginID
}

// errDeployWaitPassed ends a deploy follow whose --wait deadline passed.
var errDeployWaitPassed = errors.New("--wait deadline passed")

// deployOutcome is where following a deploy stands.
type deployOutcome int

const (
	// deployFollowing waits for the next listing state.
	deployFollowing deployOutcome = iota
	// deployLive means a deployed manifest runs.
	deployLive
	// deployFailed means the deployed manifest failed with no retry.
	deployFailed
	// deployStored means the selection is written but the space does not
	// list the plugin, so nothing loads it until the person adds the plugin.
	deployStored
)

// deployState is one state of the deployed manifest set on its plugin, as the
// Space contents listing shows it.
type deployState struct {
	// outcome says whether following goes on or ends.
	outcome deployOutcome
	// line describes the state for the person.
	line string
	// next is the next step if following stops in this state.
	next string
}

// deployedSet is the manifest set a deploy wrote, matched against the
// listing's manifest refs by root block, since the Space stores the deployed
// blocks under the same refs.
type deployedSet struct {
	// manifestID is the plugin the set deploys.
	manifestID string
	// refs are the deployed manifests, one per platform.
	refs []*bldr_manifest.ManifestRef
}

// holds reports whether the listing ref names one of the deployed manifests.
func (d *deployedSet) holds(ref string) bool {
	objRef, err := bucket.ParseObjectRef(ref)
	if err != nil || objRef == nil {
		return false
	}
	for _, deployed := range d.refs {
		if deployed.GetManifestRef().GetRootRef().EqualsRef(objRef.GetRootRef()) {
			return true
		}
	}
	return false
}

// holdsRev reports whether one of the deployed manifests carries rev.
func (d *deployedSet) holdsRev(rev uint64) bool {
	for _, deployed := range d.refs {
		if deployed.GetMeta().GetRev() == rev {
			return true
		}
	}
	return false
}

// findDeployPlugin returns the listing row for the deployed plugin, or nil.
func findDeployPlugin(state *s4wave_space.SpaceContentsState, manifestID string) *s4wave_space.SpacePluginStatus {
	for _, p := range state.GetPlugins() {
		if p.GetPluginId() == manifestID {
			return p
		}
	}
	return nil
}

// classifyDeploy derives the deploy's state from the plugin's listing row and
// its swap state, with times as clock times in loc.
func classifyDeploy(p *s4wave_space.SpacePluginStatus, set *deployedSet, loc *time.Location) deployState {
	// A plugin the Space does not list never loads, so following ends with
	// the stored selection and the step that loads it.
	clock := func(ts *timestamppb.Timestamp) string { return ts.AsTime().In(loc).Format("15:04:05") }
	rev := func(r uint64) string { return "rev " + strconv.FormatUint(r, 10) }
	if p == nil {
		return deployState{
			outcome: deployStored,
			line:    set.manifestID + " is not listed in the space",
			next:    "the selection is written; add the plugin with spacewave plugin add " + set.manifestID,
		}
	}
	if set.holds(p.GetRunningManifestRef()) {
		return deployState{outcome: deployLive, line: rev(p.GetRunningManifestRev()) + " live"}
	}

	// A swap toward a deployed manifest names what it waits on.
	swap := p.GetSwap()
	if st := swap.GetFailed(); st != nil && set.holds(st.GetCandidateManifestRef()) {
		failed := rev(st.GetCandidateManifestRev()) + " failed at " + clock(st.GetFailedAt()) + ": " + st.GetErrorMessage()
		if st.GetRetryAt() == nil {
			return deployState{outcome: deployFailed, line: failed, next: "fix the error and deploy again"}
		}
		return deployState{
			line: failed + "; retry at " + clock(st.GetRetryAt()),
			next: "the host retries at " + clock(st.GetRetryAt()) + "; follow it with spacewave plugin list --watch",
		}
	}
	if st := swap.GetWaiting(); st != nil && set.holds(st.GetCandidateManifestRef()) {
		return deployState{
			line: rev(st.GetCandidateManifestRev()) + " waits since " + clock(st.GetSinceAt()) +
				" for the running worker (" + rev(p.GetRunningManifestRev()) + ") to answer its activation check",
			next: swapWaitingNextStep(set.manifestID),
		}
	}
	if st := swap.GetPreparing(); st != nil && set.holds(st.GetCandidateManifestRef()) {
		return deployState{
			line: "preparing " + rev(st.GetCandidateManifestRev()) + " since " + clock(st.GetStartedAt()),
			next: "the host is still starting it; follow it with spacewave plugin list --watch",
		}
	}

	// Otherwise the host has selected the deployed manifest or not yet.
	if set.holds(p.GetManifestRef()) {
		return deployState{
			line: rev(p.GetManifestRev()) + " selected",
			next: "the host has not started it; follow it with spacewave plugin list --watch",
		}
	}
	line := "waiting for the host to select the deployed manifest"
	if p.GetManifestRef() != "" {
		line += " (" + rev(p.GetManifestRev()) + " selected)"
	}
	return deployState{
		line: line,
		next: "the host may prefer a higher rev or run none of the deployed platforms; check spacewave plugin list",
	}
}

// warnDeployOverlap names what the listing held before the deploy that makes
// the deploy change nothing: the deployed manifest already running, or another
// build of a deployed rev already selected, which an equal rev may not replace.
func warnDeployOverlap(w io.Writer, p *s4wave_space.SpacePluginStatus, set *deployedSet) {
	if p == nil {
		return
	}
	rev := strconv.FormatUint(p.GetManifestRev(), 10)
	switch {
	case set.holds(p.GetRunningManifestRef()):
		_, _ = io.WriteString(w, "warning: rev "+strconv.FormatUint(p.GetRunningManifestRev(), 10)+" already runs this build\n")
	case p.GetManifestRef() != "" && !set.holds(p.GetManifestRef()) && set.holdsRev(p.GetManifestRev()):
		_, _ = io.WriteString(w, "warning: the space already selects another build of rev "+rev+
			"; an equal rev may not replace it, so raise the rev to deploy this build\n")
	}
}

// followDeploy prints the deploy's state from start and from each listing
// state recv returns, each change once. It returns nil once a deployed
// manifest runs, or with the add step when the space does not list the
// plugin, since the selection is written. When the swap fails with no retry,
// or recv stops because ctx ended (errDeployWaitPassed as its cause for the
// deadline) or the stream broke, it prints the last state and its next step and returns an error.
func followDeploy(
	ctx context.Context,
	w io.Writer,
	start *s4wave_space.SpaceContentsState,
	recv func() (*s4wave_space.SpaceContentsState, error),
	set *deployedSet,
	loc *time.Location,
) error {
	// Classify the state the deploy started from, then each change.
	var last deployState
	state := start
	for {
		next := classifyDeploy(findDeployPlugin(state, set.manifestID), set, loc)
		if next.line != last.line {
			_, _ = io.WriteString(w, next.line+"\n")
		}
		last = next
		switch last.outcome {
		case deployFollowing:
			// Read the next state below.
		case deployLive:
			return nil
		case deployStored:
			_, _ = io.WriteString(w, "next: "+last.next+"\n")
			return nil
		case deployFailed:
			_, _ = io.WriteString(w, "next: "+last.next+"\n")
			return errors.New(set.manifestID + " " + last.line)
		}

		// Read the next listing state, or stop with the last one.
		var err error
		state, err = recv()
		if err != nil {
			_, _ = io.WriteString(w, "not live yet: "+last.line+"\nnext: "+last.next+"\n")
			switch {
			case errors.Is(context.Cause(ctx), errDeployWaitPassed):
				return errors.New(set.manifestID + " not live before the --wait deadline")
			case ctx.Err() != nil:
				return errors.New(set.manifestID + " not live: interrupted")
			default:
				return errors.Wrap(err, "watch space contents")
			}
		}
	}
}
