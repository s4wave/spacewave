package provider_spacewave

import (
	"context"
	"net/http"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// NewTestProviderAccount creates a minimal ProviderAccount for transfer testing.
// Only the session client and provider config are populated.
func NewTestProviderAccount(t *testing.T, endpoint string) *ProviderAccount {
	t.Helper()
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, endpoint, DefaultSigningEnvPrefix, priv, pid.String())
	le := logrus.New().WithField("test", t.Name())
	prov := NewProvider(
		le,
		nil,
		&Config{Endpoint: endpoint},
		NewProviderInfo("spacewave"),
		nil,
		nil,
	)
	acc := &ProviderAccount{
		le:            le,
		sessionClient: cli,
		accountID:     "test-account",
		p:             prov,
		conf:          &Config{Endpoint: endpoint},
		sfs:           prov.sfs,
		soListCtr:     ccontainer.NewCContainer[*sobject.SharedObjectList](nil),
	}
	acc.soListRc = refcount.NewRefCount(nil, true, nil, nil, acc.resolveSharedObjectList)
	acc.managedBAsRc = refcount.NewRefCountWithOptions(
		context.Background(),
		true,
		nil,
		nil,
		acc.resolveManagedBAs,
		snapshotRefCountOptions,
	)
	_ = acc.soListRc.SetContext(context.Background())
	acc.setWriteTicketOwnersContext(context.Background())
	return acc
}
