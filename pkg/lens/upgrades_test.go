package lens_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// mustVote encodes a LedgerUpgrade into the opaque bytes a validator votes
// with in StellarValue.upgrades.
func mustVote(t *testing.T, upgrade xdr.LedgerUpgrade) xdr.UpgradeType {
	t.Helper()
	raw, err := upgrade.MarshalBinary()
	if err != nil {
		t.Fatalf("marshalling vote: %v", err)
	}
	return xdr.UpgradeType(raw)
}

func TestDecodeLedgerUpgradeFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fixture   string
		wantArm   string
		wantField string
		wantValue uint64
		wantNote  string
	}{
		{
			name:      "protocol version bump",
			fixture:   "ledger_upgrade_version",
			wantArm:   "LedgerUpgradeTypeLedgerUpgradeVersion",
			wantField: "NewLedgerVersion",
			wantValue: 21,
		},
		{
			name:      "base fee change",
			fixture:   "ledger_upgrade_base_fee",
			wantArm:   "LedgerUpgradeTypeLedgerUpgradeBaseFee",
			wantField: "NewBaseFee",
			wantValue: 1000,
			wantNote:  "0.0001000 (stroops: 1000)",
		},
		{
			name:      "base reserve change",
			fixture:   "ledger_upgrade_base_reserve",
			wantArm:   "LedgerUpgradeTypeLedgerUpgradeBaseReserve",
			wantField: "NewBaseReserve",
			wantValue: 10000000,
			wantNote:  "1.0000000 (stroops: 10000000)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, err := lens.DecodeAs(loadFixture(t, tc.fixture), "LedgerUpgrade")
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}

			// A decoded union shows the selected arm and nothing else.
			if len(v.Node.Children) != 2 {
				t.Fatalf("children = %d, want exactly Type plus one arm", len(v.Node.Children))
			}
			if got := fmt.Sprint(v.Node.Child("Type").Value); got != tc.wantArm {
				t.Errorf("Type = %q, want %q", got, tc.wantArm)
			}
			arm := v.Node.Child(tc.wantField)
			if arm == nil {
				t.Fatalf("missing %s node", tc.wantField)
			}
			if got := fmt.Sprint(arm.Value); got != fmt.Sprint(tc.wantValue) {
				t.Errorf("%s = %s, want %d", tc.wantField, got, tc.wantValue)
			}
			if arm.Note != tc.wantNote {
				t.Errorf("%s note = %q, want %q", tc.wantField, arm.Note, tc.wantNote)
			}
		})
	}
}

func TestDecodeStellarValueUpgradeVotes(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "stellar_value_votes"), "StellarValue")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}

	upgrades := v.Node.Child("Upgrades")
	if upgrades == nil || upgrades.Kind != lens.KindList {
		t.Fatalf("Upgrades node = %+v, want a list", upgrades)
	}
	if len(upgrades.Children) != 3 {
		t.Fatalf("Upgrades has %d children, want 3", len(upgrades.Children))
	}

	tests := []struct {
		index     int
		wantValue string
		wantNote  string
	}{
		{0, "0000000100000015", "protocol version 21"},
		{1, "0000000400989680", "base reserve 10000000 stroops (1.0000000 XLM)"},
		// An upgrade unknown to this build keeps its hex form and gains no
		// note, which is the protocol's whole point of opaque upgrade votes.
		{2, "deadbeef", ""},
	}

	for _, tc := range tests {
		vote := upgrades.Children[tc.index]
		if got := fmt.Sprint(vote.Value); got != tc.wantValue {
			t.Errorf("Upgrades[%d] value = %s, want %s", tc.index, got, tc.wantValue)
		}
		if vote.Note != tc.wantNote {
			t.Errorf("Upgrades[%d] note = %q, want %q", tc.index, vote.Note, tc.wantNote)
		}
	}
}

