package hydra_api

import (
	"context"
	"strings"
	"testing"
	"time"

	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	srpc "github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

// applyBucketConfigTestStream implements the apply bucket config stream.
type applyBucketConfigTestStream struct {
	ctx context.Context
}

func (s *applyBucketConfigTestStream) Context() context.Context { return s.ctx }

func (s *applyBucketConfigTestStream) MsgSend(msg srpc.Message) error { return nil }

func (s *applyBucketConfigTestStream) MsgRecv(msg srpc.Message) error { return nil }

func (s *applyBucketConfigTestStream) CloseSend() error { return nil }

func (s *applyBucketConfigTestStream) Close() error { return nil }

func (s *applyBucketConfigTestStream) Send(*ApplyBucketConfigResponse) error { return nil }

func (s *applyBucketConfigTestStream) SendAndClose(*ApplyBucketConfigResponse) error { return nil }

// TestApplyBucketConfigResolverError checks that a resolver error surfaces to
// the RPC caller instead of being discarded.
func TestApplyBucketConfigResolverError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	t.Cleanup(cancel)

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	resolverErr := errors.New("apply bucket config resolver failed")
	relHandler, err := b.AddHandler(directive.NewFuncHandler(
		func(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
			if _, ok := di.GetDirective().(bucket.ApplyBucketConfig); !ok {
				return nil, nil
			}
			return directive.Resolvers(directive.NewFuncResolver(
				func(ctx context.Context, handler directive.ResolverHandler) error {
					return resolverErr
				},
			)), nil
		},
	))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relHandler)

	api, err := NewAPI(b, &Config{})
	if err != nil {
		t.Fatal(err.Error())
	}

	err = api.ApplyBucketConfig(&ApplyBucketConfigRequest{
		Config:       &bucket.Config{Id: "test-bucket", Rev: 1},
		VolumeIdList: []string{"test-volume"},
	}, &applyBucketConfigTestStream{ctx: ctx})
	if err == nil {
		t.Fatal("expected resolver error, got nil")
	}
	if !strings.Contains(err.Error(), resolverErr.Error()) {
		t.Fatalf("expected resolver error, got: %v", err.Error())
	}
}

// _ is a type assertion
var _ SRPCHydraDaemonService_ApplyBucketConfigStream = ((*applyBucketConfigTestStream)(nil))
