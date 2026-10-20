package lens

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// testCreditAsset builds a classic credit asset for the internal tests, which
// cannot use the lens_test package helpers.
func testCreditAsset(t *testing.T, code, issuer string) xdr.Asset {
	t.Helper()
	var aid xdr.AccountId
	if err := aid.SetAddress(issuer); err != nil {
		t.Fatalf("parsing issuer %s: %v", issuer, err)
	}
	var a xdr.Asset
	if err := a.SetCredit(code, aid); err != nil {
		t.Fatalf("building asset %s: %v", code, err)
	}
	return a
}

func TestEnrichHash(t *testing.T) {
	hash := xdr.Hash{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20}

	display, raw, ok := enrichHash(reflect.ValueOf(hash))
	if !ok {
		t.Fatal("enrichHash failed")
	}

	expected := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	if display != expected {
		t.Errorf("got %q, want %q", display, expected)
	}
	if raw != hash {
		t.Errorf("raw value altered")
	}
}

func TestEnrichPoolID(t *testing.T) {
	poolID := xdr.PoolId{0x0A, 0x0B, 0x0C} // Rest are zero

	note, raw, ok := enrichPoolID(reflect.ValueOf(poolID))
	if !ok {
		t.Fatal("enrichPoolID failed")
	}

	if note != "constant product pool id" {
		t.Errorf("note = %q, want %q", note, "constant product pool id")
	}
	expected := "0a0b0c0000000000000000000000000000000000000000000000000000000000"
	if raw != expected {
		t.Errorf("raw = %v, want hex %q", raw, expected)
	}
}

func TestEnrichPoolIDRejectsWrongType(t *testing.T) {
	t.Parallel()

	if _, _, ok := enrichPoolID(reflect.ValueOf("not a pool id")); ok {
		t.Error("enrichPoolID accepted a non-PoolId value")
	}
}

func TestPoolShareBalanceNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"shares format as decimal tokens", xdr.Int64(2_000_000_000_000), "200000.0000000 pool share tokens (stroops: 2000000000000)", true},
		{"fractional shares", xdr.Int64(15_000_000), "1.5000000 pool share tokens (stroops: 15000000)", true},
		{"zero shares", xdr.Int64(0), "0.0000000 pool share tokens (stroops: 0)", true},
		{"negative declines", xdr.Int64(-1), "", false},
		{"wrong type declines", xdr.Uint32(5), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			note, ok := poolShareBalanceNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("poolShareBalanceNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

func TestPoolSharesTrustLineCountNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		wantNote string
		wantOK   bool
	}{
		{"plural", xdr.Int64(150), "held by 150 trustlines", true},
		{"singular", xdr.Int64(1), "held by 1 trustline", true},
		{"zero", xdr.Int64(0), "held by 0 trustlines", true},
		{"negative declines", xdr.Int64(-2), "", false},
		{"wrong type declines", xdr.Uint32(3), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			note, ok := poolSharesTrustLineCountNote(reflect.ValueOf(tc.value))
			if ok != tc.wantOK || note != tc.wantNote {
				t.Errorf("poolSharesTrustLineCountNote(%v) = (%q, %v), want (%q, %v)",
					tc.value, note, ok, tc.wantNote, tc.wantOK)
			}
		})
	}
}

func TestDecorateLiquidityPoolEntry(t *testing.T) {
	t.Parallel()

	entry := xdr.LiquidityPoolEntry{
		LiquidityPoolId: xdr.PoolId{0x41},
		Body: xdr.LiquidityPoolEntryBody{
			Type: xdr.LiquidityPoolTypeLiquidityPoolConstantProduct,
			ConstantProduct: &xdr.LiquidityPoolEntryConstantProduct{
				Params: xdr.LiquidityPoolConstantProductParameters{
					AssetA: xdr.MustNewNativeAsset(),
					AssetB: testCreditAsset(t, "USDC", "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"),
					Fee:    xdr.Int32(30),
				},
			},
		},
	}
	node := buildNode("LiquidityPoolEntry", reflect.ValueOf(entry), 0)
	if node == nil {
		t.Fatal("buildNode(LiquidityPoolEntry) returned nil")
	}
	id := node.Child("LiquidityPoolId")
	if id == nil {
		t.Fatal("LiquidityPoolId child not found")
	}
	want := "constant product pool: native / USDC:"
	if len(id.Note) < len(want) || id.Note[:len(want)] != want {
		t.Errorf("LiquidityPoolId note = %q, want prefix %q", id.Note, want)
	}
	if !strings.Contains(id.Note, "@ 0.3%") {
		t.Errorf("LiquidityPoolId note = %q, want fee", id.Note)
	}
}

