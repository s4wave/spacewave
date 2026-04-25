package provider_spacewave_api

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// jsonHasField checks whether the JSON data contains a field with the given name.
func jsonHasField(data []byte, field string) bool {
	// Look for "fieldName": pattern in JSON.
	needle := []byte(`"` + field + `"`)
	return bytes.Contains(data, needle)
}

// jsonFieldStringValue extracts a string value for a given JSON field.
// Returns empty string if not found. Only works for simple string values.
func jsonFieldStringValue(data []byte, field string) string {
	needle := []byte(`"` + field + `":"`)
	idx := bytes.Index(data, needle)
	if idx < 0 {
		return ""
	}
	start := idx + len(needle)
	end := bytes.IndexByte(data[start:], '"')
	if end < 0 {
		return ""
	}
	return string(data[start : start+end])
}

// TestCheckoutRequest_ProtoJSON verifies proto-JSON roundtrip for CheckoutRequest.
func TestCheckoutRequest_ProtoJSON(t *testing.T) {
	msg := &CheckoutRequest{
		SuccessUrl:      "https://example.com/success",
		CancelUrl:       "https://example.com/cancel",
		BillingInterval: s4wave_provider_spacewave.BillingInterval_BillingInterval_MONTH,
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify camelCase field names.
	if !jsonHasField(data, "successUrl") {
		t.Fatalf("expected camelCase field 'successUrl' in JSON: %s", data)
	}
	if !jsonHasField(data, "cancelUrl") {
		t.Fatalf("expected camelCase field 'cancelUrl' in JSON: %s", data)
	}
	if !jsonHasField(data, "billingInterval") {
		t.Fatalf("expected camelCase field 'billingInterval' in JSON: %s", data)
	}

	// Verify snake_case is NOT in the output.
	if jsonHasField(data, "success_url") {
		t.Fatalf("unexpected snake_case field 'success_url' in JSON output: %s", data)
	}
	if jsonHasField(data, "cancel_url") {
		t.Fatalf("unexpected snake_case field 'cancel_url' in JSON output: %s", data)
	}

	// Verify field values.
	if v := jsonFieldStringValue(data, "successUrl"); v != "https://example.com/success" {
		t.Fatalf("successUrl value: got %q, want %q", v, "https://example.com/success")
	}

	// Roundtrip.
	got := &CheckoutRequest{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestCheckoutRequest_ZeroValues verifies proto3 zero-value omission.
func TestCheckoutRequest_ZeroValues(t *testing.T) {
	msg := &CheckoutRequest{}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Proto3 omits zero-value fields. Empty strings should not appear.
	if jsonHasField(data, "successUrl") {
		t.Fatalf("zero-value string field 'successUrl' should be omitted: %s", data)
	}
	if jsonHasField(data, "cancelUrl") {
		t.Fatalf("zero-value string field 'cancelUrl' should be omitted: %s", data)
	}
	if jsonHasField(data, "billingInterval") {
		t.Fatalf("zero-value string field 'billingInterval' should be omitted: %s", data)
	}

	// Empty message should be "{}".
	if string(data) != "{}" {
		t.Fatalf("empty CheckoutRequest should marshal to {}, got: %s", data)
	}
}

// TestBillingStateResponse_ProtoJSON verifies int64 fields serialize as strings.
func TestBillingStateResponse_ProtoJSON(t *testing.T) {
	msg := &BillingStateResponse{
		Status:           s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		BillingInterval:  s4wave_provider_spacewave.BillingInterval_BillingInterval_YEAR,
		PastDueSince:     1700000000000,
		CancelAt:         1710000000000,
		CurrentPeriodEnd: 1720000000000,
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify camelCase field names.
	if !jsonHasField(data, "pastDueSince") {
		t.Fatalf("expected field 'pastDueSince' in JSON: %s", data)
	}
	if !jsonHasField(data, "cancelAt") {
		t.Fatalf("expected field 'cancelAt' in JSON: %s", data)
	}
	if !jsonHasField(data, "currentPeriodEnd") {
		t.Fatalf("expected field 'currentPeriodEnd' in JSON: %s", data)
	}

	// Int64 fields must be serialized as quoted strings in proto-JSON.
	// The proto-JSON spec requires int64/uint64 as strings.
	if !bytes.Contains(data, []byte(`"pastDueSince":"1700000000000"`)) {
		t.Fatalf("int64 field pastDueSince should be a quoted string, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"cancelAt":"1710000000000"`)) {
		t.Fatalf("int64 field cancelAt should be a quoted string, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"currentPeriodEnd":"1720000000000"`)) {
		t.Fatalf("int64 field currentPeriodEnd should be a quoted string, got: %s", data)
	}

	// Roundtrip.
	got := &BillingStateResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestBillingStateResponse_ZeroInt64Omitted verifies zero int64 fields are omitted.
func TestBillingStateResponse_ZeroInt64Omitted(t *testing.T) {
	msg := &BillingStateResponse{
		Status:          s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		BillingInterval: s4wave_provider_spacewave.BillingInterval_BillingInterval_MONTH,
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Zero int64 fields should be omitted.
	if jsonHasField(data, "pastDueSince") {
		t.Fatalf("zero int64 field 'pastDueSince' should be omitted: %s", data)
	}
	if jsonHasField(data, "cancelAt") {
		t.Fatalf("zero int64 field 'cancelAt' should be omitted: %s", data)
	}
	if jsonHasField(data, "currentPeriodEnd") {
		t.Fatalf("zero int64 field 'currentPeriodEnd' should be omitted: %s", data)
	}
}

// TestBillingUsageResponse_ProtoJSON verifies double + int64 field serialization.
func TestBillingUsageResponse_ProtoJSON(t *testing.T) {
	msg := &BillingUsageResponse{
		StorageBytes: 1073741824.5,
		WriteOps:     42000,
		ReadOps:      100000,
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Double fields serialize as numbers (not quoted).
	if !bytes.Contains(data, []byte(`"storageBytes":1073741824.5`)) {
		t.Fatalf("double field storageBytes should be a bare number, got: %s", data)
	}

	// Int64 fields serialize as quoted strings.
	if !bytes.Contains(data, []byte(`"writeOps":"42000"`)) {
		t.Fatalf("int64 field writeOps should be a quoted string, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"readOps":"100000"`)) {
		t.Fatalf("int64 field readOps should be a quoted string, got: %s", data)
	}

	// Roundtrip.
	got := &BillingUsageResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestListOrgsResponse_ProtoJSON verifies repeated nested message serialization.
func TestListOrgsResponse_ProtoJSON(t *testing.T) {
	msg := &ListOrgsResponse{
		Organizations: []*OrgResponse{
			{Id: "org-1", DisplayName: "Org One", Role: "admin"},
			{Id: "org-2", DisplayName: "Org Two", Role: "member"},
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify the repeated field name.
	if !jsonHasField(data, "organizations") {
		t.Fatalf("expected field 'organizations' in JSON: %s", data)
	}

	// Verify nested object fields use camelCase.
	if !jsonHasField(data, "displayName") {
		t.Fatalf("expected nested camelCase field 'displayName' in JSON: %s", data)
	}

	// Verify both org IDs appear.
	if !bytes.Contains(data, []byte(`"org-1"`)) {
		t.Fatalf("expected org-1 in JSON: %s", data)
	}
	if !bytes.Contains(data, []byte(`"org-2"`)) {
		t.Fatalf("expected org-2 in JSON: %s", data)
	}

	// Roundtrip.
	got := &ListOrgsResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestListOrgsResponse_Empty verifies empty repeated field serialization.
func TestListOrgsResponse_Empty(t *testing.T) {
	msg := &ListOrgsResponse{}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Empty repeated fields should be omitted (nil slice).
	if string(data) != "{}" {
		t.Fatalf("empty ListOrgsResponse should marshal to {}, got: %s", data)
	}
}

// TestGetOrgResponse_ProtoJSON verifies nested repeated OrgMember serialization.
func TestGetOrgResponse_ProtoJSON(t *testing.T) {
	msg := &GetOrgResponse{
		Id:               "org-123",
		DisplayName:      "Test Org",
		BillingAccountId: "billing-456",
		Members: []*OrgMember{
			{
				Id:        "mem-1",
				SubjectId: "acct-aaa",
				RoleId:    "role-admin",
				CreatedAt: 1700000000000,
				EntityId:  "alice",
			},
			{
				Id:        "mem-2",
				SubjectId: "acct-bbb",
				RoleId:    "role-member",
				CreatedAt: 1700000001000,
				EntityId:  "bob",
			},
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify camelCase field names.
	if !jsonHasField(data, "billingAccountId") {
		t.Fatalf("expected camelCase field 'billingAccountId' in JSON: %s", data)
	}
	if !jsonHasField(data, "displayName") {
		t.Fatalf("expected camelCase field 'displayName' in JSON: %s", data)
	}

	// Verify nested member fields use camelCase.
	if !jsonHasField(data, "subjectId") {
		t.Fatalf("expected nested camelCase field 'subjectId' in JSON: %s", data)
	}
	if !jsonHasField(data, "roleId") {
		t.Fatalf("expected nested camelCase field 'roleId' in JSON: %s", data)
	}
	if !jsonHasField(data, "createdAt") {
		t.Fatalf("expected nested camelCase field 'createdAt' in JSON: %s", data)
	}
	if !jsonHasField(data, "entityId") {
		t.Fatalf("expected nested camelCase field 'entityId' in JSON: %s", data)
	}

	// Verify OrgMember.CreatedAt is serialized as a quoted string (int64).
	if !bytes.Contains(data, []byte(`"createdAt":"1700000000000"`)) {
		t.Fatalf("int64 OrgMember.CreatedAt should be a quoted string, got: %s", data)
	}

	// Roundtrip.
	got := &GetOrgResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestAccountInfoResponse_ProtoJSON verifies mixed string + uint32 + int64 fields.
func TestAccountInfoResponse_ProtoJSON(t *testing.T) {
	msg := &AccountInfoResponse{
		AccountId:          "acct-789",
		EntityId:           "alice",
		AuthThreshold:      2,
		KeypairCount:       3,
		Epoch:              5,
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
		CancelAt:           1710000000000,
		BillingAccountId:   "billing-xyz",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify camelCase field names.
	for _, field := range []string{
		"accountId", "entityId", "authThreshold", "keypairCount",
		"epoch", "subscriptionStatus", "cancelAt", "billingAccountId",
	} {
		if !jsonHasField(data, field) {
			t.Fatalf("expected camelCase field %q in JSON: %s", field, data)
		}
	}

	// uint32 fields serialize as bare numbers (not quoted).
	if !bytes.Contains(data, []byte(`"authThreshold":2`)) {
		t.Fatalf("uint32 field authThreshold should be a bare number, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"keypairCount":3`)) {
		t.Fatalf("uint32 field keypairCount should be a bare number, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"epoch":5`)) {
		t.Fatalf("uint32 field epoch should be a bare number, got: %s", data)
	}

	// int64 field serializes as quoted string.
	if !bytes.Contains(data, []byte(`"cancelAt":"1710000000000"`)) {
		t.Fatalf("int64 field cancelAt should be a quoted string, got: %s", data)
	}

	// Roundtrip.
	got := &AccountInfoResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestAccountInfoResponse_ZeroUint32Omitted verifies zero uint32 fields are omitted.
func TestAccountInfoResponse_ZeroUint32Omitted(t *testing.T) {
	msg := &AccountInfoResponse{
		AccountId: "acct-789",
		EntityId:  "alice",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Zero uint32 fields should be omitted.
	if jsonHasField(data, "authThreshold") {
		t.Fatalf("zero uint32 field 'authThreshold' should be omitted: %s", data)
	}
	if jsonHasField(data, "keypairCount") {
		t.Fatalf("zero uint32 field 'keypairCount' should be omitted: %s", data)
	}
	if jsonHasField(data, "epoch") {
		t.Fatalf("zero uint32 field 'epoch' should be omitted: %s", data)
	}
	if jsonHasField(data, "cancelAt") {
		t.Fatalf("zero int64 field 'cancelAt' should be omitted: %s", data)
	}
}

// TestListKeypairsResponse_ProtoJSON verifies repeated EntityKeypair serialization.
func TestListKeypairsResponse_ProtoJSON(t *testing.T) {
	msg := &ListKeypairsResponse{
		Keypairs: []*session.EntityKeypair{
			{
				PeerId:     "peer-111",
				AuthMethod: "password",
				AuthParams: []byte{0x0a, 0x0b},
			},
			{
				PeerId:     "peer-222",
				AuthMethod: "passkey",
			},
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify camelCase field names in nested EntityKeypair.
	if !jsonHasField(data, "peerId") {
		t.Fatalf("expected nested camelCase field 'peerId' in JSON: %s", data)
	}
	if !jsonHasField(data, "authMethod") {
		t.Fatalf("expected nested camelCase field 'authMethod' in JSON: %s", data)
	}
	if !jsonHasField(data, "authParams") {
		t.Fatalf("expected nested camelCase field 'authParams' in JSON: %s", data)
	}

	// Roundtrip.
	got := &ListKeypairsResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestAccountAuthMethod_ProtoJSON verifies auth-method metadata serialization.
func TestAccountAuthMethod_ProtoJSON(t *testing.T) {
	msg := &AccountAuthMethod{
		PeerId:         "peer-google",
		Kind:           AccountAuthMethodKind_ACCOUNT_AUTH_METHOD_KIND_GOOGLE_SSO,
		Provider:       "google",
		Label:          "Google",
		SecondaryLabel: "user@example.com",
		Keypair: &session.EntityKeypair{
			PeerId:     "peer-google",
			AuthMethod: "google_sso",
		},
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	if !jsonHasField(data, "secondaryLabel") {
		t.Fatalf("expected camelCase field 'secondaryLabel' in JSON: %s", data)
	}
	if !jsonHasField(data, "provider") {
		t.Fatalf("expected field 'provider' in JSON: %s", data)
	}
	if !jsonHasField(data, "keypair") {
		t.Fatalf("expected field 'keypair' in JSON: %s", data)
	}

	got := &AccountAuthMethod{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestPasskeyRegisterVerifyRequest_ProtoJSON verifies string field for opaque JSON.
func TestPasskeyRegisterVerifyRequest_ProtoJSON(t *testing.T) {
	credJSON := `{"id":"abc123","type":"public-key","response":{"attestationObject":"base64data"}}`
	msg := &PasskeyRegisterVerifyRequest{
		CredentialJson:   credJSON,
		PrfCapable:       true,
		EncryptedPrivkey: "encrypted-key-data",
		PeerId:           "peer-333",
		AuthParams:       "base64-auth-params",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify camelCase field names.
	if !jsonHasField(data, "credentialJson") {
		t.Fatalf("expected camelCase field 'credentialJson' in JSON: %s", data)
	}
	if !jsonHasField(data, "prfCapable") {
		t.Fatalf("expected camelCase field 'prfCapable' in JSON: %s", data)
	}
	if !jsonHasField(data, "encryptedPrivkey") {
		t.Fatalf("expected camelCase field 'encryptedPrivkey' in JSON: %s", data)
	}

	// Bool field true should be present as bare true.
	if !bytes.Contains(data, []byte(`"prfCapable":true`)) {
		t.Fatalf("bool field prfCapable should be bare true, got: %s", data)
	}

	// Roundtrip.
	got := &PasskeyRegisterVerifyRequest{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}

	// Verify the opaque JSON string survived roundtrip exactly.
	if got.CredentialJson != credJSON {
		t.Fatalf("credentialJson roundtrip mismatch: got %q, want %q", got.CredentialJson, credJSON)
	}
}

// TestPasskeyAuthVerifyResponse_ProtoJSON verifies mixed bool + string fields.
func TestPasskeyAuthVerifyResponse_ProtoJSON(t *testing.T) {
	msg := &PasskeyAuthVerifyResponse{
		Verified:      true,
		AccountId:     "acct-456",
		EntityId:      "bob",
		EncryptedBlob: "encrypted-blob-data",
		PrfCapable:    true,
		PrfSalt:       "salt-value",
		AuthParams:    "auth-params-data",
		PinWrapped:    true,
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify all camelCase field names.
	for _, field := range []string{
		"verified", "accountId", "entityId", "encryptedBlob",
		"prfCapable", "prfSalt", "authParams", "pinWrapped",
	} {
		if !jsonHasField(data, field) {
			t.Fatalf("expected camelCase field %q in JSON: %s", field, data)
		}
	}

	// Verify bool fields are bare true.
	if !bytes.Contains(data, []byte(`"verified":true`)) {
		t.Fatalf("bool field verified should be bare true, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"prfCapable":true`)) {
		t.Fatalf("bool field prfCapable should be bare true, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"pinWrapped":true`)) {
		t.Fatalf("bool field pinWrapped should be bare true, got: %s", data)
	}

	// Roundtrip.
	got := &PasskeyAuthVerifyResponse{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}
}

// TestPasskeyAuthVerifyResponse_FalseBoolOmitted verifies false bools are omitted.
func TestPasskeyAuthVerifyResponse_FalseBoolOmitted(t *testing.T) {
	msg := &PasskeyAuthVerifyResponse{
		AccountId: "acct-456",
		EntityId:  "bob",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// False bool fields should be omitted in proto3.
	if jsonHasField(data, "verified") {
		t.Fatalf("false bool field 'verified' should be omitted: %s", data)
	}
	if jsonHasField(data, "prfCapable") {
		t.Fatalf("false bool field 'prfCapable' should be omitted: %s", data)
	}
	if jsonHasField(data, "pinWrapped") {
		t.Fatalf("false bool field 'pinWrapped' should be omitted: %s", data)
	}
}

// TestRecoverExecuteRequest_ProtoJSON verifies nested message serialization.
func TestRecoverExecuteRequest_ProtoJSON(t *testing.T) {
	msg := &RecoverExecuteRequest{
		Token: "recovery-token-abc",
		AddKeypair: &RecoverExecuteKeypair{
			PeerId:     "peer-444",
			AuthMethod: "password",
			AuthParams: "base64-auth-params",
		},
		Signatures: []*RecoverExecuteSignature{
			{
				PeerId:    "peer-444",
				Signature: "base64-sig-1",
			},
			{
				PeerId:    "peer-555",
				Signature: "base64-sig-2",
			},
		},
		RemovePeerId: "peer-old",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Verify top-level camelCase field names.
	if !jsonHasField(data, "addKeypair") {
		t.Fatalf("expected camelCase field 'addKeypair' in JSON: %s", data)
	}
	if !jsonHasField(data, "removePeerId") {
		t.Fatalf("expected camelCase field 'removePeerId' in JSON: %s", data)
	}

	// Verify nested RecoverExecuteKeypair fields.
	if !jsonHasField(data, "authMethod") {
		t.Fatalf("expected nested camelCase field 'authMethod' in JSON: %s", data)
	}
	if !jsonHasField(data, "authParams") {
		t.Fatalf("expected nested camelCase field 'authParams' in JSON: %s", data)
	}

	// Verify nested RecoverExecuteSignature fields.
	if !bytes.Contains(data, []byte(`"base64-sig-1"`)) {
		t.Fatalf("expected signature value in JSON: %s", data)
	}
	if !bytes.Contains(data, []byte(`"base64-sig-2"`)) {
		t.Fatalf("expected second signature value in JSON: %s", data)
	}

	// Roundtrip.
	got := &RecoverExecuteRequest{}
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !msg.EqualVT(got) {
		t.Fatalf("roundtrip mismatch:\n  orig: %+v\n  got:  %+v", msg, got)
	}

	// Verify nested message field values survived roundtrip.
	if got.AddKeypair == nil {
		t.Fatal("AddKeypair should not be nil after roundtrip")
	}
	if got.AddKeypair.PeerId != "peer-444" {
		t.Fatalf("AddKeypair.PeerId: got %q, want %q", got.AddKeypair.PeerId, "peer-444")
	}
	if len(got.Signatures) != 2 {
		t.Fatalf("Signatures length: got %d, want 2", len(got.Signatures))
	}
	if got.Signatures[0].Signature != "base64-sig-1" {
		t.Fatalf("Signatures[0].Signature: got %q, want %q", got.Signatures[0].Signature, "base64-sig-1")
	}
}

// TestRecoverExecuteRequest_NilNested verifies nil nested message is omitted.
func TestRecoverExecuteRequest_NilNested(t *testing.T) {
	msg := &RecoverExecuteRequest{
		Token: "token-only",
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// Nil nested message fields should be omitted.
	if jsonHasField(data, "addKeypair") {
		t.Fatalf("nil nested message 'addKeypair' should be omitted: %s", data)
	}
	// Nil repeated field should be omitted.
	if jsonHasField(data, "signatures") {
		t.Fatalf("nil repeated field 'signatures' should be omitted: %s", data)
	}
}

// TestUnmarshalAcceptsSnakeCase verifies unmarshal accepts both camelCase and
// snake_case.
func TestUnmarshalAcceptsSnakeCase(t *testing.T) {
	// Proto-JSON unmarshal should accept both forms.
	snakeJSON := []byte(`{"success_url":"https://example.com","cancel_url":"https://cancel.com","billing_interval":"BillingInterval_YEAR"}`)

	got := &CheckoutRequest{}
	if err := got.UnmarshalJSON(snakeJSON); err != nil {
		t.Fatalf("UnmarshalJSON with snake_case: %v", err)
	}
	if got.SuccessUrl != "https://example.com" {
		t.Fatalf("SuccessUrl: got %q, want %q", got.SuccessUrl, "https://example.com")
	}
	if got.CancelUrl != "https://cancel.com" {
		t.Fatalf("CancelUrl: got %q, want %q", got.CancelUrl, "https://cancel.com")
	}
	if got.BillingInterval != s4wave_provider_spacewave.BillingInterval_BillingInterval_YEAR {
		t.Fatalf("BillingInterval: got %v, want %v", got.BillingInterval, s4wave_provider_spacewave.BillingInterval_BillingInterval_YEAR)
	}
}

// TestBillingUsageResponse_DoubleVsInt64Types verifies type distinction in JSON.
func TestBillingUsageResponse_DoubleVsInt64Types(t *testing.T) {
	msg := &BillingUsageResponse{
		StorageBytes: 1024.0,
		WriteOps:     1024,
		ReadOps:      2048,
	}

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	// double (float64) 1024.0 should serialize as a bare number.
	// The exact format depends on strconv.FormatFloat with 'f' format.
	if !bytes.Contains(data, []byte(`"storageBytes":1024`)) {
		t.Fatalf("double field storageBytes should be a bare number, got: %s", data)
	}

	// int64 1024 should be a quoted string "1024".
	if !bytes.Contains(data, []byte(`"writeOps":"1024"`)) {
		t.Fatalf("int64 field writeOps should be a quoted string, got: %s", data)
	}
	if !bytes.Contains(data, []byte(`"readOps":"2048"`)) {
		t.Fatalf("int64 field readOps should be a quoted string, got: %s", data)
	}
}
