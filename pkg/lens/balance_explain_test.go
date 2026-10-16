package lens_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func mustAccountEntry(t *testing.T, fixture string) *xdr.AccountEntry {
	t.Helper()
	v, err := lens.DecodeAs(loadFixture(t, fixture), "AccountEntry")
	if err != nil {
		t.Fatalf("DecodeAs(%s) error = %v", fixture, err)
	}
	entry, ok := v.Raw.(*xdr.AccountEntry)
	if !ok {
		t.Fatalf("fixture %s decoded to %T, want *xdr.AccountEntry", fixture, v.Raw)
	}
	return entry
}

func TestAccountLiabilitiesFromFixture(t *testing.T) {
	t.Parallel()
	entry := mustAccountEntry(t, "account_entry_ext_v1")
	if got := lens.AccountSellingLiabilities(entry); got != 250000000 {
		t.Errorf("selling liabilities = %d, want 250000000", got)
	}
	if got := lens.AccountBuyingLiabilities(entry); got != 1500000000 {
		t.Errorf("buying liabilities = %d, want 1500000000", got)
	}
}

func TestAccountLiabilitiesNilAndBare(t *testing.T) {
	t.Parallel()
	if got := lens.AccountSellingLiabilities(nil); got != 0 {
		t.Errorf("nil selling = %d, want 0", got)
	}
	if got := lens.AccountBuyingLiabilities(nil); got != 0 {
		t.Errorf("nil buying = %d, want 0", got)
	}
	bare := &xdr.AccountEntry{Balance: 100}
	if got := lens.AccountSellingLiabilities(bare); got != 0 {
		t.Errorf("bare selling = %d, want 0", got)
	}
	if got := lens.AccountBuyingLiabilities(bare); got != 0 {
		t.Errorf("bare buying = %d, want 0", got)
	}
}

func TestReserveRequirementMath(t *testing.T) {
	t.Parallel()
	// 2 + 3 subentries at 0.5 XLM each = 2.5 XLM = 25000000 stroops.
	if got := lens.ReserveRequirement(3, 0, 0, 5000000); got != 25000000 {
		t.Errorf("reserve = %d, want 25000000", got)
	}
	// Sponsoring 2, sponsored 1: 2+3+2-1 = 6 entries.
	if got := lens.ReserveRequirement(3, 2, 1, 5000000); got != 30000000 {
		t.Errorf("sponsored reserve = %d, want 30000000", got)
	}
	// Sponsored cannot drive the requirement negative.
	if got := lens.ReserveRequirement(0, 0, 10, 5000000); got != 0 {
		t.Errorf("over-sponsored reserve = %d, want 0", got)
	}
	if got := lens.AvailableBalance(100, 10, 20); got != 70 {
		t.Errorf("available = %d, want 70", got)
	}
}

func TestAccountReserveFromFixtures(t *testing.T) {
	t.Parallel()
	v1 := mustAccountEntry(t, "account_entry_ext_v1")
	// No sponsorship data: 2 + 3 = 5 entries * 0.5 XLM.
	if got := lens.AccountReserveRequirement(v1, 5000000); got != 25000000 {
		t.Errorf("v1 reserve = %d, want 25000000", got)
	}
	v2 := mustAccountEntry(t, "account_entry_ext_v2")
	sponsoring, sponsored := lens.AccountSponsorshipCounts(v2)
	if sponsoring != 2 || sponsored != 1 {
		t.Errorf("sponsorship = (%d,%d), want (2,1)", sponsoring, sponsored)
	}
	// 2+3+2-1 = 6 entries.
	if got := lens.AccountReserveRequirement(v2, 5000000); got != 30000000 {
		t.Errorf("v2 reserve = %d, want 30000000", got)
	}
	if sponsoring2, sponsored2 := lens.AccountSponsorshipCounts(nil); sponsoring2 != 0 || sponsored2 != 0 {
		t.Errorf("nil sponsorship = (%d,%d), want (0,0)", sponsoring2, sponsored2)
	}
	bare := &xdr.AccountEntry{}
	if sponsoring3, sponsored3 := lens.AccountSponsorshipCounts(bare); sponsoring3 != 0 || sponsored3 != 0 {
		t.Errorf("bare sponsorship = (%d,%d), want (0,0)", sponsoring3, sponsored3)
	}
	if got := lens.AccountReserveRequirement(nil, 5000000); got != 10000000 {
		t.Errorf("nil reserve = %d, want 10000000", got)
	}
}

func TestDiagnoseBalanceSufficient(t *testing.T) {
	t.Parallel()
	entry := mustAccountEntry(t, "account_entry_ext_v1")
	// Balance 1000 XLM, selling 25 XLM, reserve 2.5 XLM => ~972.5 available.
	d, err := lens.DiagnoseBalance(entry, 5000000, 100000000) // spend 10 XLM
	if err != nil {
		t.Fatalf("DiagnoseBalance() error = %v", err)
	}
	if !d.Sufficient {
		t.Error("Sufficient = false, want true")
	}
	if d.Shortfall != 0 {
		t.Errorf("Shortfall = %d, want 0", d.Shortfall)
	}
	if d.AvailableBalance != int64(entry.Balance)-250000000-25000000 {
		t.Errorf("Available = %d, want balance-selling-reserve", d.AvailableBalance)
	}
	if d.Hint != "" {
		t.Errorf("Hint = %q, want empty when sufficient", d.Hint)
	}
	if d.Summary == "" || !strings.Contains(d.Summary, "fits") {
		t.Errorf("Summary = %q, want sufficiency wording", d.Summary)
	}
}

