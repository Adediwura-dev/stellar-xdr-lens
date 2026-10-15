package lens

import (
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func init() {
	enricherFor[reflect.TypeOf(xdr.ScAddress{})] = enrichScAddress
	enricherFor[reflect.TypeOf(xdr.ContractDataDurability(0))] = enrichContractDataDurability
	enricherFor[reflect.TypeOf(xdr.ScVal{})] = enrichStorageKey
}

// enrichScAddress renders a Soroban address as its strkey form — "C..." for a
// contract, "G..." for an account, and the matching forms for muxed accounts,
// liquidity pools and claimable balances. The raw union is a discriminant plus
// a 32-byte hash, which is unreadable; the strkey is the scalar value itself,
// exactly as AccountId and Asset render.
func enrichScAddress(rv reflect.Value) (display string, raw any, ok bool) {
	address, valid := rv.Interface().(xdr.ScAddress)
	if !valid {
		return "", nil, false
	}
	s, err := address.String()
	if err != nil || s == "" {
		return "", nil, false
	}
	return "", s, true
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
