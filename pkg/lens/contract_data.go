package lens

import (
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func init() {
	registerEnricher(reflect.TypeOf(xdr.ContractDataDurability(0)), enrichContractDataDurability)
	registerEnricher(reflect.TypeOf(xdr.ScVal{}), enrichStorageKey)
}

// enrichContractDataDurability renders a contract data entry's storage
// durability in the terms the Soroban documentation uses: "persistent" for
// storage that survives its entry expiring, and "temporary" for storage that
// does not. Instance storage is recorded as persistent on the wire and is
// recognised by its key, which enrichStorageKey names.
func enrichContractDataDurability(rv reflect.Value) (display string, raw any, ok bool) {
	d, valid := rv.Interface().(xdr.ContractDataDurability)
	if !valid {
		return "", nil, false
	}
	switch d {
	case xdr.ContractDataDurabilityPersistent:
		return "", "persistent", true
	case xdr.ContractDataDurabilityTemporary:
		return "", "temporary", true
	default:
		return "", nil, false
	}
}

// enrichStorageKey names the special ScVal ledger keys that identify Soroban
// storage entries. The contract instance key is what marks an entry as
// instance storage: it is recorded with persistent durability, so the key is
// the only signal that separates instance from plain persistent storage. The
// nonce key keeps its nonce, which is the part callers actually need.
//
// Every other ScVal declines here and keeps the structural rendering, which
// stays machine-readable without a contract spec — the same trade-off the
// roadmap documents for Soroban values generally.
func enrichStorageKey(rv reflect.Value) (display string, raw any, ok bool) {
	key, valid := rv.Interface().(xdr.ScVal)
	if !valid {
		return "", nil, false
	}
	switch key.Type {
	case xdr.ScValTypeScvLedgerKeyContractInstance:
		return "", "instance storage key", true
	case xdr.ScValTypeScvLedgerKeyNonce:
		// A malformed nonce key (missing arm) panics inside the accessor;
		// enrich recovers and the generic rendering takes over.
		return "", fmt.Sprintf("nonce key: %d", key.MustNonceKey().Nonce), true
	default:
		return "", nil, false
	}
}
