package provider_spacewave_handoff

import "testing"

func TestBuildHandoffURLs(t *testing.T) {
	apiEndpoint := "https://api.spacewave.test/"
	publicBaseURL := "https://spacewave.test/"
	payload := "payload-123"
	wsTicket := "ticket-123"

	createURL := buildCreateSessionURL(apiEndpoint)
	if createURL != "https://api.spacewave.test/api/auth/session/create" {
		t.Fatalf("unexpected create URL: %q", createURL)
	}

	authURL := buildHandoffBrowserURL(publicBaseURL, payload, "signup", "spacewave")
	if authURL != "https://spacewave.test/#/auth/link/payload-123?intent=signup&username=spacewave" {
		t.Fatalf("unexpected auth URL: %q", authURL)
	}

	wsURL := buildHandoffWSURL(apiEndpoint, wsTicket)
	if wsURL != "wss://api.spacewave.test/api/auth/session/ws?tk=ticket-123" {
		t.Fatalf("unexpected ws URL: %q", wsURL)
	}
}

func TestValidateOpenURL(t *testing.T) {
	hosts := []string{"accounts.google.com", "spacewave.test"}
	ok := []string{
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=x",
		"https://spacewave.test/#/auth/link/payload",
		"https://SPACEWAVE.TEST/whatever",
	}
	for _, u := range ok {
		if err := validateOpenURL(u, hosts); err != nil {
			t.Errorf("expected %q to validate, got %v", u, err)
		}
	}

	bad := []string{
		"",
		"http://spacewave.test/insecure",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ms-help://hostile",
		"https://evil.example.com/steal",
		"https:///no-host",
		`https://spacewave.test" && calc.exe && "`,
	}
	for _, u := range bad {
		if err := validateOpenURL(u, hosts); err == nil {
			t.Errorf("expected %q to be rejected", u)
		}
	}
}

func TestHostsFromURLs(t *testing.T) {
	got := hostsFromURLs("https://a.example/", "", "not a url", "https://b.example:8443/x")
	want := []string{"a.example", "b.example:8443"}
	if len(got) != len(want) {
		t.Fatalf("unexpected hosts: %v", got)
	}
	for i, h := range want {
		if got[i] != h {
			t.Errorf("host %d: got %q want %q", i, got[i], h)
		}
	}
}
