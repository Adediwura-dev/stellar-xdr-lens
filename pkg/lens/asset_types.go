package lens

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// ErrNotAsset is returned when a value offered for asset inspection is not a
// classic Asset.
var ErrNotAsset = errors.New("value is not a classic asset")

// AssetDetails describes a classic Stellar asset together with the derived
// identifiers a caller needs to work with it. ContractID is the Stellar Asset
// Contract address of the asset on Network; it is deterministic, so the same
// asset on a different network yields a different contract address.
type AssetDetails struct {
	// Type is the XDR asset type: "native", "credit_alphanum4" or
	// "credit_alphanum12".
	Type string `json:"type"`
	// Code is the asset code, empty for the native asset.
	Code string `json:"code,omitempty"`
	// Issuer is the strkey account that issued the asset, empty for the
	// native asset.
	Issuer string `json:"issuer,omitempty"`
	// Canonical is the SEP-11 canonical form: "native" or "CODE:ISSUER".
	Canonical string `json:"canonical"`
	// Native reports whether the asset is the native lumen asset.
	Native bool `json:"native"`
	// ContractID is the strkey C... address of the asset's Stellar Asset
	// Contract on Network.
	ContractID string `json:"contractId"`
	// Network is the network passphrase the contract id was derived for.
	Network string `json:"network"`
}

// DescribeAsset decodes a base64-encoded classic Asset and derives its
// Stellar Asset Contract id for the supplied network passphrase.
func DescribeAsset(payload, networkPassphrase string) (*AssetDetails, error) {
	v, err := DecodeAs(payload, "Asset")
	if err != nil {
		return nil, fmt.Errorf("decoding classic asset: %w", err)
	}
	return DescribeAssetValue(v, networkPassphrase)
}

// DescribeAssetValue derives the details of a classic asset from an already
// decoded Value. It returns ErrNotAsset when v does not hold an xdr.Asset.
func DescribeAssetValue(v *Value, networkPassphrase string) (*AssetDetails, error) {
	if v == nil {
		return nil, ErrNotAsset
	}
	asset, ok := v.Raw.(*xdr.Asset)
	if !ok || asset == nil {
		return nil, fmt.Errorf("%w: got %s", ErrNotAsset, v.Type)
	}
	return describeAsset(*asset, networkPassphrase)
}

// SACContractID returns the strkey C... address of the Stellar Asset Contract
// for asset on the network identified by networkPassphrase.
//
// The address is derived by hashing the network id and the asset into a
// contract id preimage, so it can be computed offline and is stable for as
// long as the network passphrase is.
func SACContractID(asset xdr.Asset, networkPassphrase string) (string, error) {
	if networkPassphrase == "" {
		return "", errors.New("network passphrase must not be empty")
	}
	hash, err := asset.ContractID(networkPassphrase)
	if err != nil {
		return "", fmt.Errorf("deriving contract id: %w", err)
	}
	addr, err := strkey.Encode(strkey.VersionByteContract, hash[:])
	if err != nil {
		return "", fmt.Errorf("encoding contract id: %w", err)
	}
	return addr, nil
}

// describeAsset builds the details for an already-decoded asset. The SDK
// helpers used here dereference union arms without checking the discriminant,
// so the body runs behind a recover: a malformed value becomes an error rather
// than taking the process down.
func describeAsset(asset xdr.Asset, networkPassphrase string) (details *AssetDetails, err error) {
	defer func() {
		if r := recover(); r != nil {
			details, err = nil, fmt.Errorf("malformed asset: %v", r)
		}
	}()

	typeName, ok := xdr.AssetTypeToString[asset.Type]
	if !ok {
		return nil, fmt.Errorf("%w: unknown asset type %d", ErrNotAsset, asset.Type)
	}

	details = &AssetDetails{
		Type:      typeName,
		Canonical: asset.StringCanonical(),
		Native:    asset.IsNative(),
		Network:   networkPassphrase,
	}
	if !details.Native {
		details.Code = asset.GetCode()
		details.Issuer = asset.GetIssuer()
	}

	contractID, err := SACContractID(asset, networkPassphrase)
	if err != nil {
		return nil, err
	}
	details.ContractID = contractID
	return details, nil
}

func init() {
	// A contract id is a 32-byte hash on the wire; the strkey C... address is
	// the only form anyone recognises. Registering it here means every
	// contract id anywhere in a decoded tree, not just on Assets, renders
	// that way.
	enricherFor[reflect.TypeOf(xdr.ContractId{})] = enrichContractID
	// Soroban addresses switch between accounts, contracts and the other
	// addressable ledger entries. The SDK's String method already resolves
	// every arm to its strkey form.
	enricherFor[reflect.TypeOf(xdr.ScAddress{})] = enrichScAddress
}

// enrichContractID renders a contract id as its strkey C... address.
func enrichContractID(rv reflect.Value) (string, any, bool) {
	id, ok := rv.Interface().(xdr.ContractId)
	if !ok {
		return "", nil, false
	}
	addr, err := strkey.Encode(strkey.VersionByteContract, id[:])
	if err != nil {
		return "", nil, false
	}
	return "", addr, true
}

// enrichScAddress renders a Soroban address as its strkey form: G... for an
// account, C... for a contract, and the corresponding form for a muxed
// account, claimable balance or liquidity pool. An unresolvable arm declines
// and keeps the structural rendering.
func enrichScAddress(rv reflect.Value) (string, any, bool) {
	addr, ok := rv.Interface().(xdr.ScAddress)
	if !ok {
		return "", nil, false
	}
	display, err := addr.String()
	if err != nil || display == "" {
		return "", nil, false
	}
	return "", display, true
}
