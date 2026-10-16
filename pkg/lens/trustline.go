package lens

import (
	"reflect"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// Trust line ledger entries, including the V1 liabilities extension and the
// V2 liquidity pool use extension.
//
// TrustLineEntry.Flags is a bit mask whose meaning is invisible in the value
// alone — "3" tells a reader nothing — so the field note registered here
// spells the set flags out while leaving the raw number in place.

func init() {
	registerFieldNote(reflect.TypeOf(xdr.TrustLineEntry{}), "Flags", trustLineFlagNote)
}

// trustLineFlagNames decodes the trust line flag bit mask into the canonical
// names used by Horizon. Unknown bits are ignored rather than guessed at.
func trustLineFlagNames(flags xdr.Uint32) []string {
	var names []string
	if flags&xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag) != 0 {
		names = append(names, "authorized")
	}
	if flags&xdr.Uint32(xdr.TrustLineFlagsAuthorizedToMaintainLiabilitiesFlag) != 0 {
		names = append(names, "authorized_to_maintain_liabilities")
	}
	if flags&xdr.Uint32(xdr.TrustLineFlagsTrustlineClawbackEnabledFlag) != 0 {
		names = append(names, "trustline_clawback_enabled")
	}
	return names
}

// trustLineFlagNote renders the set flags for display.
func trustLineFlagNote(rv reflect.Value) (string, bool) {
	flags, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	names := trustLineFlagNames(flags)
	if len(names) == 0 {
		return "", false
	}
	return strings.Join(names, " | "), true
}
