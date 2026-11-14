package provider_spacewave

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/refcount"
	provider_gccleanup "github.com/s4wave/spacewave/core/provider/gccleanup"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewSessionAccount returns the account accountID reached through the cloud
// at endpoint with the session key priv. It holds no volume and no bus, so
// it serves the account's shared object API, such as a transfer target, and
// not mounted Spaces.
func NewSessionAccount(le *logrus.Entry, endpoint, accountID string, priv crypto.PrivKey) (*ProviderAccount, error) {
	// Build the session client and the provider.
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	cli := NewSessionClient(http.DefaultClient, endpoint, DefaultSigningEnvPrefix, priv, pid.String())
	prov := NewProvider(le, nil, &Config{Endpoint: endpoint}, NewProviderInfo("spacewave"), nil, nil)

	// Build the account with its shared object trackers and cleanup runner.
	acc := &ProviderAccount{
		le:        le,
		accountID: accountID,
		p:         prov,
		conf:      &Config{Endpoint: endpoint},
		sfs:       prov.sfs,
		soListCtr: ccontainer.NewCContainer[*sobject.SharedObjectList](nil),
	}
	acc.sessionClient = acc.configureSessionClient(cli)
	acc.sobjects = keyed.NewKeyedRefCountWithLogger(acc.buildSharedObjectTracker, le)
	acc.gcCleanupRunner = provider_gccleanup.NewRunner(
		le.WithField("component", "gc-cleanup-runner"),
		"GC swept nodes after provider account cleanup",
		acc.collectGCRootlessBlocks,
	)
	acc.soListRc = refcount.NewRefCount(nil, true, nil, nil, acc.resolveSharedObjectList)
	_ = acc.soListRc.SetContext(context.Background())
	return acc, nil
}
