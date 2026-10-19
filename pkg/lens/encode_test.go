package lens_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

// decodeJSON renders a decoded value with the same stable JSON formatter the
// CLI uses, so the encoder is tested against the real output contract rather
// than a hand-written approximation.
func decodeJSON(t *testing.T, payload, typeName string) []byte {
	t.Helper()
	v, err := lens.DecodeAs(payload, typeName)
	if err != nil {
		t.Fatalf("DecodeAs(%s) error = %v", typeName, err)
	}
	var buf bytes.Buffer
	if err := format.NewJSONFormatter(format.WithIndent("")).Format(&buf, v); err != nil {
		t.Fatalf("JSON format error = %v", err)
	}
	return buf.Bytes()
}

func TestEncodeJSONRoundTripsFixtures(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name     string
		fixture  string
		typeName string
	}{
		{name: "transaction envelope", fixture: "tx_success.env", typeName: "TransactionEnvelope"},
		{name: "soroban envelope", fixture: "tx_soroban.env", typeName: "TransactionEnvelope"},
		{name: "fee bump envelope", fixture: "tx_failed.env", typeName: "TransactionEnvelope"},
		{name: "transaction result", fixture: "tx_failed.res", typeName: "TransactionResult"},
		{name: "ledger header", fixture: "ledger_header", typeName: "LedgerHeader"},
		{name: "ledger header history", fixture: "ledger_header_history", typeName: "LedgerHeaderHistoryEntry"},
		{name: "account extension v1", fixture: "account_entry_ext_v1", typeName: "AccountEntry"},
		{name: "account extension v2", fixture: "account_entry_ext_v2", typeName: "AccountEntry"},
		{name: "trustline v1", fixture: "trustline_entry_v1", typeName: "TrustLineEntry"},
		{name: "liquidity pool", fixture: "liquidity_pool_entry", typeName: "LiquidityPoolEntry"},
		{name: "contract data", fixture: "contract_data_temporary", typeName: "ContractDataEntry"},
		{name: "contract code", fixture: "contract_code", typeName: "ContractCodeEntry"},
		{name: "overlay hello", fixture: "overlay_hello", typeName: "Hello"},
		{name: "scp envelope", fixture: "scp_envelope", typeName: "ScpEnvelope"},
		{name: "scp quorum set", fixture: "scp_quorum_set", typeName: "ScpQuorumSet"},
		{name: "ttl entry", fixture: "ttl_entry", typeName: "TtlEntry"},
		{name: "asset usdc", fixture: "asset_usdc", typeName: "Asset"},
		{name: "asset native", fixture: "asset_native", typeName: "Asset"},
		{name: "price", fixture: "price", typeName: "Price"},
		{name: "data entry text", fixture: "data_entry_text", typeName: "DataEntry"},
		{name: "data entry binary", fixture: "data_entry_binary", typeName: "DataEntry"},
		{name: "generalized tx set", fixture: "generalized_tx_set", typeName: "GeneralizedTransactionSet"},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload := loadFixture(t, tc.fixture)
			value, err := lens.DecodeAs(payload, tc.typeName)
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			want, err := lens.Encode(value)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			got, err := lens.EncodeJSON(decodeJSON(t, payload, tc.typeName))
			if err != nil {
				t.Fatalf("EncodeJSON() error = %v", err)
			}
			if got != want {
				t.Errorf("EncodeJSON() = %s, want %s", got, want)
			}
		})
	}
}

// TestEncodeJSONPeerAddresses documents the one enrichment in the fixture set
// that is only partially reversible. A PeerAddress renders as host:port while
// its failure counter is carried as a note, so re-encoding preserves the
// addresses but resets that counter. The message stays structurally valid; the
// loss is inherent to the JSON contract, not an encoder bug.
func TestEncodeJSONPeerAddresses(t *testing.T) {
	t.Parallel()

	payload := loadFixture(t, "overlay_peers")
	encoded, err := lens.EncodeJSON(decodeJSON(t, payload, "StellarMessage"))
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	v, err := lens.DecodeAs(encoded, "StellarMessage")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	peers := v.Node.Child("Peers")
	if peers == nil || len(peers.Children) != 2 {
		t.Fatalf("Peers = %v, want two addresses", peers)
	}
	want := []string{"192.0.2.1:11625", "[2001:db8::1]:11625"}
	for i, w := range want {
		if got := peers.Children[i].Value; got != w {
			t.Errorf("Peers[%d] = %v, want %q", i, got, w)
		}
	}
}

