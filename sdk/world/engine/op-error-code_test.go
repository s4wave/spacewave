package sdk_world_engine

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// worldOpReplyService answers ApplyWorldOp with a reply carrying one error
// code, decoded from its wire form as a client receives it.
type worldOpReplyService struct {
	s4wave_world.SRPCWorldStateResourceServiceClient
	code s4wave_world.WorldErrorCode
}

func (s *worldOpReplyService) ApplyWorldOp(context.Context, *s4wave_world.ApplyWorldOpRequest) (*s4wave_world.ApplyWorldOpResponse, error) {
	data, err := (&s4wave_world.ApplyWorldOpResponse{Seqno: 7, ErrorCode: s.code}).MarshalVT()
	if err != nil {
		return nil, err
	}
	resp := &s4wave_world.ApplyWorldOpResponse{}
	return resp, resp.UnmarshalVT(data)
}

// objectOpReplyService answers ApplyObjectOp as worldOpReplyService does.
type objectOpReplyService struct {
	s4wave_world.SRPCObjectStateResourceServiceClient
	code s4wave_world.WorldErrorCode
}

func (s *objectOpReplyService) ApplyObjectOp(context.Context, *s4wave_world.ApplyObjectOpRequest) (*s4wave_world.ApplyObjectOpResponse, error) {
	data, err := (&s4wave_world.ApplyObjectOpResponse{Rev: 7, ErrorCode: s.code}).MarshalVT()
	if err != nil {
		return nil, err
	}
	resp := &s4wave_world.ApplyObjectOpResponse{}
	return resp, resp.UnmarshalVT(data)
}

// emptyOp is an operation with an empty body.
type emptyOp struct {
	world.Operation
}

func (emptyOp) TypeID() string {
	return "test/empty"
}

func (emptyOp) MarshalVT() ([]byte, error) {
	return nil, nil
}

// TestApplyOpErrorCodes checks that every World error code an operation reply
// carries reaches the SDK caller as an error of its type, and only the unset
// code as success. The cases come from the enum, so a code added without a
// mapping fails here.
func TestApplyOpErrorCodes(t *testing.T) {
	// want reports whether err is the type a code stands for.
	want := map[s4wave_world.WorldErrorCode]func(err error) bool{
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNSPECIFIED: func(err error) bool { return err == nil },
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP: func(err error) bool {
			return errors.Is(err, world.ErrUnhandledOp)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_TYPED_OBJECT_GRANT_RETIRED: func(err error) bool {
			return errors.Is(err, s4wave_world.ErrTypedObjectGrantRetired)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_OBJECT_NOT_FOUND: func(err error) bool {
			return errors.Is(err, world.ErrObjectNotFound)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_OP_TIMEOUT: func(err error) bool {
			var e *world.PluginOpTimeoutError
			return errors.As(err, &e)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_UNAVAILABLE: func(err error) bool {
			var e *world.PluginUnavailableError
			return errors.As(err, &e) && e.Sent
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_PLUGIN_EXECUTABLE_MISSING: func(err error) bool {
			var e *world.PluginExecutableMissingError
			return errors.As(err, &e)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_SUPERSEDED: func(err error) bool {
			return errors.Is(err, world.ErrSuperseded)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_STALE_GENERATION: func(err error) bool {
			return errors.Is(err, coord.ErrStaleGeneration)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_ROOT_NOT_HELD: func(err error) bool {
			return errors.Is(err, block.ErrRootNotHeld)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_REPLAY_WAITS: func(err error) bool {
			return errors.Is(err, sobject.ErrReplayWaits)
		},
		s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_DURABILITY_UNKNOWN: func(err error) bool {
			return errors.Is(err, world.ErrDurabilityUnknown)
		},
	}

	// A code past the enum stands for one a newer server defines.
	unknown := s4wave_world.WorldErrorCode(len(s4wave_world.WorldErrorCode_name))
	codes := []s4wave_world.WorldErrorCode{unknown}
	for value := range s4wave_world.WorldErrorCode_name {
		codes = append(codes, s4wave_world.WorldErrorCode(value))
	}
	want[unknown] = func(err error) bool {
		var e *s4wave_world.UnknownErrorCodeError
		return errors.As(err, &e) && e.Code == unknown
	}

	ctx := context.Background()
	for _, code := range codes {
		check, ok := want[code]
		if !ok {
			t.Fatalf("code %v has no expected error", code)
		}
		ws := &SDKWorldState{service: &worldOpReplyService{code: code}}
		obj := &SDKObjectState{service: &objectOpReplyService{code: code}}

		seqno, _, wsErr := ws.ApplyWorldOp(ctx, emptyOp{}, "")
		rev, _, objErr := obj.ApplyObjectOp(ctx, emptyOp{}, "")
		for _, err := range []error{wsErr, objErr} {
			if !check(err) {
				t.Fatalf("code %v: error = %v (%T)", code, err, err)
			}
			if err != nil && err.Error() == "" {
				t.Fatalf("code %v: empty error text", code)
			}
		}
		if code == s4wave_world.WorldErrorCode_WORLD_ERROR_CODE_UNSPECIFIED && (seqno != 7 || rev != 7) {
			t.Fatalf("success seqno, rev = %d, %d, want 7, 7", seqno, rev)
		}
	}
}
