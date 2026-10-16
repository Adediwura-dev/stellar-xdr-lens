package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const (
	ledgerHeaderHashNote = "08090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f2021222324252627"
	previousHashNote     = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	closeTimeNote        = "2026-09-21T14:13:20Z"
)

func TestDecodeLedgerHeaderFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "ledger_header"), "LedgerHeader")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "LedgerHeader" {
		t.Errorf("type = %q, want LedgerHeader", v.Type)
	}
	raw, ok := v.Raw.(*xdr.LedgerHeader)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.LedgerHeader", v.Raw)
	}
	if raw.LedgerSeq != 55278432 {
		t.Errorf("LedgerSeq = %d, want 55278432", raw.LedgerSeq)
	}
	if raw.LedgerVersion != 22 {
		t.Errorf("LedgerVersion = %d, want 22", raw.LedgerVersion)
	}
}

// TestLedgerHeaderFieldsRenderHumanValues is the heart of the specialised
// rendering: a ledger header's totals and protocol parameters are stroop
// amounts, and reading them as XLM rather than raw integers is the point.
func TestLedgerHeaderFieldsRenderHumanValues(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "ledger_header"), "LedgerHeader")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}

	tests := []struct {
		path     string
		wantNote string
	}{
		{"LedgerVersion", "protocol 22"},
		{"TotalCoins", "104591297310.9999952 (stroops: 1045912973109999952)"},
		{"FeePool", "1234.0000000 (stroops: 12340000000)"},
		{"BaseFee", "0.0000100 (stroops: 100)"},
		{"BaseReserve", "0.5000000 (stroops: 5000000)"},
		{"ScpValue.CloseTime", closeTimeNote},
		{"PreviousLedgerHash", previousHashNote},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			n := v.Node.Find(tc.path)
			if n == nil {
				t.Fatalf("could not find %s in the decoded tree", tc.path)
			}
			if n.Note != tc.wantNote {
				t.Errorf("%s note = %q, want %q", tc.path, n.Note, tc.wantNote)
			}
		})
	}
}

func TestDecodeLedgerHeaderHistoryEntryFixture(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs(loadFixture(t, "ledger_header_history"), "LedgerHeaderHistoryEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if v.Type != "LedgerHeaderHistoryEntry" {
		t.Errorf("type = %q, want LedgerHeaderHistoryEntry", v.Type)
	}
	raw, ok := v.Raw.(*xdr.LedgerHeaderHistoryEntry)
	if !ok {
		t.Fatalf("Raw is %T, want *xdr.LedgerHeaderHistoryEntry", v.Raw)
	}
	if raw.Header.LedgerSeq != 55278432 {
		t.Errorf("Header.LedgerSeq = %d, want 55278432", raw.Header.LedgerSeq)
	}

	hash := v.Node.Find("Hash")
	if hash == nil {
		t.Fatal("could not find Hash in the decoded history entry")
	}
	if !strings.Contains(hash.Note, ledgerHeaderHashNote) {
		t.Errorf("Hash note = %q, want it to contain %q", hash.Note, ledgerHeaderHashNote)
	}

	// The embedded header gets the same field renderings as a standalone one.
	seq := v.Node.Find("Header.LedgerSeq")
	if seq == nil {
		t.Fatal("could not find Header.LedgerSeq in the decoded history entry")
	}
	if got, ok := seq.Value.(uint64); !ok || got != 55278432 {
		t.Errorf("Header.LedgerSeq = %v, want 55278432", seq.Value)
	}
	total := v.Node.Find("Header.TotalCoins")
	if total == nil || !strings.Contains(total.Note, "104591297310.9999952") {
		t.Errorf("Header.TotalCoins = %v, want a stroop rendering", total)
	}
}

func TestDecodeLedgerHeaderRejectsTruncatedInput(t *testing.T) {
	t.Parallel()

	if _, err := lens.DecodeAs(loadFixtureRaw("ledger_header")[:64], "LedgerHeader"); err == nil {
		t.Error("DecodeAs() error = nil, want an error for a truncated ledger header")
	}
	if _, err := lens.DecodeAs(loadFixtureRaw("ledger_header_history")[:64], "LedgerHeaderHistoryEntry"); err == nil {
		t.Error("DecodeAs() error = nil, want an error for a truncated history entry")
	}
}

// TestDetectReportsLedgerHeader guards that the registered ledger types are
// unambiguous for a full payload: a header must not be mistaken for any other
// type in the registry, which is what makes `lens decode` trustworthy.
func TestDetectReportsLedgerHeader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture  string
		wantType string
	}{
		{"ledger_header", "LedgerHeader"},
		{"ledger_header_history", "LedgerHeaderHistoryEntry"},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()

			cands, err := lens.Detect(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Detect() error = %v", err)
			}
			if len(cands) != 1 {
				names := make([]string, 0, len(cands))
				for _, c := range cands {
					names = append(names, c.Type)
				}
				t.Errorf("Detect() matched %d types (%s), want exactly 1",
					len(cands), strings.Join(names, ", "))
			}
			if cands[0].Type != tc.wantType {
				t.Errorf("Detect() best candidate = %q, want %q", cands[0].Type, tc.wantType)
			}
		})
	}
}
