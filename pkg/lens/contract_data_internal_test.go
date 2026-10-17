package lens

import (
	"reflect"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestEnrichContractDataDurabilityFallsBackForUnknownLevel(t *testing.T) {
	t.Parallel()

	// A durability outside the enum must decline enrichment so the value
	// falls through to the generic integer rendering.
	display, raw, ok := enrichContractDataDurability(reflect.ValueOf(xdr.ContractDataDurability(7)))
	if ok {
		t.Errorf("enrichContractDataDurability(7) = (%q, %v, true), want ok=false", display, raw)
	}
}

// TestContractDataEnrichersDeclineUnexpectedTypes pins that each enricher is
// safe to call with a value it does not own — the registry lookup is by type,
// but the interface assertions are the last line of defence.
func TestContractDataEnrichersDeclineUnexpectedTypes(t *testing.T) {
	t.Parallel()

	wrong := reflect.ValueOf(xdr.AccountId{})
	tests := []struct {
		name string
		fn   Enricher
	}{
		{"enrichScAddress", enrichScAddress},
		{"enrichContractDataDurability", enrichContractDataDurability},
		{"enrichStorageKey", enrichStorageKey},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			display, raw, ok := tc.fn(wrong)
			if ok {
				t.Errorf("%s(AccountId) = (%q, %v, true), want ok=false", tc.name, display, raw)
			}
		})
	}
}

func TestEnrichScAddressFallsBackForUnknownType(t *testing.T) {
	t.Parallel()

	// An ScAddress carrying an unknown type has a well-formed Go value but
	// no known strkey form; the enricher declines and the generic rendering
	// wins.
	display, raw, ok := enrichScAddress(reflect.ValueOf(xdr.ScAddress{Type: xdr.ScAddressType(99)}))
	if ok {
		t.Errorf("enrichScAddress(unknown type) = (%q, %v, true), want ok=false", display, raw)
	}
}

func TestEnrichContractDataDurabilityRendersKnownLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		durability xdr.ContractDataDurability
		want       string
	}{
		{xdr.ContractDataDurabilityPersistent, "persistent"},
		{xdr.ContractDataDurabilityTemporary, "temporary"},
	}

	for _, tc := range tests {
		got, raw, ok := enrichContractDataDurability(reflect.ValueOf(tc.durability))
		if !ok {
			t.Errorf("enrichContractDataDurability(%d) declined, want it to render", tc.durability)
			continue
		}
		if got != "" || raw != tc.want {
			t.Errorf("enrichContractDataDurability(%d) = (%q, %v), want (\"\", %q)", tc.durability, got, raw, tc.want)
		}
	}
}

func TestEnrichStorageKeyDeclinesForOrdinaryValues(t *testing.T) {
	t.Parallel()

	// Ordinary ScVals keep the structural rendering; only the two ledger
	// key variants are claimed by the name enrichment.
	value := xdr.ScVal{Type: xdr.ScValTypeScvU32}
	display, raw, ok := enrichStorageKey(reflect.ValueOf(value))
	if ok {
		t.Errorf("enrichStorageKey(u32) = (%q, %v, true), want ok=false", display, raw)
	}
}

func TestEnrichScAddressFallsBackForMalformedAddress(t *testing.T) {
	t.Parallel()

	// A zero ScAddress carries no union arm; String panics and enrich must
	// recover and decline rather than take the process down.
	display, raw, ok := enrich(reflect.ValueOf(xdr.ScAddress{}))
	if ok {
		t.Errorf("enrich(zero ScAddress) = (%q, %v, true), want ok=false", display, raw)
	}
}

func TestEnrichStorageKeyFallsBackForMalformedNonceKey(t *testing.T) {
	t.Parallel()

	// A nonce key missing its arm panics inside the SDK accessor; enrich
	// recovers and the generic rendering takes over.
	malformed := xdr.ScVal{Type: xdr.ScValTypeScvLedgerKeyNonce}
	display, raw, ok := enrich(reflect.ValueOf(malformed))
	if ok {
		t.Errorf("enrich(malformed nonce key) = (%q, %v, true), want ok=false", display, raw)
	}
}
