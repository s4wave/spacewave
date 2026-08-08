//go:build !js && !windows

package nativeendpoint

import (
	"context"
	"errors"
	"unicode"
	"unicode/utf8"

	native "github.com/s4wave/spacewave/sdk/viewer/native"
)

const maxRequestIDBytes = native.MaxIdentityBytes

var errInvalidStateRequest = errors.New("native endpoint: invalid state request")

// stateService exposes one selected state atom over SRPC.
type stateService struct {
	// store persists the selected viewer state.
	store stateStore
	// stateKey identifies the state exposed by this service.
	stateKey string
}

// stateStore is the persistence boundary required by stateService.
type stateStore interface {
	Get(context.Context) (string, uint64, error)
	Set(context.Context, string) (uint64, error)
}

// newStateService binds one state store to its selected state key.
func newStateService(store stateStore, stateKey string) *stateService {
	return &stateService{store: store, stateKey: stateKey}
}

// Load returns the validated selected viewer state.
func (s *stateService) Load(ctx context.Context, req *native.NativeViewerStateLoadRequest) (*native.NativeViewerStateLoadResponse, error) {
	if req == nil || req.GetStateKey() != s.stateKey {
		return nil, errInvalidStateRequest
	}
	stateJSON, _, err := s.store.Get(ctx)
	if err != nil {
		return nil, err
	}
	state := new(native.NativeViewerSelectedState)
	if stateJSON != "" && stateJSON != "{}" {
		if err := state.UnmarshalJSON([]byte(stateJSON)); err != nil {
			return nil, err
		}
	}
	if err := native.ValidateSelectedState(state); err != nil {
		return nil, err
	}
	return &native.NativeViewerStateLoadResponse{State: state.CloneVT(), StateKey: s.stateKey}, nil
}

// Save validates and persists the selected viewer state.
func (s *stateService) Save(ctx context.Context, req *native.NativeViewerStateSaveRequest) (*native.NativeViewerStateSaveResponse, error) {
	if req == nil || req.GetStateKey() != s.stateKey || !validRequestID(req.GetRequestId()) || req.GetState() == nil {
		return nil, errInvalidStateRequest
	}
	if err := native.ValidateSelectedState(req.GetState()); err != nil {
		return nil, err
	}
	stateJSON, err := req.GetState().MarshalJSON()
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Set(ctx, string(stateJSON)); err != nil {
		return nil, err
	}
	return &native.NativeViewerStateSaveResponse{Accepted: true, StateKey: s.stateKey, RequestId: req.GetRequestId()}, nil
}

// validRequestID accepts non-empty bounded UTF-8 request IDs without controls.
func validRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
