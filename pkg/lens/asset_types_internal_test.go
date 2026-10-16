package lens

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"
)

const (
	zeroContractAddr = "CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABSC4"
	seqContractAddr  = "CAAACAQDAQCQMBYIBEFAWDANBYHRAEISCMKBKFQXDAMRUGY4DUPB6N4O"
	fillAccountAddr  = "GAIRCEIRCEIRCEIRCEIRCEIRCEIRCEIRCEIRCEIRCEIRCEIRCEIRCF6M"
)

// TestDescribeAssetMalformedValue covers the hostile-input path: the SDK
// accessors dereference union arms without checking the discriminant, and a
// credit asset without its arm must surface as an error, not a panic.
func TestDescribeAssetMalformedValue(t *testing.T) {
	t.Parallel()

	_, err := describeAsset(xdr.Asset{Type: xdr.AssetTypeAssetTypeCreditAlphanum4}, network.PublicNetworkPassphrase)
	if err == nil || !strings.Contains(err.Error(), "malformed asset") {
		t.Errorf("describeAsset(armless credit asset) error = %v, want a malformed-asset error", err)
	}
}

func TestDescribeAssetUnknownType(t *testing.T) {
	t.Parallel()

	_, err := describeAsset(xdr.Asset{Type: xdr.AssetType(9)}, network.PublicNetworkPassphrase)
	if !errors.Is(err, ErrNotAsset) {
		t.Errorf("describeAsset(unknown type) error = %v, want ErrNotAsset", err)
	}
}

func TestEnrichContractID(t *testing.T) {
	t.Parallel()

	var id xdr.ContractId
	note, raw, ok := enrichContractID(reflect.ValueOf(id))
	if !ok {
		t.Fatal("enrichContractID() declined an all-zero contract id")
	}
	if note != "" || raw != zeroContractAddr {
		t.Errorf("enrichContractID() = (%q, %v), want (\"\", %q)", note, raw, zeroContractAddr)
	}

	var seq xdr.ContractId
	for i := range seq {
		seq[i] = byte(i)
	}
	if _, raw, ok := enrichContractID(reflect.ValueOf(seq)); !ok || raw != seqContractAddr {
		t.Errorf("enrichContractID(seq) = (_, %v, %v), want %q", raw, ok, seqContractAddr)
	}

	if _, _, ok := enrichContractID(reflect.ValueOf(xdr.Auth{})); ok {
		t.Error("enrichContractID(Auth) succeeded, want ok=false")
	}
}

func TestEnrichScAddress(t *testing.T) {
	t.Parallel()

	var key xdr.Uint256
	for i := range key {
		key[i] = 0x11
	}
	account := xdr.ScAddress{
		Type:      xdr.ScAddressTypeScAddressTypeAccount,
		AccountId: &xdr.AccountId{Type: xdr.PublicKeyTypePublicKeyTypeEd25519, Ed25519: &key},
	}
	if note, raw, ok := enrichScAddress(reflect.ValueOf(account)); !ok || note != "" || raw != fillAccountAddr {
		t.Errorf("enrichScAddress(account) = (%q, %v, %v), want (\"\", %q, true)", note, raw, ok, fillAccountAddr)
	}

	contract := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &xdr.ContractId{},
	}
	if note, raw, ok := enrichScAddress(reflect.ValueOf(contract)); !ok || note != "" || raw != zeroContractAddr {
		t.Errorf("enrichScAddress(contract) = (%q, %v, %v), want (\"\", %q, true)", note, raw, ok, zeroContractAddr)
	}

	if _, _, ok := enrichScAddress(reflect.ValueOf(xdr.Auth{})); ok {
		t.Error("enrichScAddress(Auth) succeeded, want ok=false")
	}

	// A populated discriminant with a missing arm panics inside the SDK's
	// String method; the registry wrapper must recover and decline.
	malformed := xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount}
	if _, _, ok := enrich(reflect.ValueOf(malformed)); ok {
		t.Error("enrich(malformed ScAddress) succeeded, want ok=false")
	}
}
