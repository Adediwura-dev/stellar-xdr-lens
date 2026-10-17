package lens_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func testAccountEntry(master, low, med, high byte, signers []xdr.Signer) *xdr.AccountEntry {
	aid := xdr.MustAddress("GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN")
	return &xdr.AccountEntry{
		AccountId:     aid,
		Balance:       10000000000,
		Thresholds:    xdr.Thresholds{master, low, med, high},
		Signers:       signers,
		NumSubEntries: 0,
	}
}

func ed25519SignerKey(t *testing.T, fill byte) xdr.SignerKey {
	t.Helper()
	var raw xdr.Uint256
	for i := range raw {
		raw[i] = fill
	}
	key, err := xdr.NewSignerKey(xdr.SignerKeyTypeSignerKeyTypeEd25519, raw)
	if err != nil {
		t.Fatalf("NewSignerKey() error = %v", err)
	}
	return key
}

func decoratedWithHint(h xdr.SignatureHint) xdr.DecoratedSignature {
	return xdr.DecoratedSignature{Hint: h, Signature: xdr.Signature{1, 2, 3}}
}

func TestOperationThresholdLevels(t *testing.T) {
	t.Parallel()
	cases := map[xdr.OperationType]string{
		xdr.OperationTypeAllowTrust:            "low",
		xdr.OperationTypeSetTrustLineFlags:     "low",
		xdr.OperationTypeBumpSequence:          "low",
		xdr.OperationTypeClaimClaimableBalance: "low",
		xdr.OperationTypeAccountMerge:          "high",
		xdr.OperationTypeSetOptions:            "high",
		xdr.OperationTypePayment:               "medium",
		xdr.OperationTypeCreateAccount:         "medium",
		xdr.OperationTypeInvokeHostFunction:    "medium",
	}
	for op, want := range cases {
		if got := lens.OperationThresholdLevel(op); got != want {
			t.Errorf("OperationThresholdLevel(%s) = %q, want %q", op.String(), got, want)
		}
	}
}

func TestAccountThresholdAccessors(t *testing.T) {
	t.Parallel()
	entry := testAccountEntry(2, 1, 3, 5, nil)
	master, low, med, high := lens.AccountThresholds(entry)
	if master != 2 || low != 1 || med != 3 || high != 5 {
		t.Errorf("thresholds = (%d,%d,%d,%d), want (2,1,3,5)", master, low, med, high)
	}
	if got := lens.RequiredThresholdForLevel(entry, "low"); got != 1 {
		t.Errorf("low = %d, want 1", got)
	}
	if got := lens.RequiredThresholdForLevel(entry, "medium"); got != 3 {
		t.Errorf("medium = %d, want 3", got)
	}
	if got := lens.RequiredThresholdForLevel(entry, "high"); got != 5 {
		t.Errorf("high = %d, want 5", got)
	}
	if got := lens.RequiredThresholdForLevel(entry, "bogus"); got != 3 {
		t.Errorf("bogus level = %d, want medium fallback 3", got)
	}
	m0, l0, m10, h0 := lens.AccountThresholds(nil)
	if m0 != 0 || l0 != 0 || m10 != 0 || h0 != 0 {
		t.Errorf("nil thresholds = (%d,%d,%d,%d), want zeros", m0, l0, m10, h0)
	}
}

