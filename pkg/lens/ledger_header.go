package lens

import (
	"reflect"
	"strconv"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Ledger headers carry the protocol parameters and the totals that a validator
// agreed on for one ledger. Four of those fields are stroop amounts — the
// total supply, the fee pool and the two protocol parameters that set the
// price of ledger entries — and none of them is self-describing on the wire,
// which is what this file teaches the walker.
//
// A ledger header is deliberately not collapsed into a single scalar by a
// type enricher: formatters and the differ consume its tree, and hiding the
// fields would throw away the structure that makes the tree useful. The
// human rendering is the sum of its per-field notes instead.
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