func TestDiagnoseBalanceShortfall(t *testing.T) {
	t.Parallel()
	entry := mustAccountEntry(t, "account_entry_ext_v1")
	huge := int64(100000000000) // 10000 XLM, far beyond the 1000 XLM balance
	d, err := lens.DiagnoseBalance(entry, 5000000, huge)
	if err != nil {
		t.Fatalf("DiagnoseBalance() error = %v", err)
	}
	if d.Sufficient {
		t.Error("Sufficient = true, want false")
	}
	wantShort := huge - d.AvailableBalance
	if d.Shortfall != wantShort {
		t.Errorf("Shortfall = %d, want %d", d.Shortfall, wantShort)
	}
	if !strings.Contains(d.Summary, "falls short") {
		t.Errorf("Summary = %q, want shortfall wording", d.Summary)
	}
	if d.Hint == "" {
		t.Error("Hint is empty, want remediation advice")
	}
	// Selling liabilities must be mentioned since this fixture has them.
	if !strings.Contains(d.Summary, "selling liabilities") {
		t.Errorf("Summary = %q, want mention of selling liabilities", d.Summary)
	}
}

func TestDiagnoseBalanceDefaultsAndErrors(t *testing.T) {
	t.Parallel()
	entry := mustAccountEntry(t, "account_entry_ext_v1")
	d, err := lens.DiagnoseBalance(entry, 0, 1000)
	if err != nil {
		t.Fatalf("DiagnoseBalance(baseReserve=0) error = %v", err)
	}
	if d.BaseReserve != lens.DefaultBaseReserve {
		t.Errorf("BaseReserve = %d, want default %d", d.BaseReserve, lens.DefaultBaseReserve)
	}
	if _, err := lens.DiagnoseBalance(nil, 5000000, 1); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil entry error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseBalance(entry, 5000000, -1); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("negative spend error = %v, want ErrInvalidArgument", err)
	}
}

func TestDiagnoseBalanceFromValue(t *testing.T) {
	t.Parallel()
	v, err := lens.DecodeAs(loadFixture(t, "account_entry_ext_v1"), "AccountEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	d, err := lens.DiagnoseBalanceFromValue(v, 5000000, 1000)
	if err != nil {
		t.Fatalf("DiagnoseBalanceFromValue() error = %v", err)
	}
	if d.Balance != 10000000000 {
		t.Errorf("Balance = %d, want 10000000000", d.Balance)
	}
	if _, err := lens.DiagnoseBalanceFromValue(nil, 5000000, 1); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil value error = %v, want ErrInvalidArgument", err)
	}
	memo, err := lens.DecodeAs("AAAAAA==", "Memo")
	if err != nil {
		t.Fatalf("DecodeAs(Memo) error = %v", err)
	}
	if _, err := lens.DiagnoseBalanceFromValue(memo, 5000000, 1); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("wrong type error = %v, want ErrInvalidArgument", err)
	}
}

func TestDiagnoseTrustlineSpend(t *testing.T) {
	t.Parallel()
	v, err := lens.DecodeAs(loadFixture(t, "trustline_entry_v1"), "TrustLineEntry")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	entry := v.Raw.(*xdr.TrustLineEntry)
	ok, err := lens.DiagnoseTrustlineSpend(entry, 1000)
	if err != nil {
		t.Fatalf("DiagnoseTrustlineSpend() error = %v", err)
	}
	if !ok.Sufficient {
		t.Error("Sufficient = false, want true for tiny spend")
	}
	if ok.ReserveRequirement != 0 {
		t.Errorf("Reserve = %d, want 0 for trustlines", ok.ReserveRequirement)
	}
	huge := int64(1000000000000)
	short, err := lens.DiagnoseTrustlineSpend(entry, huge)
	if err != nil {
		t.Fatalf("DiagnoseTrustlineSpend(huge) error = %v", err)
	}
	if short.Sufficient {
		t.Error("Sufficient = true, want false for huge spend")
	}
	if short.Shortfall != huge-short.AvailableBalance {
		t.Errorf("Shortfall = %d, want huge-available", short.Shortfall)
	}
	if short.Hint == "" {
		t.Error("Hint is empty for trustline shortfall")
	}
	if _, err := lens.DiagnoseTrustlineSpend(nil, 1); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseTrustlineSpend(entry, -5); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("negative error = %v, want ErrInvalidArgument", err)
	}
	// Bare trustline without extensions carries no liabilities.
	bare := &xdr.TrustLineEntry{Balance: 500}
	bareOK, err := lens.DiagnoseTrustlineSpend(bare, 100)
	if err != nil {
		t.Fatalf("bare trustline error = %v", err)
	}
	if bareOK.SellingLiabilities != 0 || !bareOK.Sufficient {
		t.Errorf("bare = %+v, want zero liabilities and sufficient", bareOK)
	}
}