func TestSignerHintDerivation(t *testing.T) {
	t.Parallel()
	key := ed25519SignerKey(t, 0xAB)
	h, err := lens.SignerHint(key)
	if err != nil {
		t.Fatalf("SignerHint() error = %v", err)
	}
	raw := key.MustEd25519()
	if h[0] != raw[28] || h[3] != raw[31] {
		t.Errorf("hint %v does not match last 4 bytes of key", h)
	}
	// HashX and pre-auth keys use the same tail rule.
	var hxRaw xdr.Uint256
	for i := range hxRaw {
		hxRaw[i] = 0x11
	}
	hx, err := xdr.NewSignerKey(xdr.SignerKeyTypeSignerKeyTypeHashX, hxRaw)
	if err != nil {
		t.Fatalf("hashX key error = %v", err)
	}
	hh, err := lens.SignerHint(hx)
	if err != nil {
		t.Fatalf("SignerHint(hashX) error = %v", err)
	}
	if hh[0] != 0x11 {
		t.Errorf("hashX hint = %v, want trailing 0x11 bytes", hh)
	}
	pre, err := xdr.NewSignerKey(xdr.SignerKeyTypeSignerKeyTypePreAuthTx, hxRaw)
	if err != nil {
		t.Fatalf("preAuth key error = %v", err)
	}
	if _, err := lens.SignerHint(pre); err != nil {
		t.Errorf("SignerHint(preAuth) error = %v", err)
	}
	// Signed payload embeds an ed25519 key.
	sp := xdr.SignerKeyEd25519SignedPayload{Ed25519: hxRaw, Payload: []byte{1, 2}}
	signedPayload, err := xdr.NewSignerKey(xdr.SignerKeyTypeSignerKeyTypeEd25519SignedPayload, sp)
	if err != nil {
		t.Fatalf("signed payload key error = %v", err)
	}
	if _, err := lens.SignerHint(signedPayload); err != nil {
		t.Errorf("SignerHint(signedPayload) error = %v", err)
	}
	unknown := xdr.SignerKey{Type: xdr.SignerKeyType(99)}
	if _, err := lens.SignerHint(unknown); err == nil {
		t.Error("SignerHint(unknown) error = nil, want an error")
	}
	_ = strkey.VersionByteAccountID // keep strkey import referenced if unused elsewhere
}

func TestDiagnoseSignaturesSufficient(t *testing.T) {
	t.Parallel()
	signerKey := ed25519SignerKey(t, 0x22)
	signers := []xdr.Signer{{Key: signerKey, Weight: 1}}
	entry := testAccountEntry(1, 1, 2, 3, signers)
	masterHint, err := lens.SignerHint(xdr.MustSigner(entry.AccountId.Address()))
	if err != nil {
		t.Fatalf("master hint error = %v", err)
	}
	signerHint, err := lens.SignerHint(signerKey)
	if err != nil {
		t.Fatalf("signer hint error = %v", err)
	}
	sigs := []xdr.DecoratedSignature{decoratedWithHint(masterHint), decoratedWithHint(signerHint)}
	d, err := lens.DiagnoseSignatures(entry, sigs, 2)
	if err != nil {
		t.Fatalf("DiagnoseSignatures() error = %v", err)
	}
	if !d.Sufficient {
		t.Error("Sufficient = false, want true")
	}
	if d.AccumulatedWeight != 2 {
		t.Errorf("Accumulated = %d, want 2", d.AccumulatedWeight)
	}
	if d.MissingWeight != 0 {
		t.Errorf("Missing = %d, want 0", d.MissingWeight)
	}
	if d.Hint != "" {
		t.Errorf("Hint = %q, want empty when sufficient", d.Hint)
	}
	if !strings.Contains(d.Summary, "meeting threshold") {
		t.Errorf("Summary = %q, want success wording", d.Summary)
	}
}

func TestDiagnoseSignaturesShortfall(t *testing.T) {
	t.Parallel()
	signerKey := ed25519SignerKey(t, 0x33)
	entry := testAccountEntry(1, 1, 2, 3, []xdr.Signer{{Key: signerKey, Weight: 1}})
	masterHint, err := lens.SignerHint(xdr.MustSigner(entry.AccountId.Address()))
	if err != nil {
		t.Fatalf("master hint error = %v", err)
	}
	d, err := lens.DiagnoseSignatures(entry, []xdr.DecoratedSignature{decoratedWithHint(masterHint)}, 2)
	if err != nil {
		t.Fatalf("DiagnoseSignatures() error = %v", err)
	}
	if d.Sufficient {
		t.Error("Sufficient = true, want false")
	}
	if d.MissingWeight != 1 {
		t.Errorf("Missing = %d, want 1", d.MissingWeight)
	}
	if d.Hint == "" {
		t.Error("Hint is empty, want missing-signer guidance")
	}
	if !strings.Contains(d.Summary, "requires 1 more") {
		t.Errorf("Summary = %q, want shortfall wording", d.Summary)
	}
	// Unknown signatures are counted, not fatal.
	unknown := xdr.SignatureHint{9, 9, 9, 9}
	d2, err := lens.DiagnoseSignatures(entry, []xdr.DecoratedSignature{decoratedWithHint(unknown)}, 1)
	if err != nil {
		t.Fatalf("DiagnoseSignatures(unknown) error = %v", err)
	}
	if d2.UnknownSignatures != 1 {
		t.Errorf("Unknown = %d, want 1", d2.UnknownSignatures)
	}
	if d2.Sufficient {
		t.Error("Sufficient = true with only unknown sigs, want false")
	}
}

