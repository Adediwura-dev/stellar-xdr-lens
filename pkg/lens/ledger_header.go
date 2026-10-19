package lens

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Ledger headers carry the protocol parameters and the totals that a validator
// agreed on for one ledger. Four of those fields are stroop amounts — the
// total supply, the fee pool and the two protocol parameters that set the
// price of ledger entries — and none of them is self-describing on the wire,
// which is what this file teaches the walker.
//
// Beyond the plain parameters, a header also carries:
//   - liquidity-pool control flags in the V1 extension, and
//   - upgrade votes: opaque blobs in the SCP value, each one an encoded
//     LedgerUpgrade proposing a protocol change.
//
// Both are invisible on the wire without decoding, so this file expands the
// flag bits into their canonical names and unmarshals each upgrade vote into
// the parameter it proposes. A ledger header is deliberately not collapsed
// into a single scalar by a type enricher: formatters and the differ consume
// its tree, and hiding the fields would throw away the structure that makes
// the tree useful. The human rendering is the sum of its per-field notes
// instead.
func init() {
	// TotalCoins and FeePool are Int64 stroop amounts. BaseFee and
	// BaseReserve are Uint32 protocol parameters, also denominated in
	// stroops: the base fee is the price of one operation, and the base
	// reserve is the minimum balance a ledger entry locks up.
	amountFields["TotalCoins"] = true
	amountFields["FeePool"] = true
	amountFields["BaseFee"] = true
	amountFields["BaseReserve"] = true

	// The version field is a bare Uint32; naming it as a protocol version is
	// the difference between "22" and knowing what "22" means. The same
	// field name appears in BucketMetadata, which gets the note for free.
	fieldNotes["LedgerVersion"] = protocolVersionNote

	// Liquidity-pool control flags live in the V1 header extension. The
	// containing type is keyed so other Flags fields stay untouched.
	registerFieldNote(reflect.TypeOf(xdr.LedgerHeaderExtensionV1{}), "Flags", ledgerHeaderFlagNote)
}

// protocolVersionNote renders a protocol version field as "protocol N", which
// is how the value is referred to everywhere outside the XDR itself. A zero
// version is not a released protocol, so it declines and stays a plain 0.
func protocolVersionNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok || v == 0 {
		return "", false
	}
	return "protocol " + strconv.FormatUint(uint64(v), 10), true
}

// ledgerHeaderFlagNames decodes the header flag bit mask into the canonical
// Horizon-style names. Unknown bits are ignored rather than guessed at.
func ledgerHeaderFlagNames(flags xdr.Uint32) []string {
	var names []string
	if flags&xdr.Uint32(xdr.LedgerHeaderFlagsDisableLiquidityPoolTradingFlag) != 0 {
		names = append(names, "disable_liquidity_pool_trading")
	}
	if flags&xdr.Uint32(xdr.LedgerHeaderFlagsDisableLiquidityPoolDepositFlag) != 0 {
		names = append(names, "disable_liquidity_pool_deposit")
	}
	if flags&xdr.Uint32(xdr.LedgerHeaderFlagsDisableLiquidityPoolWithdrawalFlag) != 0 {
		names = append(names, "disable_liquidity_pool_withdrawal")
	}
	return names
}

// ledgerHeaderFlagNote renders the set header flags for display.
func ledgerHeaderFlagNote(rv reflect.Value) (string, bool) {
	flags, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	names := ledgerHeaderFlagNames(flags)
	if len(names) == 0 {
		return "", false
	}
	return strings.Join(names, " | "), true
}
