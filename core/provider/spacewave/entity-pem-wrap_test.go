package provider_spacewave

import (
	"bytes"
	"encoding/base64"
	"testing"

	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// TestRecoverPasskeyEntityPem verifies each passkey wrapping the app writes
// recovers the entity PEM.
func TestRecoverPasskeyEntityPem(t *testing.T) {
	// Fix the key, PRF output and PIN shared by every case.
	pem := []byte("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----\n")
	prfOutput := bytes.Repeat([]byte{7}, PasskeyPrfOutputSize)
	const pin = "4321"

	// sealPrf wraps plaintext with the PRF output as the browser stores it.
	sealPrf := func(plaintext []byte, pinWrapped bool) *s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse {
		// Seal and encode the blob and its parameters.
		ciphertext, params, err := WrapWithPasskeyPrf(plaintext, prfOutput, pinWrapped)
		if err != nil {
			t.Fatal(err)
		}
		paramsData, err := params.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		return &s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse{
			EncryptedBlob: base64.StdEncoding.EncodeToString(ciphertext),
			PrfCapable:    true,
			AuthParams:    base64.StdEncoding.EncodeToString(paramsData),
			PrfOutput:     base64.RawStdEncoding.EncodeToString(prfOutput),
		}
	}

	// Wrap the PEM with the PIN for the PIN cases.
	pinWrapped, err := WrapPemWithPin(pem, pin)
	if err != nil {
		t.Fatal(err)
	}
	pinWrappedB64 := base64.StdEncoding.EncodeToString(pinWrapped)

	// Recover each wrapping the app stores.
	for _, tc := range []struct {
		name    string
		reauth  *s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse
		wantPin bool
	}{
		{name: "plain", reauth: &s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse{
			EncryptedBlob: base64.StdEncoding.EncodeToString(pem),
		}},
		{name: "pin", wantPin: true, reauth: &s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse{
			EncryptedBlob: pinWrappedB64,
			PinWrapped:    true,
		}},
		{name: "prf", reauth: sealPrf(pem, false)},
		{name: "prf and pin", wantPin: true, reauth: sealPrf([]byte(pinWrappedB64), true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Recover the PEM and record whether the PIN was requested.
			var askedPin bool
			got, err := RecoverPasskeyEntityPem(tc.reauth, func() (string, error) {
				askedPin = true
				return pin, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, pem) {
				t.Fatalf("recovered %q, want %q", got, pem)
			}
			if askedPin != tc.wantPin {
				t.Fatalf("asked for PIN = %v, want %v", askedPin, tc.wantPin)
			}
		})
	}
}
