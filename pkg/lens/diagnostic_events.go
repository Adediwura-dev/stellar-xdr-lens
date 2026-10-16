package lens

import (
	"reflect"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// Diagnostic and contract events emitted during Soroban execution.
//
// ContractEvent.ContractId and the contract arm of ScAddress are raw 32-byte
// hashes in XDR. Wallets and explorers show them as C... strkeys, so the
// enrichers registered here render them the same way. ScAddress covers the
// account and contract arms at once, which makes event topics readable
// without collapsing the rest of the event: topics and data stay structured
// ScVal trees.

func init() {
	registerEnricher(reflect.TypeOf(xdr.ContractId{}), enrichContractID)
	registerEnricher(reflect.TypeOf(xdr.ScAddress{}), enrichScAddress)
}

// enrichContractID renders a contract hash as its C... strkey.
func enrichContractID(rv reflect.Value) (string, any, bool) {
	id, valid := rv.Interface().(xdr.ContractId)
	if !valid {
		return "", nil, false
	}
	str, err := strkey.Encode(strkey.VersionByteContract, id[:])
	if err != nil || str == "" {
		return "", nil, false
	}
	return "", str, true
}

// enrichScAddress renders any Soroban address arm — account or contract — as
// its strkey form, which is what the SDK's own String method produces.
func enrichScAddress(rv reflect.Value) (string, any, bool) {
	addr, valid := rv.Interface().(xdr.ScAddress)
	if !valid {
		return "", nil, false
	}
	str, err := addr.String()
	if err != nil || str == "" {
		return "", nil, false
	}
	return "", str, true
}
