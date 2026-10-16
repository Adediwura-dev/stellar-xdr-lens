package lens

import (
	"reflect"
	"strconv"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Stellar keeps the transaction sets and result sets it has applied in a
// history archive, one entry per ledger. A TransactionHistoryEntry carries the
// transaction set for a ledger; since protocol 19 the set itself is a
// GeneralizedTransactionSet, whose v1 form splits transactions into phases so
// they can be scheduled with data dependencies. A TransactionHistoryResultEntry
// pairs the same ledger sequence with the results of applying that set, and a
// LedgerCloseMetaBatch carries the close metadata for a contiguous run of
// ledgers as exported for analytics.
//
// The types are already structurally readable; what they lack is any clue that
// the leading integer is a ledger sequence rather than an arbitrary counter.
// Naming that field is the whole human-rendering job for this family.
func init() {
	fieldNotes["LedgerSeq"] = ledgerSeqNote
	fieldNotes["StartSequence"] = firstLedgerNote
	fieldNotes["EndSequence"] = lastLedgerNote
}

// ledgerSeqNote renders a ledger sequence field as "ledger N", which is how
// the value is referred to everywhere outside the XDR. Ledger zero is the
// genesis placeholder and is never a real sequence, so it declines.
func ledgerSeqNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok || v == 0 {
		return "", false
	}
	return "ledger " + strconv.FormatUint(uint64(v), 10), true
}

// firstLedgerNote names the opening ledger of a batch. A zero sequence means
// the batch bounds were not set, which is worth saying rather than showing a
// bare 0.
func firstLedgerNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	return "first ledger in batch " + strconv.FormatUint(uint64(v), 10), true
}

// lastLedgerNote names the closing ledger of a batch. When it equals the
// first, the batch covers exactly one ledger.
func lastLedgerNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	return "last ledger in batch " + strconv.FormatUint(uint64(v), 10), true
}
