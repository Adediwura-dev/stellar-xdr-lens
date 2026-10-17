package lens

import (
	"encoding/base64"
	"reflect"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// TestMarshalBinary marshals an XDR value to binary for testing.
func TestMarshalBinary(v interface{ MarshalBinary() ([]byte, error) }) []byte {
	b, _ := v.MarshalBinary() //nolint:errcheck // test helper, error ignored
	return b
}

// GetEnricherForTest returns the enricher registered for a type, for testing.
func GetEnricherForTest(t reflect.Type) Enricher {
	return enricherFor[t]
}

// TestFieldNote applies the field-note lookup for a field name and value.
func TestFieldNote(fieldName string, rv reflect.Value) (string, bool) {
	return enrichField(fieldName, rv)
}

// TestLedgerKeySummary returns the ledger key summary for testing.
func TestLedgerKeySummary(key xdr.LedgerKey) (string, bool) {
	return ledgerKeySummary(key)
}

// TestScvalShortKey returns the short key representation for testing.
func TestScvalShortKey(v xdr.ScVal) string {
	return scvalShortKey(v)
}

// TestConfigSettingShortName returns the short config setting name for testing.
func TestConfigSettingShortName(id xdr.ConfigSettingId) string {
	return configSettingShortName(id)
}

// TestBuildConfigSettingEntry builds a minimal ConfigSettingEntry for testing.
// This is a convenience for tests that want to construct entries.
func TestBuildConfigSettingEntry(settingID xdr.ConfigSettingId, value interface{}) *xdr.ConfigSettingEntry {
	entry := &xdr.ConfigSettingEntry{
		ConfigSettingId: settingID,
	}
	// Note: The generated XDR types use pointer fields for optional arms.
	// We only support a few simple cases here for testing.
	switch settingID {
	case xdr.ConfigSettingIdConfigSettingContractMaxSizeBytes:
		if v, ok := value.(uint32); ok {
			u := xdr.Uint32(v)
			entry.ContractMaxSizeBytes = &u
		}
	case xdr.ConfigSettingIdConfigSettingContractDataKeySizeBytes:
		if v, ok := value.(uint32); ok {
			u := xdr.Uint32(v)
			entry.ContractDataKeySizeBytes = &u
		}
	case xdr.ConfigSettingIdConfigSettingContractDataEntrySizeBytes:
		if v, ok := value.(uint32); ok {
			u := xdr.Uint32(v)
			entry.ContractDataEntrySizeBytes = &u
		}
	}
	return entry
}

// TestMarshalBase64 marshals an XDR value to base64 for testing.
func TestMarshalBase64(v interface{ MarshalBinary() ([]byte, error) }) string {
	b, _ := v.MarshalBinary() //nolint:errcheck // test helper, error ignored
	return base64.StdEncoding.EncodeToString(b)
}
