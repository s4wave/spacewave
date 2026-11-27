//go:build !tinygo

package block_store_gdrive_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	block_store_gdrive "github.com/s4wave/spacewave/db/block/store/gdrive"
	"golang.org/x/oauth2"
)

// TestDeviceFlowKeepsTokensPrivate exercises pending and slow-down against an
// offline OAuth server, and proves neither refresh tokens nor error bodies escape.
func TestDeviceFlowKeepsTokensPrivate(t *testing.T) {
	// The fake grants one code after first requesting a slower poll interval.
	var mtx sync.Mutex
	var polls []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/device" {
			_, _ = io.WriteString(w, `{"device_code":"synthetic-device-code","user_code":"ABCD-EFGH","verification_uri":"https://example.test/authorize","expires_in":60,"interval":1}`)
			return
		}
		mtx.Lock()
		defer mtx.Unlock()
		polls = append(polls, time.Now())
		if len(polls) == 1 {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"error":"slow_down","error_description":"synthetic-refresh-must-not-print"}`)
			return
		}
		_, _ = io.WriteString(w, `{"access_token":"synthetic-access-must-not-print","refresh_token":"synthetic-refresh-must-not-print","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	conf := &oauth2.Config{ClientID: "synthetic-client", ClientSecret: "synthetic-client-setting", Scopes: []string{"https://www.googleapis.com/auth/drive.file"},
		Endpoint: oauth2.Endpoint{DeviceAuthURL: server.URL + "/device", TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	var output bytes.Buffer
	credentials, err := block_store_gdrive.Authorize(t.Context(), conf, func(uri, code string) error {
		_, err := output.WriteString(uri + " " + code)
		return err
	})
	if err != nil || credentials.GetRefreshToken() != "synthetic-refresh-must-not-print" {
		t.Fatalf("device authorization did not complete: %v", err)
	}

	// Google's slow_down adds five seconds to the original returned interval.
	mtx.Lock()
	defer mtx.Unlock()
	if len(polls) != 2 || polls[1].Sub(polls[0]) < 6*time.Second {
		t.Fatalf("polls ignored slow_down: %v", polls)
	}
	if strings.Contains(output.String(), "must-not-print") || strings.Contains(output.String(), "device-code") || !strings.Contains(output.String(), "ABCD-EFGH") {
		t.Fatalf("authorization output contained private material: %q", output.String())
	}
}

// TestDeviceFlowNamedOutcomes checks decline, expiry and revoked authorization
// without exposing the OAuth server's raw response or token canaries.
func TestDeviceFlowNamedOutcomes(t *testing.T) {
	for _, tc := range []struct {
		// code is the OAuth outcome the fake returns.
		code string
		// want is the safe named error the user sees.
		want error
	}{{"access_denied", block_store_gdrive.ErrAccessDenied}, {"expired_token", block_store_gdrive.ErrCodeExpired}, {"invalid_grant", block_store_gdrive.ErrSignIn}} {
		t.Run(tc.code, func(t *testing.T) {
			// Return a synthetic token canary only inside the provider's error body.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/device" {
					_, _ = io.WriteString(w, `{"device_code":"fake","user_code":"PUBLIC-CODE","verification_uri":"https://example.test/authorize","expires_in":30,"interval":1}`)
					return
				}
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"`+tc.code+`","error_description":"synthetic-refresh-must-not-print"}`)
			}))
			defer server.Close()
			conf := &oauth2.Config{ClientID: "fake", ClientSecret: "fake",
				Endpoint: oauth2.Endpoint{DeviceAuthURL: server.URL + "/device", TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
			_, err := block_store_gdrive.Authorize(t.Context(), conf, func(string, string) error { return nil })
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "must-not-print") {
				t.Fatalf("unsafe or wrong authorization outcome: %v", err)
			}
		})
	}
}

// TestAuthorizedHTTPRefreshAndRevocation exercises the real OAuth transport on
// a metadata GET, then maps a revoked refresh token to a safe sign-in outcome.
func TestAuthorizedHTTPRefreshAndRevocation(t *testing.T) {
	// Keep every token exchange and authenticated request on the fake listener.
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			if revoked.Load() {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"synthetic-refresh-must-not-print"}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"synthetic-access","token_type":"Bearer","expires_in":3600}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer synthetic-access" {
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, `{"files":[]}`)
	}))
	defer server.Close()
	conf := &oauth2.Config{ClientID: "fake", ClientSecret: "fake", Endpoint: oauth2.Endpoint{TokenURL: server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	credentials := &block_store_gdrive.Credentials{RefreshToken: "synthetic-refresh"}
	httpClient, err := block_store_gdrive.AuthorizedHTTP(t.Context(), conf, credentials)
	if err != nil {
		t.Fatal(err)
	}
	client, err := block_store_gdrive.NewClient(httpClient, "root", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ListObjects(t.Context(), "", "", func(string, int64) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// A fresh transport cannot hide a revoked token behind a cached access token.
	revoked.Store(true)
	httpClient, err = block_store_gdrive.AuthorizedHTTP(t.Context(), conf, credentials)
	if err != nil {
		t.Fatal(err)
	}
	client, err = block_store_gdrive.NewClient(httpClient, "root", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ListObjects(t.Context(), "", "", func(string, int64) error { return nil }); !errors.Is(err, block_store_gdrive.ErrSignIn) || strings.Contains(err.Error(), "must-not-print") {
		t.Fatalf("revocation outcome: %v", err)
	}
}
