//go:build !goscript

package provider_local

import (
	"bytes"
	"context"
	"testing"
	"testing/synctest"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
)

// TestWaitOperationFencesPublishedSnapshot checks both success paths against a
// host state that has advanced before the snapshot exposed to body readers.
func TestWaitOperationFencesPublishedSnapshot(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		name := "host acceptance"
		if persisted {
			name = "persisted result"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Build an accepted root with a real decryptable participant grant.
				ctx := t.Context()
				host, _ := newTestLocalSOHost(t)
				host.sfs = block_transform.NewStepFactorySet()
				host.sfs.AddStepFactory(transform_gzip.NewStepFactory())
				transformConfig := &block_transform.Config{
					Steps: []*block_transform.StepConfig{{Id: transform_gzip.ConfigID}},
				}
				grant, err := sobject.EncryptSOGrant(
					host.privKey,
					host.privKey.GetPublic(),
					testSharedObjectID,
					&sobject.SOGrantInner{TransformConf: transformConfig},
				)
				if err != nil {
					t.Fatal(err)
				}
				transformer, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: host.le}, host.sfs, transformConfig)
				if err != nil {
					t.Fatal(err)
				}

				// Encode the accepted body through the same transform used by readers.
				want := []byte("accepted operation")
				inner, err := (&sobject.SORootInner{Seqno: 2, StateData: want}).MarshalVT()
				if err != nil {
					t.Fatal(err)
				}
				inner, err = transformer.EncodeBlock(inner)
				if err != nil {
					t.Fatal(err)
				}
				accepted := &sobject.SOState{
					Root:       &sobject.SORoot{InnerSeqno: 2, Inner: inner},
					RootGrants: []*sobject.SOGrant{grant},
				}

				// Expose acceptance from the host while body readers still see blank state.
				hostState := ccontainer.NewCContainer(accepted)
				host.soHost = sobject.NewSOHost(ctx, func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
					return hostState, func() {}, nil
				}, nil, testSharedObjectID)
				t.Cleanup(host.soHost.ClearContext)
				host.stateSnapCtr = ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](
					newLsoStateSnapshot(sobject.NewSOStateParticipantHandle(
						host.le, host.sfs, testSharedObjectID, &sobject.SOState{}, host.privKey, host.peerID,
					), &LocalSOState{}),
				)

				// A persisted result and an observed queue removal must provide the same fence.
				localID := sobject.NewSOOperationLocalID()
				if persisted {
					if err := host.writeLocalOpResult(ctx, &LocalSOOperationResult{
						LocalId:   localID,
						RootSeqno: 2,
						Result:    sobject.BuildSOOperationResult(host.peerID.String(), 1, true, nil),
					}); err != nil {
						t.Fatal(err)
					}
				}

				// Run completion to a stable blocked point without depending on elapsed time.
				var seqno uint64
				var rejected bool
				var waitErr error
				done := make(chan struct{})
				go func() {
					seqno, rejected, waitErr = host.WaitOperation(ctx, localID)
					close(done)
				}()
				synctest.Wait()
				select {
				case <-done:
					t.Fatalf("operation completed before its snapshot was published: seqno=%d rejected=%v err=%v", seqno, rejected, waitErr)
				default:
				}

				// Publishing the accepted snapshot releases completion and immediate readers.
				host.stateSnapCtr.SetValue(newLsoStateSnapshot(sobject.NewSOStateParticipantHandle(
					host.le, host.sfs, testSharedObjectID, accepted, host.privKey, host.peerID,
				), &LocalSOState{}))
				<-done
				if waitErr != nil {
					t.Fatal(waitErr)
				}
				if rejected {
					t.Fatal("accepted operation was reported as rejected")
				}
				if seqno != 2 {
					t.Fatalf("operation completed at seqno %d, want 2", seqno)
				}

				// Read through the public SharedObject API immediately after completion.
				shared := &SharedObject{lsoHost: host}
				snapshot, err := shared.GetSharedObjectState(ctx)
				if err != nil {
					t.Fatal(err)
				}
				root, err := snapshot.GetRootInner(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(root.GetStateData(), want) {
					t.Fatalf("published body = %q, want %q", root.GetStateData(), want)
				}
			})
		})
	}
}