func TestDecorateLiquidityPoolEntryLeavesMalformedEntryAlone(t *testing.T) {
	t.Parallel()

	entry := xdr.LiquidityPoolEntry{
		LiquidityPoolId: xdr.PoolId{0x41},
		Body:            xdr.LiquidityPoolEntryBody{Type: xdr.LiquidityPoolTypeLiquidityPoolConstantProduct},
	}
	node := buildNode("LiquidityPoolEntry", reflect.ValueOf(entry), 0)
	if node == nil {
		t.Fatal("buildNode(LiquidityPoolEntry) returned nil")
	}
	id := node.Child("LiquidityPoolId")
	if id == nil {
		t.Fatal("LiquidityPoolId child not found")
	}
	if id.Note != "constant product pool id" {
		t.Errorf("LiquidityPoolId note = %q, want the plain pool id note", id.Note)
	}
}

func TestEnrichSignature(t *testing.T) {
	sig := xdr.Signature{0xFF, 0xEE, 0xDD}

	display, raw, ok := enrichSignature(reflect.ValueOf(sig))
	if !ok {
		t.Fatal("enrichSignature failed")
	}

	expected := "ffeedd"
	if display != expected {
		t.Errorf("got %q, want %q", display, expected)
	}
	// bytes slice equality
	rawSig := raw.(xdr.Signature)
	if len(rawSig) != len(sig) || rawSig[0] != sig[0] || rawSig[1] != sig[1] || rawSig[2] != sig[2] {
		t.Errorf("raw value altered")
	}
}

// The domain enrichers are registered against exact types, so routing one a
// different value means a bug in registration. They must decline cleanly
// rather than panic or invent a rendering.
func TestDomainEnrichersRejectWrongType(t *testing.T) {
	t.Parallel()

	wrong := reflect.ValueOf("not an xdr value")
	enrichers := map[string]Enricher{
		"enrichPoolParameters":     enrichPoolParameters,
		"enrichClaimPredicate":     enrichClaimPredicate,
		"enrichClaimableBalanceID": enrichClaimableBalanceID,
		"enrichContractID":         enrichContractID,
		"enrichScAddress":          enrichScAddress,
		"enrichUpgradeType":        enrichUpgradeType,
		"enrichLedgerUpgrade":      enrichLedgerUpgrade,
	}
	for name, fn := range enrichers {
		if _, _, ok := fn(wrong); ok {
			t.Errorf("%s accepted a non-XDR value", name)
		}
	}
}

func TestClaimableBalanceIDRejectsUnknownType(t *testing.T) {
	t.Parallel()

	id := xdr.ClaimableBalanceId{Type: xdr.ClaimableBalanceIdType(99)}
	if _, _, ok := enrichClaimableBalanceID(reflect.ValueOf(id)); ok {
		t.Error("enrichClaimableBalanceID accepted an unknown id type")
	}
}

func TestScAddressRejectsUnknownType(t *testing.T) {
	t.Parallel()

	addr := xdr.ScAddress{Type: xdr.ScAddressType(99)}
	if _, _, ok := enrichScAddress(reflect.ValueOf(addr)); ok {
		t.Error("enrichScAddress accepted an unknown address type")
	}
}

func TestTrustLineFlagNote(t *testing.T) {
	t.Parallel()

	if _, ok := trustLineFlagNote(reflect.ValueOf(xdr.Uint32(0))); ok {
		t.Error("trustLineFlagNote produced a note for an empty flag set")
	}
	if _, ok := trustLineFlagNote(reflect.ValueOf("x")); ok {
		t.Error("trustLineFlagNote accepted a non-Uint32 value")
	}
}