func TestDiagnoseSignaturesDuplicateAndZeroThreshold(t *testing.T) {
	t.Parallel()
	entry := testAccountEntry(1, 1, 1, 1, nil)
	masterHint, err := lens.SignerHint(xdr.MustSigner(entry.AccountId.Address()))
	if err != nil {
		t.Fatalf("master hint error = %v", err)
	}
	// The same signature twice must not double-count weight.
	d, err := lens.DiagnoseSignatures(entry, []xdr.DecoratedSignature{decoratedWithHint(masterHint), decoratedWithHint(masterHint)}, 1)
	if err != nil {
		t.Fatalf("DiagnoseSignatures() error = %v", err)
	}
	if d.AccumulatedWeight != 1 {
		t.Errorf("Accumulated = %d, want 1 (no double count)", d.AccumulatedWeight)
	}
	if d.UnknownSignatures != 1 {
		t.Errorf("Unknown = %d, want 1 for the duplicate", d.UnknownSignatures)
	}
	// Zero threshold is trivially satisfied.
	empty, err := lens.DiagnoseSignatures(entry, nil, 0)
	if err != nil {
		t.Fatalf("zero threshold error = %v", err)
	}
	if !empty.Sufficient {
		t.Error("zero threshold should be sufficient with no signatures")
	}
}

func TestDiagnoseSignaturesErrors(t *testing.T) {
	t.Parallel()
	if _, err := lens.DiagnoseSignatures(nil, nil, 1); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil entry error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseSignaturesForLevel(nil, nil, "medium"); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil level error = %v, want ErrInvalidArgument", err)
	}
}

func TestDiagnoseSignaturesForLevelAndTransaction(t *testing.T) {
	t.Parallel()
	signerKey := ed25519SignerKey(t, 0x44)
	entry := testAccountEntry(1, 1, 2, 5, []xdr.Signer{{Key: signerKey, Weight: 2}})
	signerHint, err := lens.SignerHint(signerKey)
	if err != nil {
		t.Fatalf("hint error = %v", err)
	}
	d, err := lens.DiagnoseSignaturesForLevel(entry, []xdr.DecoratedSignature{decoratedWithHint(signerHint)}, "medium")
	if err != nil {
		t.Fatalf("DiagnoseSignaturesForLevel() error = %v", err)
	}
	if d.ThresholdLevel != "medium" {
		t.Errorf("Level = %q, want medium", d.ThresholdLevel)
	}
	if d.RequiredThreshold != 2 {
		t.Errorf("Required = %d, want 2", d.RequiredThreshold)
	}
	if !d.Sufficient {
		t.Error("Sufficient = false, want true")
	}
	// Transaction wrapper picks the operation's level (payment => medium).
	envPayload := loadFixture(t, "tx_success.env")
	envVal, err := lens.Decode(envPayload)
	if err != nil {
		t.Fatalf("Decode(env) error = %v", err)
	}
	env := envVal.Raw.(*xdr.TransactionEnvelope)
	txDiag, err := lens.DiagnoseTransactionSignatures(env, entry)
	if err != nil {
		t.Fatalf("DiagnoseTransactionSignatures() error = %v", err)
	}
	if txDiag.ThresholdLevel != "medium" {
		t.Errorf("tx level = %q, want medium for payment", txDiag.ThresholdLevel)
	}
	if _, err := lens.DiagnoseTransactionSignatures(nil, entry); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil env error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseTransactionSignatures(env, nil); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil entry error = %v, want ErrInvalidArgument", err)
	}
	// Value wrapper rejects mismatched types.
	memo, err := lens.DecodeAs("AAAAAA==", "Memo")
	if err != nil {
		t.Fatalf("DecodeAs(Memo) error = %v", err)
	}
	acctVal, err := lens.DecodeAs(loadFixture(t, "account_entry_ext_v1"), "AccountEntry")
	if err != nil {
		t.Fatalf("DecodeAs(AccountEntry) error = %v", err)
	}
	if _, err := lens.DiagnoseSignaturesFromValues(nil, acctVal); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil envelope value error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseSignaturesFromValues(memo, acctVal); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("wrong envelope type error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseSignaturesFromValues(envVal, memo); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("wrong account type error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseSignaturesFromValues(envVal, acctVal); err != nil {
		t.Errorf("DiagnoseSignaturesFromValues() error = %v", err)
	}
}
