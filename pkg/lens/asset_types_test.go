package lens_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

const (
	usdcIssuer = "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"

	// Well-known Stellar Asset Contract ids, independently verifiable from
	// any Stellar SDK or the soroban-examples contract deployments.
	nativeSACMainnet = "CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA"
	nativeSACTestnet = "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"
	usdcSACMainnet   = "CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75"
)

func TestDescribeAssetNative(t *testing.T) {
	t.Parallel()

	details, err := lens.DescribeAsset(loadFixture(t, "asset_native"), network.PublicNetworkPassphrase)
	if err != nil {
		t.Fatalf("DescribeAsset() error = %v", err)
	}
	if details.Type != "native" || !details.Native {
		t.Errorf("asset = %q native=%v, want native", details.Type, details.Native)
	}
	if details.Code != "" || details.Issuer != "" {
		t.Errorf("code/issuer = %q/%q, want empty for the native asset", details.Code, details.Issuer)
	}
	if details.Canonical != "native" {
		t.Errorf("Canonical = %q, want native", details.Canonical)
	}
	if details.ContractID != nativeSACMainnet {
		t.Errorf("ContractID = %q, want %q", details.ContractID, nativeSACMainnet)
	}
	if details.Network != network.PublicNetworkPassphrase {
		t.Errorf("Network = %q, want the mainnet passphrase", details.Network)
	}
}

func TestDescribeAssetUSDC(t *testing.T) {
	t.Parallel()

	details, err := lens.DescribeAsset(loadFixture(t, "asset_usdc"), network.PublicNetworkPassphrase)
	if err != nil {
		t.Fatalf("DescribeAsset() error = %v", err)
	}
	if details.Type != "credit_alphanum4" || details.Native {
		t.Errorf("asset = %q native=%v, want a credit asset", details.Type, details.Native)
	}
	if details.Code != "USDC" || details.Issuer != usdcIssuer {
		t.Errorf("code/issuer = %q/%q, want USDC/%s", details.Code, details.Issuer, usdcIssuer)
	}
	if want := "USDC:" + usdcIssuer; details.Canonical != want {
		t.Errorf("Canonical = %q, want %q", details.Canonical, want)
	}
	if details.ContractID != usdcSACMainnet {
		t.Errorf("ContractID = %q, want %q", details.ContractID, usdcSACMainnet)
	}
}

// TestSACContractIDIsNetworkSpecific pins the property that makes the offline
// derivation useful: the same asset yields a different contract on each
// network, because the network id is part of the hash preimage.
func TestSACContractIDIsNetworkSpecific(t *testing.T) {
	t.Parallel()

	native := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}

	mainnet, err := lens.SACContractID(native, network.PublicNetworkPassphrase)
	if err != nil {
		t.Fatalf("SACContractID(mainnet) error = %v", err)
	}
	testnet, err := lens.SACContractID(native, network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("SACContractID(testnet) error = %v", err)
	}
	if mainnet != nativeSACMainnet || testnet != nativeSACTestnet {
		t.Errorf("SAC ids = %q/%q, want %q/%q", mainnet, testnet, nativeSACMainnet, nativeSACTestnet)
	}

	// The details struct must report the same id the public helper derives.
	details, err := lens.DescribeAsset(loadFixture(t, "asset_native"), network.TestNetworkPassphrase)
	if err != nil {
		t.Fatalf("DescribeAsset() error = %v", err)
	}
	if details.ContractID != testnet {
		t.Errorf("DescribeAsset(testnet).ContractID = %q, want %q", details.ContractID, testnet)
	}
}

func TestSACContractIDRequiresPassphrase(t *testing.T) {
	t.Parallel()

	if _, err := lens.SACContractID(xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}, ""); err == nil {
		t.Error("SACContractID(\"\") error = nil, want an error")
	}
}

func TestDescribeAssetRejectsNonAssets(t *testing.T) {
	t.Parallel()

	// The hello fixture decodes as Hello; asking for an Asset must fail
	// rather than reinterpret the bytes.
	if _, err := lens.DescribeAsset(loadFixture(t, "overlay_hello"), network.PublicNetworkPassphrase); err == nil {
		t.Error("DescribeAsset(Hello payload) error = nil, want an error")
	}
	if _, err := lens.DescribeAsset("not-base64", network.PublicNetworkPassphrase); err == nil {
		t.Error("DescribeAsset(invalid base64) error = nil, want an error")
	}
	if _, err := lens.DescribeAsset(loadFixture(t, "asset_native"), ""); err == nil ||
		!strings.Contains(err.Error(), "network passphrase") {
		t.Errorf("DescribeAsset(empty passphrase) error = %v, want a passphrase error", err)
	}
}

func TestDescribeAssetValueRejectsNonAssets(t *testing.T) {
	t.Parallel()

	if _, err := lens.DescribeAssetValue(nil, network.PublicNetworkPassphrase); !errors.Is(err, lens.ErrNotAsset) {
		t.Errorf("DescribeAssetValue(nil) error = %v, want ErrNotAsset", err)
	}

	v, err := lens.DecodeAs(loadFixture(t, "overlay_auth_cert"), "AuthCert")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if _, err := lens.DescribeAssetValue(v, network.PublicNetworkPassphrase); !errors.Is(err, lens.ErrNotAsset) {
		t.Errorf("DescribeAssetValue(AuthCert) error = %v, want ErrNotAsset", err)
	}
}
