package lens

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
)

func init() {
	// Opaque ledger keys carried by frozen/freeze-bypass config settings.
	// EncodedLedgerKey is defined as `opaque <>`, but the protocol guarantees
	// the bytes are a valid LedgerKey. Decoding them recovers the actual key
	// structure (account, trustline, offer, data, contract data, etc.) which
	// is far more useful than a 64-byte hex blob.
	enricherFor[reflect.TypeOf(xdr.EncodedLedgerKey{})] = enrichEncodedLedgerKey

	// Ledger-count TTL fields in StateArchivalSettings. The protocol stores
	// TTLs as a ledger count; at the default 5s target close time, one ledger
	// ≈ 5 seconds. We render an approximate wall-clock duration alongside the
	// raw count so developers can immediately see the practical meaning.
	// SamplePeriod is also ledger-counted; the same note applies.
	fieldNotes["MaxEntryTtl"] = ledgerCountNote
	fieldNotes["MinTemporaryTtl"] = ledgerCountNote
	fieldNotes["MinPersistentTtl"] = ledgerCountNote
	fieldNotes["LiveSorobanStateSizeWindowSamplePeriod"] = ledgerCountNote
}

// enrichEncodedLedgerKey decodes the opaque EncodedLedgerKey bytes as a
// LedgerKey and produces a concise one-line summary. The raw hex is kept as
// the machine value; the decoded summary becomes the note. Anything that does
// not decode cleanly declines, falling back to the generic hex rendering.
func enrichEncodedLedgerKey(rv reflect.Value) (string, any, bool) {
	ek, valid := rv.Interface().(xdr.EncodedLedgerKey)
	if !valid || len(ek) == 0 {
		return "", nil, false
	}
	var key xdr.LedgerKey
	if err := xdr.SafeUnmarshal(ek, &key); err != nil {
		return "", nil, false
	}
	summary, ok := ledgerKeySummary(key)
	if !ok {
		return "", nil, false
	}
	return summary, hex.EncodeToString(ek), true
}

// ledgerKeySummary renders a LedgerKey as a single human-readable line.
// It covers all current LedgerEntryType arms. For keys that contain
// addresses or hashes, the identifying value is included.
func ledgerKeySummary(key xdr.LedgerKey) (string, bool) {
	switch key.Type {
	case xdr.LedgerEntryTypeAccount:
		if key.Account == nil {
			return "", false
		}
		addr, err := key.Account.AccountId.GetAddress()
		if err != nil {
			return "", false
		}
		return "account key: " + addr, true

	case xdr.LedgerEntryTypeTrustline:
		if key.TrustLine == nil {
			return "", false
		}
		addr, err := key.TrustLine.AccountId.GetAddress()
		if err != nil {
			return "", false
		}
		assetStr := key.TrustLine.Asset.ToAsset().StringCanonical()
		return "trustline key: " + addr + " for " + assetStr, true

	case xdr.LedgerEntryTypeOffer:
		if key.Offer == nil {
			return "", false
		}
		addr, err := key.Offer.SellerId.GetAddress()
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("offer key: #%d by %s", key.Offer.OfferId, addr), true

	case xdr.LedgerEntryTypeData:
		if key.Data == nil {
			return "", false
		}
		addr, err := key.Data.AccountId.GetAddress()
		if err != nil {
			return "", false
		}
		// DataName is a string; use %q to show special characters safely.
		return fmt.Sprintf("data entry key: %q for %s", key.Data.DataName, addr), true

	case xdr.LedgerEntryTypeClaimableBalance:
		if key.ClaimableBalance == nil {
			return "", false
		}
		// BalanceId is a union; v0 carries a Hash.
		var h xdr.Hash
		if key.ClaimableBalance.BalanceId.Type == xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0 &&
			key.ClaimableBalance.BalanceId.V0 != nil {
			h = *key.ClaimableBalance.BalanceId.V0
		} else {
			return "", false
		}
		return "claimable balance key: " + hex.EncodeToString(h[:]), true

	case xdr.LedgerEntryTypeLiquidityPool:
		if key.LiquidityPool == nil {
			return "", false
		}
		// PoolId is a Hash; render as hex prefix (full 64 chars is verbose).
		pid := hex.EncodeToString(key.LiquidityPool.LiquidityPoolId[:])
		return "liquidity pool key: " + pid[:16] + "…", true

	case xdr.LedgerEntryTypeContractData:
		if key.ContractData == nil {
			return "", false
		}
		contractStr, err := key.ContractData.Contract.String()
		if err != nil {
			return "", false
		}
		// The ScVal key is the distinguishing part of a contract data key.
		// Render a compact form: kind + short key representation.
		keyStr := scvalShortKey(key.ContractData.Key)
		return "contract data key: " + contractStr + " " + keyStr, true

	case xdr.LedgerEntryTypeContractCode:
		if key.ContractCode == nil {
			return "", false
		}
		hash := hex.EncodeToString(key.ContractCode.Hash[:])
		return "contract code key: " + hash[:16] + "…", true

	case xdr.LedgerEntryTypeConfigSetting:
		if key.ConfigSetting == nil {
			return "", false
		}
		id := key.ConfigSetting.ConfigSettingId
		return "config setting key: " + configSettingShortName(id), true

	case xdr.LedgerEntryTypeTtl:
		if key.Ttl == nil {
			return "", false
		}
		kh := hex.EncodeToString(key.Ttl.KeyHash[:])
		return "ttl key: " + kh[:16] + "…", true

	default:
		return "", false
	}
}

