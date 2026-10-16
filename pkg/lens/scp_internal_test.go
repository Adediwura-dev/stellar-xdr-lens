package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const scpTestNodeAddr = "GBIFCUSTKRKVMV2YLFNFWXC5LZPWAYLCMNSGKZTHNBUWU23MNVXG6NJZ"

func scpTestNodeID(t *testing.T) xdr.NodeId {
	t.Helper()

	raw := strkey.MustDecode(strkey.VersionByteAccountID, scpTestNodeAddr)
	var key xdr.Uint256
	copy(key[:], raw)
	return xdr.NodeId{Type: xdr.PublicKeyTypePublicKeyTypeEd25519, Ed25519: &key}
}

func TestEnrichNodeID(t *testing.T) {
	t.Parallel()

	display, raw, ok := enrichNodeID(reflect.ValueOf(scpTestNodeID(t)))
	if !ok {
		t.Fatal("enrichNodeID() declined a valid node id")
	}
	if display != "" || raw != scpTestNodeAddr {
		t.Errorf("enrichNodeID() = (%q, %v), want (\"\", %q)", display, raw, scpTestNodeAddr)
	}
}

func TestEnrichPublicKey(t *testing.T) {
	t.Parallel()

	pk := xdr.PublicKey(scpTestNodeID(t))
	display, raw, ok := enrichPublicKey(reflect.ValueOf(pk))
	if !ok {
		t.Fatal("enrichPublicKey() declined a valid public key")
	}
	if display != "" || raw != scpTestNodeAddr {
		t.Errorf("enrichPublicKey() = (%q, %v), want (\"\", %q)", display, raw, scpTestNodeAddr)
	}
}

// TestScpKeyEnrichersDeclineMalformedValues covers the hostile-input path: a
// zero union has no populated arm, and the SDK accessor panics on it. The
// registry wrapper recovers and the generic rendering takes over.
func TestScpKeyEnrichersDeclineMalformedValues(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]any{
		"PublicKey": xdr.PublicKey{},
		"NodeId":    xdr.NodeId{},
	} {
		if display, raw, ok := enrich(reflect.ValueOf(value)); ok {
			t.Errorf("enrich(%s zero) = (%q, %v, true), want ok=false", name, display, raw)
		}
	}
}

func TestScpKeyEnrichersDeclineUnexpectedTypes(t *testing.T) {
	t.Parallel()

	wrong := reflect.ValueOf(xdr.AccountId{})
	for name, fn := range map[string]Enricher{
		"enrichPublicKey": enrichPublicKey,
		"enrichNodeID":    enrichNodeID,
	} {
		if display, raw, ok := fn(wrong); ok {
			t.Errorf("%s(AccountId) = (%q, %v, true), want ok=false", name, display, raw)
		}
	}
}

func TestEnrichScpStatementType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value  xdr.ScpStatementType
		want   string
		wantOK bool
	}{
		{xdr.ScpStatementTypeScpStPrepare, "prepare", true},
		{xdr.ScpStatementTypeScpStConfirm, "confirm", true},
		{xdr.ScpStatementTypeScpStExternalize, "externalize", true},
		{xdr.ScpStatementTypeScpStNominate, "nominate", true},
		{xdr.ScpStatementType(9), "", false},
	}

	for _, tc := range tests {
		_, raw, ok := enrichScpStatementType(reflect.ValueOf(tc.value))
		if ok != tc.wantOK || (tc.wantOK && raw != tc.want) {
			t.Errorf("enrichScpStatementType(%d) = (_, %v, %v), want (_, %q, %v)",
				tc.value, raw, ok, tc.want, tc.wantOK)
		}
	}

	if _, _, ok := enrichScpStatementType(reflect.ValueOf(xdr.AccountId{})); ok {
		t.Error("enrichScpStatementType(AccountId) succeeded, want ok=false")
	}
}

func TestSlotIndexNote(t *testing.T) {
	t.Parallel()

	note, ok := slotIndexNote(reflect.ValueOf(xdr.Uint64(55278000)))
	if !ok || note != "ledger slot 55278000" {
		t.Errorf("slotIndexNote(55278000) = (%q, %v), want (ledger slot 55278000, true)", note, ok)
	}
	if _, ok := slotIndexNote(reflect.ValueOf(xdr.Uint32(1))); ok {
		t.Error("slotIndexNote(Uint32) succeeded, want ok=false")
	}
}
