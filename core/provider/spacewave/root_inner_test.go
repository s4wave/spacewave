package provider_spacewave

import (
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
)

func decodePostedRootInner(
	t *testing.T,
	soID string,
	localPriv crypto.PrivKey,
	localPeerID string,
	epoch *sobject.SOKeyEpoch,
	root *sobject.SORoot,
) *sobject.SORootInner {
	t.Helper()

	grant := findSOGrantByPeerID(epoch.GetGrants(), localPeerID)
	if grant == nil {
		t.Fatal("expected local grant")
	}
	grantInner, err := grant.DecryptInnerData(localPriv, soID)
	if err != nil {
		t.Fatalf("decrypt grant inner: %v", err)
	}
	xfrm, err := block_transform.NewTransformer(
		controller.ConstructOpts{Logger: logrus.New().WithField("test", t.Name())},
		buildStandaloneSpaceInitStepFactorySet(),
		grantInner.GetTransformConf(),
	)
	if err != nil {
		t.Fatalf("build transformer: %v", err)
	}
	innerData, err := xfrm.DecodeBlock(root.GetInner())
	if err != nil {
		t.Fatalf("decode root inner: %v", err)
	}
	inner := &sobject.SORootInner{}
	if err := inner.UnmarshalVT(innerData); err != nil {
		t.Fatalf("unmarshal root inner: %v", err)
	}
	return inner
}
