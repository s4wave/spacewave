package s4wave_sshhost

import "testing"

func TestSshHostValidatePinsEndpointAndCredentialRefs(t *testing.T) {
	host := &SshHost{
		Label: "Prod Host",
		Endpoint: &SshHostEndpoint{
			Host:     "prod.example.com",
			Port:     22,
			Username: "deploy",
		},
		Credentials: &SshHostCredentialRefs{
			PrivateKeySecretObjectKey: "secrets/ssh/prod-key",
			PassphraseSecretObjectKey: "secrets/ssh/prod-passphrase",
		},
		HostKeyPins: []*SshHostKeyPin{{
			Algorithm:         "ssh-ed25519",
			Sha256Fingerprint: "SHA256:example",
		}},
	}
	if err := host.Validate(); err != nil {
		t.Fatalf("valid ssh host failed validation: %v", err)
	}

	host.Endpoint.Host = ""
	if err := host.Validate(); err == nil {
		t.Fatal("expected missing endpoint host to fail validation")
	}

	host.Endpoint.Host = "prod.example.com"
	host.Credentials.PrivateKeySecretObjectKey = "-----BEGIN PRIVATE KEY-----\n"
	if err := host.Validate(); err == nil {
		t.Fatal("expected raw credential-looking ref to fail validation")
	}
}

func TestNormalizeSshHostEndpointDefaultsPort(t *testing.T) {
	endpoint := NormalizeSshHostEndpoint(&SshHostEndpoint{
		Host:     " prod.example.com ",
		Username: " deploy ",
	})
	if endpoint.GetHost() != "prod.example.com" {
		t.Fatalf("host = %q", endpoint.GetHost())
	}
	if endpoint.GetUsername() != "deploy" {
		t.Fatalf("username = %q", endpoint.GetUsername())
	}
	if endpoint.GetPort() != DefaultSshPort {
		t.Fatalf("port = %d, want %d", endpoint.GetPort(), DefaultSshPort)
	}
}
