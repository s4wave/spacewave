package provider_spacewave

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

// decodePostedCheckpointState decrypts the state data of a posted checkpoint
// with the read key epoch grants to the local peer.
func decodePostedCheckpointState(
	t *testing.T,
	soID string,
	localPriv crypto.PrivKey,
	localPeerID string,
	epoch *sobject.SOKeyEpoch,
	checkpoint *sobject.SOCheckpoint,
) []byte {
	// Decrypt the local peer's grant.
	t.Helper()
	grant := epoch.FindGrant(localPeerID)
	if grant == nil {
		t.Fatal("expected local grant")
	}
	grantInner, err := grant.DecryptInnerData(localPriv, soID)
	if err != nil {
		t.Fatalf("decrypt grant inner: %v", err)
	}

	// Build the transformer of its epoch.
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: logrus.New().WithField("test", t.Name())},
		buildStandaloneSpaceInitStepFactorySet(),
		grantInner.GetTransformConf(),
	)
	if err != nil {
		t.Fatalf("build transformer: %v", err)
	}

	// Decode the checkpoint state, if any.
	inner, err := checkpoint.UnmarshalInner()
	if err != nil {
		t.Fatalf("unmarshal checkpoint inner: %v", err)
	}
	if len(inner.GetStateData()) == 0 {
		return nil
	}
	data, err := xfrm.DecodeBlock(inner.GetStateData())
	if err != nil {
		t.Fatalf("decode checkpoint state: %v", err)
	}
	return data
}
