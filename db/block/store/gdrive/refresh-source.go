//go:build !tinygo

package block_store_gdrive

import "golang.org/x/oauth2"

// refreshSource translates a revoked credential into a sign-in outcome, without
// returning OAuth response bodies that might contain tokens to callers.
type refreshSource struct {
	// source renews access in memory from the encrypted Secret's refresh token.
	source oauth2.TokenSource
}

// Token renews access or asks the owner to sign in again.
func (s *refreshSource) Token() (*oauth2.Token, error) {
	token, err := s.source.Token()
	if err != nil {
		return nil, oauthError(err)
	}
	return token, nil
}

// _ is a type assertion.
var _ oauth2.TokenSource = (*refreshSource)(nil)