// scvalShortKey produces a compact one-line representation of a contract
// data key ScVal. It handles the common cases: ledger-key-contract-instance,
// symbol, string, bytes, and address. Other types fall back to the SDK's
// String() which is already concise for primitives.
func scvalShortKey(v xdr.ScVal) string {
	switch v.Type {
	case xdr.ScValTypeScvLedgerKeyContractInstance:
		return "(instance)"
	case xdr.ScValTypeScvSymbol:
		return "symbol:" + string(*v.Sym)
	case xdr.ScValTypeScvString:
		// Contract data keys are often symbols; strings may contain arbitrary
		// bytes. Quote and truncate if long.
		s := string(*v.Str)
		if len(s) > 20 {
			s = s[:20] + "…"
		}
		return "string:" + strconv.Quote(s)
	case xdr.ScValTypeScvBytes:
		b := *v.Bytes
		if len(b) > 12 {
			return "bytes:" + hex.EncodeToString(b[:12]) + "…"
		}
		return "bytes:" + hex.EncodeToString(b)
	case xdr.ScValTypeScvAddress:
		// ScAddress contains a type and optional hash. Check validity.
		if v.Address.Type == xdr.ScAddressTypeScAddressTypeContract &&
			v.Address.ContractId != nil {
			addr, err := v.Address.String()
			if err != nil {
				return "address:<error>"
			}
			return "address:" + addr
		}
		return "address:<invalid>"
	default:
		// For U32, I32, U64, I64, U128, I128, etc., the SDK String() is fine.
		return v.String()
	}
}

// configSettingShortName maps a ConfigSettingId to a concise human name.
// The generated String() includes the full "ConfigSettingIdConfigSetting..."
// prefix; this map strips that noise.
func configSettingShortName(id xdr.ConfigSettingId) string {
	switch id {
	case xdr.ConfigSettingIdConfigSettingContractMaxSizeBytes:
		return "contract max size bytes"
	case xdr.ConfigSettingIdConfigSettingContractComputeV0:
		return "contract compute V0"
	case xdr.ConfigSettingIdConfigSettingContractLedgerCostV0:
		return "contract ledger cost V0"
	case xdr.ConfigSettingIdConfigSettingContractHistoricalDataV0:
		return "contract historical data V0"
	case xdr.ConfigSettingIdConfigSettingContractEventsV0:
		return "contract events V0"
	case xdr.ConfigSettingIdConfigSettingContractBandwidthV0:
		return "contract bandwidth V0"
	case xdr.ConfigSettingIdConfigSettingContractCostParamsCpuInstructions:
		return "contract cost params CPU instructions"
	case xdr.ConfigSettingIdConfigSettingContractCostParamsMemoryBytes:
		return "contract cost params memory bytes"
	case xdr.ConfigSettingIdConfigSettingContractDataKeySizeBytes:
		return "contract data key size bytes"
	case xdr.ConfigSettingIdConfigSettingContractDataEntrySizeBytes:
		return "contract data entry size bytes"
	case xdr.ConfigSettingIdConfigSettingStateArchival:
		return "state archival"
	case xdr.ConfigSettingIdConfigSettingContractExecutionLanes:
		return "contract execution lanes"
	case xdr.ConfigSettingIdConfigSettingLiveSorobanStateSizeWindow:
		return "live soroban state size window"
	case xdr.ConfigSettingIdConfigSettingEvictionIterator:
		return "eviction iterator"
	case xdr.ConfigSettingIdConfigSettingContractParallelComputeV0:
		return "contract parallel compute V0"
	case xdr.ConfigSettingIdConfigSettingContractLedgerCostExtV0:
		return "contract ledger cost ext V0"
	case xdr.ConfigSettingIdConfigSettingScpTiming:
		return "SCP timing"
	case xdr.ConfigSettingIdConfigSettingFrozenLedgerKeys:
		return "frozen ledger keys"
	case xdr.ConfigSettingIdConfigSettingFrozenLedgerKeysDelta:
		return "frozen ledger keys delta"
	case xdr.ConfigSettingIdConfigSettingFreezeBypassTxs:
		return "freeze bypass txs"
	case xdr.ConfigSettingIdConfigSettingFreezeBypassTxsDelta:
		return "freeze bypass txs delta"
	default:
		return fmt.Sprintf("unknown(%d)", id)
	}
}

// ledgerCountNote renders a ledger-count field (Uint32) as an approximate
// wall-clock duration at the protocol's default 5-second target close time.
// The note uses "≈" and states the assumption explicitly.
func ledgerCountNote(rv reflect.Value) (string, bool) {
	v, ok := rv.Interface().(xdr.Uint32)
	if !ok {
		return "", false
	}
	ledgers := uint64(v)
	seconds := ledgers * 5 // 5s per ledger (default target close time)
	d := time.Duration(seconds) * time.Second

	var human string
	if d >= 24*time.Hour {
		days := float64(d) / (24 * float64(time.Hour))
		human = fmt.Sprintf("≈%.1f days", days)
	} else {
		human = "≈" + d.String()
	}
	return human + " (at 5s per ledger)", true
}
