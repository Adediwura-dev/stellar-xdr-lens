package lens

import (
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func init() {
	enricherFor[reflect.TypeOf(xdr.UpgradeType{})] = enrichUpgradeType
}

// enrichUpgradeType decodes the opaque vote carried by an UpgradeType. The
// protocol deliberately keeps these bytes untyped — StellarValue.upgrades is
// "a vector of encoded LedgerUpgrade so that nodes can drop unknown steps
// during consensus" — which is exactly why a decoder helps: a validator's
// upgrade vote shows up in a ledger header as an unreadable hex blob.
//
// The hex form is kept as the machine value and the decoded vote becomes the
// note, so JSON output stays byte-exact while the tree names the vote.
// Anything that does not decode cleanly as a LedgerUpgrade declines, and the
// generic hex rendering takes over.
func enrichUpgradeType(rv reflect.Value) (display string, raw any, ok bool) {
	vote, valid := rv.Interface().(xdr.UpgradeType)
	if !valid || len(vote) == 0 {
		return "", nil, false
	}
	var upgrade xdr.LedgerUpgrade
	if err := xdr.SafeUnmarshal(vote, &upgrade); err != nil {
		return "", nil, false
	}
	summary, ok := ledgerUpgradeSummary(upgrade)
	if !ok {
		return "", nil, false
	}
	return summary, hex.EncodeToString(vote), true
}

// ledgerUpgradeSummary renders a LedgerUpgrade as one self-describing line,
// since a vote's type is as important as its value: "protocol version 21"
// reads without the surrounding union. Unknown types and absent arms decline
// so callers can fall back rather than print something misleading.
func ledgerUpgradeSummary(upgrade xdr.LedgerUpgrade) (string, bool) {
	switch upgrade.Type {
	case xdr.LedgerUpgradeTypeLedgerUpgradeVersion:
		if upgrade.NewLedgerVersion == nil {
			return "", false
		}
		return fmt.Sprintf("protocol version %d", *upgrade.NewLedgerVersion), true

	case xdr.LedgerUpgradeTypeLedgerUpgradeBaseFee:
		if upgrade.NewBaseFee == nil {
			return "", false
		}
		return fmt.Sprintf("base fee %d stroops (%s XLM)",
			*upgrade.NewBaseFee, amount.String(xdr.Int64(*upgrade.NewBaseFee))), true

	case xdr.LedgerUpgradeTypeLedgerUpgradeMaxTxSetSize:
		if upgrade.NewMaxTxSetSize == nil {
			return "", false
		}
		return fmt.Sprintf("max tx set size %d", *upgrade.NewMaxTxSetSize), true

	case xdr.LedgerUpgradeTypeLedgerUpgradeBaseReserve:
		if upgrade.NewBaseReserve == nil {
			return "", false
		}
		return fmt.Sprintf("base reserve %d stroops (%s XLM)",
			*upgrade.NewBaseReserve, amount.String(xdr.Int64(*upgrade.NewBaseReserve))), true

	case xdr.LedgerUpgradeTypeLedgerUpgradeFlags:
		if upgrade.NewFlags == nil {
			return "", false
		}
		return fmt.Sprintf("ledger flags %d", *upgrade.NewFlags), true

	case xdr.LedgerUpgradeTypeLedgerUpgradeConfig:
		if upgrade.NewConfig == nil {
			return "", false
		}
		contract, err := contractIDString(upgrade.NewConfig.ContractId)
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("config upgrade set: contract %s, content hash %s",
			contract, upgrade.NewConfig.ContentHash.HexString()), true

	case xdr.LedgerUpgradeTypeLedgerUpgradeMaxSorobanTxSetSize:
		if upgrade.NewMaxSorobanTxSetSize == nil {
			return "", false
		}
		return fmt.Sprintf("max Soroban tx set size %d", *upgrade.NewMaxSorobanTxSetSize), true

	default:
		return "", false
	}
}

// contractIDString renders a contract id as its C... strkey through the SDK's
// ScAddress helper, the same one `lens explain` uses for contract addresses,
// so the two renderings cannot drift apart.
func contractIDString(id xdr.ContractId) (string, error) {
	address := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &id,
	}
	return address.String()
}