func TestEncodeMatchesOriginalFixture(t *testing.T) {
	t.Parallel()

	for _, fixture := range []string{"tx_success.env", "tx_soroban.env", "tx_failed.env"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()
			payload := loadFixture(t, fixture)
			v, err := lens.Decode(payload)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			got, err := lens.Encode(v)
			if err != nil {
				t.Fatalf("Encode() error = %v", err)
			}
			if got != payload {
				t.Errorf("Encode() did not reproduce the canonical fixture")
			}
		})
	}
}

func TestEncodeRequiresValue(t *testing.T) {
	t.Parallel()

	_, err := lens.Encode(nil)
	if err == nil {
		t.Fatal("Encode(nil) error = nil, want an error")
	}
	if !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("error = %v, want ErrInvalidArgument", err)
	}
}

func TestEncodeJSONDocument(t *testing.T) {
	t.Parallel()

	payload := loadFixture(t, "tx_failed.res")
	document := decodeJSON(t, payload, "TransactionResult")

	got, err := lens.EncodeJSON(document)
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	want, err := lens.EncodeJSONAs("TransactionResult", document)
	if err != nil {
		t.Fatalf("EncodeJSONAs() error = %v", err)
	}
	if got != want {
		t.Errorf("EncodeJSON() and EncodeJSONAs() disagree")
	}

	// Passing a bare value (no document envelope) through the typed entry
	// point must work too.
	var doc struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(document, &doc); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	direct, err := lens.EncodeJSONAs("TransactionResult", doc.Value)
	if err != nil {
		t.Fatalf("EncodeJSONAs(raw value) error = %v", err)
	}
	if direct != want {
		t.Errorf("raw-value encode differs from document encode")
	}
}

func TestEncodeJSONRendersPassThrough(t *testing.T) {
	t.Parallel()

	payload := loadFixture(t, "tx_success.env")
	got, err := lens.EncodeJSON([]byte(`{"type":"TransactionEnvelope","xdr":"` + payload + `"}`))
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	if got != payload {
		t.Errorf("EncodeJSON() = %s, want the supplied xdr", got)
	}
}

func TestEncodeJSONScValLiteral(t *testing.T) {
	t.Parallel()

	got, err := lens.EncodeJSON([]byte(`{"symbol":"transfer"}`))
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	value, err := lens.DecodeAs(got, "ScVal")
	if err != nil {
		t.Fatalf("decoding encoded ScVal: %v", err)
	}
	symbol := value.Node.Child("Sym")
	if symbol == nil || symbol.Value != "transfer" {
		t.Errorf("encoded ScVal symbol = %v, want transfer", symbol)
	}
}

func TestEncodeJSONTransactionTemplate(t *testing.T) {
	t.Parallel()

	template := `{
		"source_account": "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H",
		"fee": "100",
		"sequence": "1",
		"operations": [{
			"type": "payment",
			"destination": "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW",
			"amount": "10000000"
		}]
	}`
	encoded, err := lens.EncodeJSON([]byte(template))
	if err != nil {
		t.Fatalf("EncodeJSON() error = %v", err)
	}
	if _, err := lens.DecodeAs(encoded, "TransactionEnvelope"); err != nil {
		t.Fatalf("encoded template is not a valid TransactionEnvelope: %v", err)
	}
}

func TestEncodeJSONSpecialValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		typeName string
		json     string
		check    func(t *testing.T, v *lens.Value)
	}{
		{
			name:     "account address",
			typeName: "AccountEntry",
			json:     `{"AccountId":"GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ","Balance":1,"SeqNum":1,"NumSubEntries":0,"InflationDest":null,"Flags":0,"HomeDomain":"","Thresholds":"01000000","Signers":null,"Ext":{"V":0}}`,
			check: func(t *testing.T, v *lens.Value) {
				t.Helper()
				if addr := v.Node.Child("AccountId"); addr == nil || addr.Value != "GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ" {
					t.Errorf("AccountId = %v", addr)
				}
			},
		},
		{
			name:     "asset",
			typeName: "Asset",
			json:     `"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"`,
			check: func(t *testing.T, v *lens.Value) {
				t.Helper()
				if v.Node.Value != "USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN" {
					t.Errorf("Asset = %v", v.Node.Value)
				}
			},
		},
		{
			name:     "price",
			typeName: "Price",
			json:     `"7/3 (= 2.3333333)"`,
			check: func(t *testing.T, v *lens.Value) {
				t.Helper()
				if v.Node.Value != "7/3 (= 2.3333333)" {
					t.Errorf("Price = %v", v.Node.Value)
				}
			},
		},
		{
			name:     "signature",
			typeName: "TransactionEnvelope",
			json:     `{"Type":"EnvelopeTypeEnvelopeTypeTx","V1":{"Tx":{"SourceAccount":"GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ","Fee":100,"SeqNum":1,"Cond":{"Type":"PreconditionTypePrecondNone"},"Memo":{"Type":"MemoTypeMemoNone"},"Operations":[],"Ext":{"V":0}},"Signatures":[{"Hint":"aabbccdd","Signature":"AQID"}]}}`,
			check: func(t *testing.T, v *lens.Value) {
				t.Helper()
				if sig := v.Node.Find("V1.Signatures[0].Signature"); sig == nil || sig.Value == nil {
					t.Errorf("signature node = %v", sig)
				}
			},
		},
		{
			name:     "data value text",
			typeName: "DataEntry",
			json:     `{"AccountId":"GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ","DataName":"hello","DataValue":"world","Ext":{"V":0}}`,
			check: func(t *testing.T, v *lens.Value) {
				t.Helper()
				if data := v.Node.Child("DataValue"); data == nil || data.Value != "world" {
					t.Errorf("DataValue = %v", data)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := lens.EncodeJSONAs(tc.typeName, []byte(tc.json))
			if err != nil {
				t.Fatalf("EncodeJSONAs() error = %v", err)
			}
			decoded, err := lens.DecodeAs(encoded, tc.typeName)
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			tc.check(t, decoded)
		})
	}
}

func TestEncodeJSONErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     []byte
		wantErr  error
		typeName string
	}{
		{name: "empty", data: []byte("  "), wantErr: lens.ErrInvalidJSON, typeName: "TransactionResult"},
		{name: "malformed", data: []byte("{"), wantErr: lens.ErrInvalidJSON, typeName: "TransactionResult"},
		{name: "unknown type", data: []byte(`{}`), wantErr: lens.ErrUnknownType, typeName: "NotAType"},
		{name: "unknown field", data: []byte(`{"Nope":1}`), wantErr: lens.ErrInvalidArgument, typeName: "TransactionResult"},
		{name: "wrong shape", data: []byte(`{"FeeCharged":"lots"}`), wantErr: lens.ErrInvalidArgument, typeName: "TransactionResult"},
		{name: "enum name", data: []byte(`{"Result":{"Code":"NotACode"}}`), wantErr: lens.ErrInvalidArgument, typeName: "TransactionResult"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := lens.EncodeJSONAs(tc.typeName, tc.data)
			if err == nil {
				t.Fatal("EncodeJSONAs() error = nil, want an error")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestEncodeJSONRequiresType(t *testing.T) {
	t.Parallel()

	_, err := lens.EncodeJSON([]byte(`{"value":{"FeeCharged":1}}`))
	if err == nil {
		t.Fatal("EncodeJSON() error = nil, want an error")
	}
	if !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("error = %v, want ErrInvalidArgument", err)
	}
}

func TestEncodeJSONRejectsTrailingData(t *testing.T) {
	t.Parallel()

	_, err := lens.EncodeJSON([]byte(`{"type":"Price","value":"1/2"} {"type":"Price"}`))
	if err == nil {
		t.Fatal("EncodeJSON() error = nil, want an error")
	}
	if !errors.Is(err, lens.ErrInvalidJSON) {
		t.Errorf("error = %v, want ErrInvalidJSON", err)
	}
}

func TestEncodeJSONClaimPredicateIsClassified(t *testing.T) {
	t.Parallel()

	payload := loadFixture(t, "claimable_balance_entry_simple")
	_, err := lens.EncodeJSON(decodeJSON(t, payload, "ClaimableBalanceEntry"))
	if err == nil {
		t.Fatal("EncodeJSON() error = nil, want the lossy claim predicate to be rejected")
	}
	if !errors.Is(err, lens.ErrUnsupportedType) {
		t.Errorf("error = %v, want ErrUnsupportedType", err)
	}
	if !strings.Contains(err.Error(), "ClaimPredicate") && !strings.Contains(err.Error(), "claim predicate") {
		t.Errorf("error %q does not name the unsupported value", err)
	}
}
