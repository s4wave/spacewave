//go:build !tinygo

package block_store_gdrive

import (
	"context"
	"errors"
	"net/http"
	"os"

	"golang.org/x/oauth2"
)

// OAuthClientID is set at build time or by SPACEWAVE_GDRIVE_CLIENT_ID at runtime.
// It stays empty until the operator configures their own Google OAuth client.
var OAuthClientID string

// OAuthClientSecret is the installed application's client setting. It stays
// empty until the operator supplies it; an installed app cannot keep it secret.
var OAuthClientSecret string

// ErrClientUnconfigured asks the operator to configure the installed app.
var ErrClientUnconfigured = errors.New("the Google Drive OAuth client is not configured; set SPACEWAVE_GDRIVE_CLIENT_ID and SPACEWAVE_GDRIVE_CLIENT_SECRET")

// ErrAccessDenied names a device sign-in the account owner declined.
var ErrAccessDenied = errors.New("the Google Drive sign-in was declined")

// ErrCodeExpired names a device code that needs a fresh sign-in attempt.
var ErrCodeExpired = errors.New("the Google Drive sign-in code expired; run storage add gdrive again")

// OAuthConfig returns the installed app's runtime or build settings. It never
// reads a credential Secret or starts authorization on its own.
func OAuthConfig() (*oauth2.Config, error) {
	// Runtime settings let the operator configure a deployed binary themselves.
	id, secret := os.Getenv("SPACEWAVE_GDRIVE_CLIENT_ID"), os.Getenv("SPACEWAVE_GDRIVE_CLIENT_SECRET")
	if id == "" {
		id = OAuthClientID
	}
	if secret == "" {
		secret = OAuthClientSecret
	}
	if id == "" || secret == "" {
		return nil, ErrClientUnconfigured
	}
	return &oauth2.Config{ClientID: id, ClientSecret: secret,
		Scopes: []string{"https://www.googleapis.com/auth/drive.file"},
		Endpoint: oauth2.Endpoint{ //nolint:gosec // These are Google's public OAuth endpoint URLs.
			DeviceAuthURL: "https://oauth2.googleapis.com/device/code",
			TokenURL:      "https://oauth2.googleapis.com/token", AuthStyle: oauth2.AuthStyleInParams,
		}}, nil
}

// Authorize runs the device flow, exposing only the verification URL and user
// code to display. The returned refresh token goes directly into a Secret.
// oauth2 honors Google's interval and increases it by five seconds on slow_down.
func Authorize(ctx context.Context, conf *oauth2.Config, show func(string, string) error) (*Credentials, error) {
	// Obtain the short-lived user code without exposing the device code.
	if conf == nil || conf.ClientID == "" || conf.ClientSecret == "" {
		return nil, ErrClientUnconfigured
	}
	device, err := conf.DeviceAuth(ctx)
	if err != nil {
		return nil, oauthError(err)
	}
	if err := show(device.VerificationURI, device.UserCode); err != nil {
		return nil, err
	}

	// Wait for the owner; return a named outcome without provider response bodies.
	token, err := conf.DeviceAccessToken(ctx, device)
	if err != nil {
		return nil, oauthError(err)
	}
	if token.RefreshToken == "" {
		return nil, errors.New("no refresh token from Google Drive; authorize offline access again")
	}
	return &Credentials{RefreshToken: token.RefreshToken}, nil
}

// oauthError strips OAuth response bodies before an error reaches logs or CLI.
func oauthError(err error) error {
	// Preserve cancellation and the device flow's named user outcomes.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if e, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		switch e.ErrorCode {
		case "access_denied":
			return ErrAccessDenied
		case "expired_token":
			return ErrCodeExpired
		case "invalid_grant", "invalid_token":
			return ErrSignIn
		}
	}
	return errors.New("authorization with Google Drive failed")
}

// AuthorizedHTTP renews access in memory; it writes no token file and logs none.
func AuthorizedHTTP(ctx context.Context, conf *oauth2.Config, credentials *Credentials) (*http.Client, error) {
	// Require the credential before building a lazily authenticating transport.
	if conf == nil || conf.ClientID == "" || conf.ClientSecret == "" {
		return nil, ErrClientUnconfigured
	}
	if credentials.GetRefreshToken() == "" {
		return nil, ErrSignIn
	}
	source := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: credentials.GetRefreshToken()})
	return oauth2.NewClient(ctx, &refreshSource{source: source}), nil
}

// CreateRootFolder adds one backup folder under the client's configured root.
func (c *Client) CreateRootFolder(ctx context.Context, name string) (File, error) {
	return c.createFolder(ctx, c.root, name)
}