// TestDecodeStellarValueEveryUpgradeArm covers the arms that have no fixture
// by building the vote with the SDK and decoding the enclosing StellarValue
// through the public API.
func TestDecodeStellarValueEveryUpgradeArm(t *testing.T) {
	t.Parallel()

	maxTxSetSize := xdr.Uint32(1000)
	flags := xdr.Uint32(1)
	maxSorobanTxSetSize := xdr.Uint32(42)
	contractID := xdr.ContractId(sha256.Sum256([]byte("stellar-xdr-lens/config-upgrade-contract")))
	contentHash := xdr.Hash(sha256.Sum256([]byte("stellar-xdr-lens/config-upgrade-content")))

	sv := xdr.StellarValue{
		TxSetHash: xdr.Hash(sha256.Sum256([]byte("stellar-xdr-lens/upgrade-arms"))),
		CloseTime: xdr.TimePoint(1790000000),
		Upgrades: []xdr.UpgradeType{
			mustVote(t, xdr.LedgerUpgrade{
				Type:            xdr.LedgerUpgradeTypeLedgerUpgradeMaxTxSetSize,
				NewMaxTxSetSize: &maxTxSetSize,
			}),
			mustVote(t, xdr.LedgerUpgrade{
				Type:     xdr.LedgerUpgradeTypeLedgerUpgradeFlags,
				NewFlags: &flags,
			}),
			mustVote(t, xdr.LedgerUpgrade{
				Type: xdr.LedgerUpgradeTypeLedgerUpgradeConfig,
				NewConfig: &xdr.ConfigUpgradeSetKey{
					ContractId:  contractID,
					ContentHash: contentHash,
				},
			}),
			mustVote(t, xdr.LedgerUpgrade{
				Type:                   xdr.LedgerUpgradeTypeLedgerUpgradeMaxSorobanTxSetSize,
				NewMaxSorobanTxSetSize: &maxSorobanTxSetSize,
			}),
		},
		Ext: xdr.StellarValueExt{V: xdr.StellarValueTypeStellarValueBasic},
	}

	v, err := lens.DecodeAs(mustMarshal(t, &sv), "StellarValue")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}

	upgrades := v.Node.Child("Upgrades")
	if upgrades == nil || len(upgrades.Children) != 4 {
		t.Fatalf("Upgrades = %+v, want 4 children", upgrades)
	}

	wantNotes := []string{
		"max tx set size 1000",
		"ledger flags 1",
		"config upgrade set: contract CBL662UMC7LD7G7WN5NU3YSFO64UC7FZNNZ6IGT5GZALNS3P2HVVGKVG," +
			" content hash db3ade040d0aa30bb0adcea47f4032b48f64345cbc178789e8801780b6d4256c",
		"max Soroban tx set size 42",
	}
	for i, want := range wantNotes {
		if got := upgrades.Children[i].Note; got != want {
			t.Errorf("Upgrades[%d] note = %q, want %q", i, got, want)
		}
	}
}

func TestDetectUpgradeVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fixture   string
		wantFirst string
		// wantAlso lists types that decode the same bytes. Ambiguity here is
		// inherent: LedgerUpgrade arm 4 (base reserve) has the same wire
		// shape as an ScVal i32, and any pair of int32s is a Price.
		wantAlso []string
	}{
		{"protocol version vote", "ledger_upgrade_version", "LedgerUpgrade", []string{"Price"}},
		{"base fee vote", "ledger_upgrade_base_fee", "LedgerUpgrade", []string{"Price"}},
		{"base reserve vote", "ledger_upgrade_base_reserve", "ScVal", []string{"LedgerUpgrade", "Price"}},
		{"stellar value carrying votes", "stellar_value_votes", "StellarValue", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cands, err := lens.Detect(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Detect() error = %v", err)
			}
			if len(cands) == 0 || cands[0].Type != tc.wantFirst {
				t.Fatalf("Detect() best = %+v, want %s first", cands, tc.wantFirst)
			}
			for _, want := range tc.wantAlso {
				var found bool
				for _, c := range cands {
					if c.Type == want {
						found = true
					}
				}
				if !found {
					t.Errorf("Detect() = %+v, want %s among candidates", cands, want)
				}
			}
		})
	}
}

func TestDecodeLedgerUpgradeTruncated(t *testing.T) {
	t.Parallel()

	// Eight full bytes of base64 encode six bytes of XDR: enough for the
	// discriminant, not enough for the arm.
	payload := loadFixture(t, "ledger_upgrade_base_reserve")[:8]
	if _, err := lens.DecodeAs(payload, "LedgerUpgrade"); err == nil {
		t.Error("DecodeAs() with truncated vote error = nil, want an error")
	}
}
