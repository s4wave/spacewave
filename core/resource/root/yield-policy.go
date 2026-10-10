//go:build !js

package resource_root

import (
	"context"

	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// WatchListenerYieldPrompts streams the set of pending takeover
// prompts surfaced by the desktop resource listener's yield broker.
// The UI subscribes to this stream, renders a modal for the first
// prompt, and resolves via RespondToListenerYieldPrompt. A runtime without a
// resource listener sends one empty prompt set.
func (s *CoreRootServer) WatchListenerYieldPrompts(
	_ *s4wave_root.WatchListenerYieldPromptsRequest,
	strm s4wave_root.SRPCRootResourceService_WatchListenerYieldPromptsStream,
) error {
	// Resolve the listener broker or send an empty prompt set.
	ctx := strm.Context()
	broker := s.getYieldBroker()
	if broker == nil {
		if err := strm.Send(&s4wave_root.WatchListenerYieldPromptsResponse{}); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}

	// Watch the broker and send changed takeover prompts.
	var sentIDs []string
	for {
		snapshot, waitCh := broker.SnapshotPrompts()
		if !promptIDsEqual(sentIDs, snapshot) {
			prompts := make([]*s4wave_root.ListenerYieldPrompt, 0, len(snapshot))
			sentIDs = sentIDs[:0]
			for _, p := range snapshot {
				prompts = append(prompts, &s4wave_root.ListenerYieldPrompt{
					PromptId:       p.ID,
					RequesterName:  p.RequesterName,
					SocketPath:     p.SocketPath,
					DeadlineUnixMs: p.DeadlineUnixMs,
				})
				sentIDs = append(sentIDs, p.ID)
			}
			if err := strm.Send(&s4wave_root.WatchListenerYieldPromptsResponse{
				Prompts: prompts,
			}); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// RespondToListenerYieldPrompt resolves a pending prompt with the
// user's decision.
func (s *CoreRootServer) RespondToListenerYieldPrompt(
	_ context.Context,
	req *s4wave_root.RespondToListenerYieldPromptRequest,
) (*s4wave_root.RespondToListenerYieldPromptResponse, error) {
	broker := s.getYieldBroker()
	if broker == nil {
		return &s4wave_root.RespondToListenerYieldPromptResponse{NotFound: true}, nil
	}
	if err := broker.ResolvePrompt(req.GetPromptId(), req.GetAllow()); err != nil {
		return &s4wave_root.RespondToListenerYieldPromptResponse{NotFound: true}, nil
	}
	return &s4wave_root.RespondToListenerYieldPromptResponse{}, nil
}

// WatchRuntimeHandoff streams the current runtime handoff state so
// the UI can render the "Runtime handed off" banner and the Reclaim
// action. A runtime without a resource listener sends one inactive state.
func (s *CoreRootServer) WatchRuntimeHandoff(
	_ *s4wave_root.WatchRuntimeHandoffRequest,
	strm s4wave_root.SRPCRootResourceService_WatchRuntimeHandoffStream,
) error {
	// Resolve the listener broker or send the inactive handoff state.
	ctx := strm.Context()
	broker := s.getYieldBroker()
	if broker == nil {
		if err := strm.Send(&s4wave_root.WatchRuntimeHandoffResponse{State: &s4wave_root.RuntimeHandoffState{}}); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}

	// Watch the broker and send changed runtime handoff state.
	var prev s4wave_root.RuntimeHandoffState
	first := true
	for {
		snapshot, waitCh := broker.SnapshotHandoff()
		current := s4wave_root.RuntimeHandoffState{
			Active:        snapshot.Active,
			RequesterName: snapshot.RequesterName,
			SocketPath:    snapshot.SocketPath,
			SinceUnixMs:   snapshot.SinceUnixMs,
		}
		if first || !handoffEqual(prev, current) {
			first = false
			prev = current
			if err := strm.Send(&s4wave_root.WatchRuntimeHandoffResponse{
				State: &current,
			}); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// ReclaimRuntime signals the listener controller to reclaim the
// runtime from the remote owner.
func (s *CoreRootServer) ReclaimRuntime(
	_ context.Context,
	_ *s4wave_root.ReclaimRuntimeRequest,
) (*s4wave_root.ReclaimRuntimeResponse, error) {
	broker := s.getYieldBroker()
	if broker == nil {
		return &s4wave_root.ReclaimRuntimeResponse{}, nil
	}
	return &s4wave_root.ReclaimRuntimeResponse{Reclaimed: broker.Reclaim()}, nil
}

// promptIDsEqual checks whether the set of prompt ids in snapshot
// matches the already-sent list.
func promptIDsEqual(sent []string, snapshot []yield_policy.Prompt) bool {
	if len(sent) != len(snapshot) {
		return false
	}
	for i := range snapshot {
		if sent[i] != snapshot[i].ID {
			return false
		}
	}
	return true
}

// handoffEqual compares two handoff state snapshots for equality.
func handoffEqual(a, b s4wave_root.RuntimeHandoffState) bool {
	return a.GetActive() == b.GetActive() &&
		a.GetRequesterName() == b.GetRequesterName() &&
		a.GetSocketPath() == b.GetSocketPath() &&
		a.GetSinceUnixMs() == b.GetSinceUnixMs()
}
