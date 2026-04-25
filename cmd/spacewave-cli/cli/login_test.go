//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/cli"
	s4wave_provider "github.com/s4wave/spacewave/core/provider"
	session_pb "github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestRunLoginBrowserWithStreams(t *testing.T) {
	oldResolveStatePath := loginResolveStatePath
	oldConnectDaemon := loginConnectDaemon
	oldCloseClient := loginCloseClient
	oldLoginWithEntityKey := loginWithEntityKey
	oldBrowserHandoff := loginBrowserHandoff
	t.Cleanup(func() {
		loginResolveStatePath = oldResolveStatePath
		loginConnectDaemon = oldConnectDaemon
		loginCloseClient = oldCloseClient
		loginWithEntityKey = oldLoginWithEntityKey
		loginBrowserHandoff = oldBrowserHandoff
	})

	loginResolveStatePath = func(_ *cli.Context, statePath string) (string, error) {
		if statePath != ".spacewave" {
			t.Fatalf("unexpected state path: %s", statePath)
		}
		return "/tmp/state", nil
	}
	loginConnectDaemon = func(ctx context.Context, statePath string) (*sdkClient, error) {
		if statePath != "/tmp/state" {
			t.Fatalf("unexpected resolved state path: %s", statePath)
		}
		return &sdkClient{}, nil
	}
	loginCloseClient = func(*sdkClient) {}
	loginBrowserHandoff = func(
		ctx context.Context,
		client *sdkClient,
		providerID string,
		req *s4wave_provider_spacewave.StartBrowserHandoffRequest,
	) (*session_pb.SessionListEntry, error) {
		if providerID != "spacewave" {
			t.Fatalf("unexpected provider id: %s", providerID)
		}
		if req.GetClientType() != "cli" {
			t.Fatalf("unexpected client type: %s", req.GetClientType())
		}
		if req.GetAuthIntent() != "login" {
			t.Fatalf("unexpected auth intent: %s", req.GetAuthIntent())
		}
		if req.GetUsername() != "" {
			t.Fatalf("unexpected username: %s", req.GetUsername())
		}
		return &session_pb.SessionListEntry{
			SessionRef: &session_pb.SessionRef{
				ProviderResourceRef: &s4wave_provider.ProviderResourceRef{
					ProviderId:        "spacewave",
					ProviderAccountId: "acct-123",
					Id:                "sess-456",
				},
			},
		}, nil
	}

	cmd := newLoginBrowserCommand()
	set := flagSet(t)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if err := set.Parse([]string{"--provider-id", "spacewave"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	c := cli.NewContext(nil, set, nil)
	c.Command = cmd
	c.Context = context.Background()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runLoginBrowserWithStreams(c, ".spacewave", "text", &stdout, &stderr); err != nil {
		t.Fatalf("run login browser: %v", err)
	}

	if got := stderr.String(); got != "Opening browser for Spacewave CLI sign-in...\n" {
		t.Fatalf("unexpected stderr: %q", got)
	}

	out := stdout.String()
	assertContains(t, out, "Signed in via browser.")
	assertContains(t, out, "spacewave")
	assertContains(t, out, "acct-123")
	assertContains(t, out, "sess-456")
}

func TestRunBrowserSignupWithStreams(t *testing.T) {
	oldResolveStatePath := loginResolveStatePath
	oldConnectDaemon := loginConnectDaemon
	oldCloseClient := loginCloseClient
	oldBrowserHandoff := loginBrowserHandoff
	t.Cleanup(func() {
		loginResolveStatePath = oldResolveStatePath
		loginConnectDaemon = oldConnectDaemon
		loginCloseClient = oldCloseClient
		loginBrowserHandoff = oldBrowserHandoff
	})

	loginResolveStatePath = func(_ *cli.Context, statePath string) (string, error) {
		if statePath != ".spacewave" {
			t.Fatalf("unexpected state path: %s", statePath)
		}
		return "/tmp/state", nil
	}
	loginConnectDaemon = func(ctx context.Context, statePath string) (*sdkClient, error) {
		if statePath != "/tmp/state" {
			t.Fatalf("unexpected resolved state path: %s", statePath)
		}
		return &sdkClient{}, nil
	}
	loginCloseClient = func(*sdkClient) {}
	loginBrowserHandoff = func(
		ctx context.Context,
		client *sdkClient,
		providerID string,
		req *s4wave_provider_spacewave.StartBrowserHandoffRequest,
	) (*session_pb.SessionListEntry, error) {
		if providerID != "spacewave" {
			t.Fatalf("unexpected provider id: %s", providerID)
		}
		if req.GetClientType() != "cli" {
			t.Fatalf("unexpected client type: %s", req.GetClientType())
		}
		if req.GetAuthIntent() != "signup" {
			t.Fatalf("unexpected auth intent: %s", req.GetAuthIntent())
		}
		if req.GetUsername() != "spacewave" {
			t.Fatalf("unexpected username: %s", req.GetUsername())
		}
		return &session_pb.SessionListEntry{
			SessionRef: &session_pb.SessionRef{
				ProviderResourceRef: &s4wave_provider.ProviderResourceRef{
					ProviderId:        "spacewave",
					ProviderAccountId: "acct-signup",
					Id:                "sess-signup",
				},
			},
		}, nil
	}

	set := flagSet(t)
	cmd := newAccountCreateSpacewaveCommand()
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if err := set.Parse([]string{"--provider-id", "spacewave", "--username", "spacewave"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	c := cli.NewContext(nil, set, nil)
	c.Command = cmd
	c.Context = context.Background()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runBrowserSignupWithStreams(c, ".spacewave", "text", &stdout, &stderr); err != nil {
		t.Fatalf("run browser signup: %v", err)
	}

	if got := stderr.String(); got != "Opening browser for Spacewave CLI sign-up...\n" {
		t.Fatalf("unexpected stderr: %q", got)
	}

	out := stdout.String()
	assertContains(t, out, "Browser sign-up complete.")
	assertContains(t, out, "acct-signup")
	assertContains(t, out, "sess-signup")
}

func TestRunLoginWithEntityKey(t *testing.T) {
	oldResolveStatePath := loginResolveStatePath
	oldConnectDaemon := loginConnectDaemon
	oldCloseClient := loginCloseClient
	oldLoginWithEntityKey := loginWithEntityKey
	t.Cleanup(func() {
		loginResolveStatePath = oldResolveStatePath
		loginConnectDaemon = oldConnectDaemon
		loginCloseClient = oldCloseClient
		loginWithEntityKey = oldLoginWithEntityKey
	})

	loginResolveStatePath = func(_ *cli.Context, statePath string) (string, error) {
		if statePath != ".spacewave" {
			t.Fatalf("unexpected state path: %s", statePath)
		}
		return "/tmp/state", nil
	}
	loginConnectDaemon = func(ctx context.Context, statePath string) (*sdkClient, error) {
		if statePath != "/tmp/state" {
			t.Fatalf("unexpected resolved state path: %s", statePath)
		}
		return &sdkClient{}, nil
	}
	loginCloseClient = func(*sdkClient) {}
	loginWithEntityKey = func(
		ctx context.Context,
		client *sdkClient,
		providerID string,
		pemData []byte,
	) (*session_pb.SessionListEntry, error) {
		if providerID != "spacewave" {
			t.Fatalf("unexpected provider id: %s", providerID)
		}
		if string(pemData) != "pem-data" {
			t.Fatalf("unexpected pem data: %q", string(pemData))
		}
		return &session_pb.SessionListEntry{
			SessionRef: &session_pb.SessionRef{
				ProviderResourceRef: &s4wave_provider.ProviderResourceRef{
					ProviderId:        "spacewave",
					ProviderAccountId: "acct-pem",
					Id:                "sess-pem",
				},
			},
		}, nil
	}

	pemPath := filepath.Join(t.TempDir(), "backup.pem")
	if err := os.WriteFile(pemPath, []byte("pem-data"), 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}

	set := flagSet(t)
	cmd := newLoginCommand(nil)
	for _, fl := range cmd.Flags {
		if err := fl.Apply(set); err != nil {
			t.Fatalf("apply flag: %v", err)
		}
	}
	if err := set.Parse([]string{"--provider-id", "spacewave"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	c := cli.NewContext(nil, set, nil)
	c.Command = cmd
	c.Context = context.Background()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	runErr := runLogin(c, ".spacewave", "text", pemPath)
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	if runErr != nil {
		t.Fatalf("run login: %v", runErr)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	assertContains(t, string(out), "Logged in with PEM key.")
	assertContains(t, string(out), "acct-pem")
	assertContains(t, string(out), "sess-pem")
}

func flagSet(t *testing.T) *flag.FlagSet {
	t.Helper()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}
