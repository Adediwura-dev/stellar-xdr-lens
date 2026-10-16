package lens_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestReasonByCodeLookup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		code      string
		wantFound bool
		wantCode  string
	}{
		{
			name:      "known failure code",
			code:      "payment_underfunded",
			wantFound: true,
			wantCode:  "payment_underfunded",
		},
		{
			name:      "known transaction failure code",
			code:      "tx_bad_seq",
			wantFound: true,
			wantCode:  "tx_bad_seq",
		},
		{
			name:      "known success code",
			code:      "tx_success",
			wantFound: true,
			wantCode:  "tx_success",
		},
		{
			name:      "case insensitive and trimmed",
			code:      "  PAYMENT_UNDERFUNDED  ",
			wantFound: true,
			wantCode:  "payment_underfunded",
		},
		{
			name:      "unknown code",
			code:      "nonexistent_dummy_code",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reason, found := lens.ReasonByCode(tt.code)
			if found != tt.wantFound {
				t.Fatalf("ReasonByCode(%q) found = %v, want %v", tt.code, found, tt.wantFound)
			}
			if found && reason.Code != tt.wantCode {
				t.Errorf("ReasonByCode(%q) code = %q, want %q", tt.code, reason.Code, tt.wantCode)
			}
			if found && reason.Summary == "" {
				t.Errorf("ReasonByCode(%q) summary is empty", tt.code)
			}
		})
	}
}

func TestAllReasons(t *testing.T) {
	t.Parallel()

	reasons := lens.AllReasons()
	if len(reasons) == 0 {
		t.Fatal("AllReasons returned an empty slice")
	}

	// Verify the slice is sorted by Code
	for i := 1; i < len(reasons); i++ {
		if reasons[i-1].Code > reasons[i].Code {
			t.Errorf("AllReasons not sorted: index %d (%q) > index %d (%q)",
				i-1, reasons[i-1].Code, i, reasons[i].Code)
		}
	}
}

func TestReasonLookupTable(t *testing.T) {
	t.Parallel()

	table := lens.ReasonLookupTable()
	if len(table) == 0 {
		t.Fatal("ReasonLookupTable returned an empty map")
	}

	reason, ok := table["payment_underfunded"]
	if !ok {
		t.Fatal("payment_underfunded not found in ReasonLookupTable")
	}
	if reason.Code != "payment_underfunded" {
		t.Errorf("expected code payment_underfunded, got %q", reason.Code)
	}

	// Verify mutating the returned map does not alter library state
	table["payment_underfunded"] = lens.Reason{Code: "corrupted"}
	fresh, ok := lens.ReasonByCode("payment_underfunded")
	if !ok || fresh.Code != "payment_underfunded" {
		t.Errorf("ReasonLookupTable returned map allowed mutating internal state")
	}
}

func TestReasonLookupMatchesCanonicalFixtures(t *testing.T) {
	t.Parallel()

	// Load tx_failed.res.txt fixture and decode
	rawBytes, err := os.ReadFile("../../testdata/tx_failed.res.txt")
	if err != nil {
		rawBytes, err = os.ReadFile("testdata/tx_failed.res.txt")
	}
	if err != nil {
		rawBytes, err = os.ReadFile("../testdata/tx_failed.res.txt")
	}
	if err != nil {
		t.Skipf("cannot read fixture: %v", err)
	}

	payload := strings.TrimSpace(string(rawBytes))
	decoded, err := lens.Decode(payload)
	if err != nil {
		t.Fatalf("decoding fixture failed: %v", err)
	}

	summary, err := lens.Explain(decoded)
	if err != nil {
		t.Fatalf("explaining fixture failed: %v", err)
	}

	if summary.Outcome == nil {
		t.Fatal("expected summary outcome to be present")
	}

	// Outcome reason code should be resolvable via ReasonByCode
	resCode := summary.Outcome.Reason.Code
	lookupReason, found := lens.ReasonByCode(resCode)
	if !found {
		t.Fatalf("ReasonByCode(%q) failed for outcome code in canonical fixture", resCode)
	}
	if lookupReason.Code != resCode {
		t.Errorf("code mismatch: got %q, want %q", lookupReason.Code, resCode)
	}
}
