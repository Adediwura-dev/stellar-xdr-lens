package lens

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func TestTrustlineAssetNameVariants(t *testing.T) {
	t.Parallel()

	issuerKey := xdr.Uint256{}
	issuerKey[0] = 42
	issuerID := xdr.AccountId{Type: xdr.PublicKeyTypePublicKeyTypeEd25519, Ed25519: &issuerKey}
	issuer, err := issuerID.GetAddress()
	if err != nil {
		t.Fatalf("GetAddress() error = %v", err)
	}
	poolID := xdr.PoolId{}
	poolID[0] = 5

	tests := []struct {
		name  string
		asset xdr.TrustLineAsset
		want  string
		bad   bool
	}{
		{"native", xdr.TrustLineAsset{Type: xdr.AssetTypeAssetTypeNative}, "XLM", false},
		{"credit alphanum4", xdr.MustNewCreditAsset("USD", issuer).ToTrustLineAsset(), "USD:" + issuer, false},
		{"credit alphanum12", xdr.MustNewCreditAsset("ABCDEFGHIJKL", issuer).ToTrustLineAsset(), "ABCDEFGHIJKL:" + issuer, false},
		{"pool share", xdr.TrustLineAsset{Type: xdr.AssetTypeAssetTypePoolShare, LiquidityPoolId: &poolID}, "pool:" + hex.EncodeToString(poolID[:]), false},
		{"missing credit arm", xdr.TrustLineAsset{Type: xdr.AssetTypeAssetTypeCreditAlphanum4}, "", true},
		{"unknown type", xdr.TrustLineAsset{Type: xdr.AssetType(99)}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := trustlineAssetName(tc.asset)
			if tc.bad {
				if !errors.Is(err, ErrInvalidArgument) {
					t.Errorf("trustlineAssetName() error = %v, want ErrInvalidArgument", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("trustlineAssetName() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestClassicAssetNameVariants(t *testing.T) {
	t.Parallel()

	issuerKey := xdr.Uint256{}
	issuerKey[0] = 43
	issuerID := xdr.AccountId{Type: xdr.PublicKeyTypePublicKeyTypeEd25519, Ed25519: &issuerKey}
	issuer, err := issuerID.GetAddress()
	if err != nil {
		t.Fatalf("GetAddress() error = %v", err)
	}

	tests := []struct {
		name  string
		asset xdr.Asset
		want  string
		bad   bool
	}{
		{"native", xdr.MustNewNativeAsset(), "XLM", false},
		{"credit alphanum4", xdr.MustNewCreditAsset("USD", issuer), "USD:" + issuer, false},
		{"credit alphanum12", xdr.MustNewCreditAsset("ABCDEFGHIJKL", issuer), "ABCDEFGHIJKL:" + issuer, false},
		{"missing credit arm", xdr.Asset{Type: xdr.AssetTypeAssetTypeCreditAlphanum4}, "", true},
		{"unknown type", xdr.Asset{Type: xdr.AssetType(99)}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := classicAssetName(tc.asset)
			if tc.bad {
				if !errors.Is(err, ErrInvalidArgument) {
					t.Errorf("classicAssetName() error = %v, want ErrInvalidArgument", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("classicAssetName() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestBalanceFromMalformedEntryReturnsClassifiedError(t *testing.T) {
	t.Parallel()

	_, _, err := balanceFromEntry(&xdr.LedgerEntry{
		Data: xdr.LedgerEntryData{Type: xdr.LedgerEntryTypeAccount},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("balanceFromEntry() error = %v, want ErrInvalidArgument", err)
	}
}
