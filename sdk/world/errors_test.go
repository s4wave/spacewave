package s4wave_world

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// TestOperationResponsesPreserveTypedErrors checks the error representation
// after protobuf transport for both World and object operations.
func TestOperationResponsesPreserveTypedErrors(t *testing.T) {
	for _, code := range []string{"VERSION_MISMATCH", "VALIDATION", "LIMIT"} {
		// Decode each wire response before checking the public Go error contract.
		worldResponse := &ApplyWorldOpResponse{RejectionCode: code, RejectionMessage: "Change rejected"}
		objectResponse := &ApplyObjectOpResponse{RejectionCode: code, RejectionMessage: "Change rejected"}
		worldData, err := worldResponse.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		objectData, err := objectResponse.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		worldDecoded := &ApplyWorldOpResponse{}
		if err := worldDecoded.UnmarshalVT(worldData); err != nil {
			t.Fatal(err)
		}
		objectDecoded := &ApplyObjectOpResponse{}
		if err := objectDecoded.UnmarshalVT(objectData); err != nil {
			t.Fatal(err)
		}

		// Callers can branch on the stable code without parsing human prose.
		for _, result := range []error{worldDecoded.GetError(), objectDecoded.GetError()} {
			var rejection *world.OperationRejection
			if !errors.As(result, &rejection) || rejection.Code != code || rejection.Message != "Change rejected" {
				t.Fatalf("rejection = %#v, want %s", result, code)
			}
		}
	}

	// Unhandled operations retain their sentinel, and successful responses stay nil.
	for _, result := range []error{
		(&ApplyWorldOpResponse{ErrorCode: WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP}).GetError(),
		(&ApplyObjectOpResponse{ErrorCode: WorldErrorCode_WORLD_ERROR_CODE_UNHANDLED_OP}).GetError(),
	} {
		if !errors.Is(result, world.ErrUnhandledOp) {
			t.Fatalf("unhandled error = %v", result)
		}
	}
	if (&ApplyWorldOpResponse{}).GetError() != nil || (&ApplyObjectOpResponse{}).GetError() != nil {
		t.Fatal("successful response returned an error")
	}
}
