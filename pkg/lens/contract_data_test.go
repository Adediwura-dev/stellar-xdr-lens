package lens_test

import (
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const fixtureContract = "CDFP5OV6AAAQEAYEAUDAOCAJBIFQYDIOB4IBCEQTCQKRMFYYDENBWGZ3"

func TestDecodeContractDataEntryFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		fixture         string
		wantDurability  string
		wantInstanceKey bool
		wantKeySym      string
		wantValType     string
	}{
		{
			name:            "instance storage",
			fixture:         "contract_data_instance",
			wantDurability:  "persistent", // instance storage is persistent on the wire
			wantInstanceKey: true,
			wantValType:     "ScValTypeScvContractInstance",
		},
		{
			name:           "persistent storage",
			fixture:        "contract_data_persistent",
			wantDurability: "persistent",
			wantKeySym:     "COUNTER",
			wantValType:    "ScValTypeScvU32",
		},
		{
			name:           "temporary storage",
			fixture:        "contract_data_temporary",
			wantDurability: "temporary",
			wantKeySym:     "SEEN",
			wantValType:    "ScValTypeScvBool",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, err := lens.DecodeAs(loadFixture(t, tc.fixture), "ContractDataEntry")
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			if v.Type != "ContractDataEntry" {
				t.Errorf("type = %q, want ContractDataEntry", v.Type)
			}
			if _, ok := v.Raw.(*xdr.ContractDataEntry); !ok {
				t.Fatalf("Raw is %T, want *xdr.ContractDataEntry", v.Raw)
			}

			// The contract address renders as a C... strkey, not a raw union.
			contract := v.Node.Find("Contract")
			if contract == nil {
				t.Fatal("could not find Contract in the decoded tree")
			}
			if got := contract.Value.(string); got != fixtureContract {
				t.Errorf("Contract = %q, want %q", got, fixtureContract)
			}

			// Durability renders as the storage class, not an enum constant.
			durability := v.Node.Find("Durability")
			if durability == nil {
				t.Fatal("could not find Durability in the decoded tree")
			}
			if got := durability.Value.(string); got != tc.wantDurability {
				t.Errorf("Durability = %q, want %q", got, tc.wantDurability)
			}

			// Instance storage is marked by its key; every other entry key
			// keeps the structural ScVal rendering.
			key := v.Node.Find("Key")
			if key == nil {
				t.Fatal("could not find Key in the decoded tree")
			}
			if tc.wantInstanceKey {
				if key.Kind != lens.KindScalar {
					t.Fatalf("Key kind = %v, want scalar for the instance storage key", key.Kind)
				}
				if got := key.Value.(string); got != "instance storage key" {
					t.Errorf("Key = %q, want %q", got, "instance storage key")
				}
			} else {
				sym := key.Find("Sym")
				if sym == nil {
					t.Fatal("could not find Key.Sym in the decoded tree")
				}
				if got := sym.Value.(string); got != tc.wantKeySym {
					t.Errorf("Key.Sym = %q, want %q", got, tc.wantKeySym)
				}
			}

			valType := v.Node.Find("Val.Type")
			if valType == nil {
				t.Fatal("could not find Val.Type in the decoded tree")
			}
			if got := valType.Value.(string); got != tc.wantValType {
				t.Errorf("Val.Type = %q, want %q", got, tc.wantValType)
			}
		})
	}
}

// TestContractDataStorageLevelsAreDistinguished pins the property the decoder
// exists for: instance, persistent and temporary entries are told apart, even
// though instance storage is recorded with persistent durability and can only
// be recognised by its key.
func TestContractDataStorageLevelsAreDistinguished(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture string
		want    string
	}{
		{"contract_data_instance", "instance"},
		{"contract_data_persistent", "persistent"},
		{"contract_data_temporary", "temporary"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			v, err := lens.DecodeAs(loadFixture(t, tc.fixture), "ContractDataEntry")
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			if got := storageLevelOf(t, v); got != tc.want {
				t.Errorf("storage level = %q, want %q", got, tc.want)
			}
		})
	}
}

// storageLevelOf classifies a decoded contract data entry the way the
// protocol does: the instance storage key wins, then durability decides
// between persistent and temporary.
func storageLevelOf(t *testing.T, v *lens.Value) string {
	t.Helper()
	if key := v.Node.Find("Key"); key != nil && key.Kind == lens.KindScalar && key.Value == "instance storage key" {
		return "instance"
	}
	if d := v.Node.Find("Durability"); d != nil && d.Value == "temporary" {
		return "temporary"
	}
	return "persistent"
}

func TestDecodeContractDataInsideLedgerEntry(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "ledger_entry_contract_data"), "LedgerEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}

	dataType := v.Node.Find("Data.Type")
	if dataType == nil {
		t.Fatal("could not find Data.Type in the decoded ledger entry")
	}
	if got := dataType.Value.(string); got != "LedgerEntryTypeContractData" {
		t.Errorf("Data.Type = %q, want LedgerEntryTypeContractData", got)
	}

	entry := v.Node.Find("Data.ContractData")
	if entry == nil {
		t.Fatal("could not find Data.ContractData in the decoded ledger entry")
	}
	durability := entry.Find("Durability")
	if durability == nil || durability.Value != "persistent" {
		t.Errorf("Data.ContractData.Durability = %v, want persistent", durability)
	}
	contract := entry.Find("Contract")
	if contract == nil || contract.Value != fixtureContract {
		t.Errorf("Data.ContractData.Contract = %v, want %q", contract, fixtureContract)
	}
}

func TestDecodeContractDataRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	// A truncated contract data entry must report an error rather than panic
	// or silently decode a partial entry.
	if _, err := lens.DecodeAs(loadFixtureRaw("contract_data_persistent")[:24], "ContractDataEntry"); err == nil {
		t.Error("DecodeAs() error = nil, want an error for a truncated contract data entry")
	}
}

// TestDecodeNonceLedgerKey covers the second special ScVal storage key. It is
// constructed with the SDK encoder and round-tripped through the real decode
// path, since a nonce key entry cannot be captured as a standalone fixture.
func TestDecodeNonceLedgerKey(t *testing.T) {
	t.Parallel()

	nonce := xdr.ScNonceKey{Nonce: 5}
	key := xdr.ScVal{Type: xdr.ScValTypeScvLedgerKeyNonce, NonceKey: &nonce}

	v, err := lens.DecodeAs(mustMarshal(t, &key), "ScVal")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Node.Kind != lens.KindScalar {
		t.Fatalf("node kind = %v, want scalar", v.Node.Kind)
	}
	if got := v.Node.Value.(string); got != "nonce key: 5" {
		t.Errorf("value = %q, want %q", got, "nonce key: 5")
	}
}

func TestDetectReportsContractDataEntry(t *testing.T) {
	t.Parallel()

	cands, err := lens.Detect(loadFixture(t, "contract_data_persistent"))
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	names := make(map[string]bool, len(cands))
	for _, c := range cands {
		names[c.Type] = true
	}
	if !names["ContractDataEntry"] {
		t.Errorf("Detect() candidates = %v, want ContractDataEntry among them", cands)
	}
}