// TestTrustLineFlagNoteCombinations pins the expansion of every documented
// trustline flag bit, including the combined bitmask used by a clawback-
// enabled, fully authorized line.
func TestTrustLineFlagNoteCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		flags xdr.Uint32
		want  string
		ok    bool
	}{
		{"zero declines", 0, "", false},
		{
			"authorized",
			xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
			"authorized",
			true,
		},
		{
			"maintain liabilities only",
			xdr.Uint32(xdr.TrustLineFlagsAuthorizedToMaintainLiabilitiesFlag),
			"authorized_to_maintain_liabilities",
			true,
		},
		{
			"clawback only",
			xdr.Uint32(xdr.TrustLineFlagsTrustlineClawbackEnabledFlag),
			"trustline_clawback_enabled",
			true,
		},
		{
			"authorized and clawback",
			xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag |
				xdr.TrustLineFlagsTrustlineClawbackEnabledFlag),
			"authorized | trustline_clawback_enabled",
			true,
		},
		{
			"all three flags",
			xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag |
				xdr.TrustLineFlagsAuthorizedToMaintainLiabilitiesFlag |
				xdr.TrustLineFlagsTrustlineClawbackEnabledFlag),
			"authorized | authorized_to_maintain_liabilities | trustline_clawback_enabled",
			true,
		},
		{"unknown bits alone decline", xdr.Uint32(0x8), "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := trustLineFlagNote(reflect.ValueOf(tc.flags))
			if ok != tc.ok || note != tc.want {
				t.Errorf("trustLineFlagNote(%d) = (%q, %v), want (%q, %v)",
					tc.flags, note, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestTrustLineLiabilitiesAmountFormatting pins the shared amount-field
// enricher for the V1 liabilities: Buying and Selling are Int64 token
// balances and must render as decimal units alongside the raw stroop integer,
// exactly like every other amount field in the protocol.
func TestTrustLineLiabilitiesAmountFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		value xdr.Int64
		want  string
	}{
		{"buying", "Buying", xdr.Int64(100 * amount.One), "100.0000000 (stroops: 1000000000)"},
		{"selling", "Selling", xdr.Int64(200 * amount.One), "200.0000000 (stroops: 2000000000)"},
		{"zero buying", "Buying", xdr.Int64(0), "0.0000000 (stroops: 0)"},
		{"balance", "Balance", xdr.Int64(50 * amount.One), "50.0000000 (stroops: 500000000)"},
		{"limit", "Limit", xdr.Int64(1000 * amount.One), "1000.0000000 (stroops: 10000000000)"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			note, ok := enrichAmountField(tc.field, reflect.ValueOf(tc.value))
			if !ok || note != tc.want {
				t.Errorf("enrichAmountField(%q, %d) = (%q, %v), want (%q, true)",
					tc.field, tc.value, note, ok, tc.want)
			}
		})
	}
}

func TestClaimableBalanceFlagNote(t *testing.T) {
	t.Parallel()

	clawback := xdr.Uint32(xdr.ClaimableBalanceFlagsClaimableBalanceClawbackEnabledFlag)
	if note, ok := claimableBalanceFlagNote(reflect.ValueOf(clawback)); !ok || note != "clawback enabled" {
		t.Errorf("clawback note = %q, %v; want 'clawback enabled', true", note, ok)
	}
	if _, ok := claimableBalanceFlagNote(reflect.ValueOf(xdr.Uint32(0))); ok {
		t.Error("claimableBalanceFlagNote produced a note for an empty flag set")
	}
	if _, ok := claimableBalanceFlagNote(reflect.ValueOf("x")); ok {
		t.Error("claimableBalanceFlagNote accepted a non-Uint32 value")
	}
}

