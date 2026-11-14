package provider_spacewave

import (
	"testing"

	"github.com/sirupsen/logrus"
)

// NewTestProviderAccount creates a minimal ProviderAccount for transfer testing.
// Only the session client and provider config are populated.
func NewTestProviderAccount(t *testing.T, endpoint string) *ProviderAccount {
	t.Helper()
	priv, _ := generateTestKeypair(t)
	acc, err := NewSessionAccount(logrus.New().WithField("test", t.Name()), endpoint, "test-account", priv)
	if err != nil {
		t.Fatal(err)
	}
	return acc
}
