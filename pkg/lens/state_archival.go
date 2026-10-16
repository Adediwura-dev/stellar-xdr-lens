package lens

import (
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Soroban's state archival keeps the live ledger bounded by giving every
// contract entry a time to live. A TTLEntry records when the entry it belongs
// to expires; a TTL ledger key identifies that record; and the eviction
// iterator is the cursor validators use to walk the bucket list looking for
// entries whose TTL has run out. All three are plain numbers and hashes on
// the wire, so the useful human rendering is naming what the numbers mean.
func init() {
	// A TTL is a ledger sequence, not a duration: rendering the field as the
	// absolute ledger it points at answers "when does this expire" directly.
	fieldNotes["LiveUntilLedgerSeq"] = liveUntilLedgerNote
}

// liveUntilLedgerNote renders the TTL field as an expiry in ledger terms.
// The entry is live through the named ledger, so it expires once the ledger
// sequence passes it. A TTL of zero means the entry has already expired —
// archived if persistent, deleted forever if temporary.
func liveUntilLedgerNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	if v == 0 {
		return "already expired (evict at next close)", true
	}
	return fmt.Sprintf("live through ledger %d", uint64(v)), true
}