func TestEnrichContractExecutableWasm(t *testing.T) {
	t.Parallel()

	hash := xdr.Hash{0x4F}
	exec := xdr.ContractExecutable{
		Type:     xdr.ContractExecutableTypeContractExecutableWasm,
		WasmHash: &hash,
	}
	note, raw, ok := enrichContractExecutable(reflect.ValueOf(exec))
	if !ok {
		t.Fatal("enrichContractExecutable declined a wasm executable")
	}
	wantHex := hash.HexString()
	if note != "wasm:"+wantHex {
		t.Errorf("note = %q, want %q", note, "wasm:"+wantHex)
	}
	if raw != wantHex {
		t.Errorf("raw = %v, want %q", raw, wantHex)
	}
}

func TestEnrichContractExecutableStellarAsset(t *testing.T) {
	t.Parallel()

	exec := xdr.ContractExecutable{Type: xdr.ContractExecutableTypeContractExecutableStellarAsset}
	note, raw, ok := enrichContractExecutable(reflect.ValueOf(exec))
	if !ok {
		t.Fatal("enrichContractExecutable declined the stellar asset arm")
	}
	if note != "stellar asset contract (built-in token)" {
		t.Errorf("note = %q, want the built-in token description", note)
	}
	if raw != "stellar_asset" {
		t.Errorf("raw = %v, want stellar_asset", raw)
	}
}

func TestEnrichContractExecutableExternalRef(t *testing.T) {
	t.Parallel()

	owner := xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract}
	var contractID xdr.ContractId
	contractID[0] = 0xCA
	owner.ContractId = &contractID

	ownerStr, err := owner.String()
	if err != nil {
		t.Fatalf("encoding owner: %v", err)
	}

	exec := xdr.ContractExecutable{
		Type: xdr.ContractExecutableTypeContractExecutableExternalRef,
		ExternalRef: &xdr.ContractExecutableExternalRef{
			ExecutableOwner: owner,
			Tag:             "evm:1",
		},
	}
	note, raw, ok := enrichContractExecutable(reflect.ValueOf(exec))
	if !ok {
		t.Fatal("enrichContractExecutable declined the external ref arm")
	}
	if note != "external contract reference" {
		t.Errorf("note = %q, want 'external contract reference'", note)
	}
	if raw != ownerStr+" tag: evm:1" {
		t.Errorf("raw = %v, want owner and tag", raw)
	}
}

func TestEnrichContractExecutableDeclinesMalformed(t *testing.T) {
	t.Parallel()

	cases := map[string]xdr.ContractExecutable{
		"unknown type":         {Type: xdr.ContractExecutableType(99)},
		"missing wasm hash":    {Type: xdr.ContractExecutableTypeContractExecutableWasm},
		"missing external ref": {Type: xdr.ContractExecutableTypeContractExecutableExternalRef},
		"unknown owner type": {
			Type: xdr.ContractExecutableTypeContractExecutableExternalRef,
			ExternalRef: &xdr.ContractExecutableExternalRef{
				ExecutableOwner: xdr.ScAddress{Type: xdr.ScAddressType(99)},
				Tag:             "x",
			},
		},
	}
	for name, exec := range cases {
		if _, _, ok := enrichContractExecutable(reflect.ValueOf(exec)); ok {
			t.Errorf("%s: enrichContractExecutable produced a rendering, want decline", name)
		}
	}
	if _, _, ok := enrichContractExecutable(reflect.ValueOf("not an executable")); ok {
		t.Error("enrichContractExecutable accepted a non-ContractExecutable value")
	}
}

func TestBuildNodeContractExecutableWasm(t *testing.T) {
	t.Parallel()

	hash := xdr.Hash{0xAB}
	instance := xdr.ScContractInstance{
		Executable: xdr.ContractExecutable{
			Type:     xdr.ContractExecutableTypeContractExecutableWasm,
			WasmHash: &hash,
		},
	}

	node := buildNode("Instance", reflect.ValueOf(instance), 0)
	if node == nil {
		t.Fatal("buildNode(ScContractInstance) returned nil")
	}
	exec := node.Child("Executable")
	if exec == nil {
		t.Fatal("Executable child not found")
	}
	if exec.Note != "wasm:"+hash.HexString() {
		t.Errorf("Executable Note = %q, want the wasm annotation", exec.Note)
	}
}
