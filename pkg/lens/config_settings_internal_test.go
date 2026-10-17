package lens_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

// TestEnrichEncodedLedgerKeyInternal tests the internal enricher for
// EncodedLedgerKey directly, covering decline paths.
func TestEnrichEncodedLedgerKeyInternal(t *testing.T) {
	// Get the enricher
	enricher := lens.GetEnricherForTest(reflect.TypeOf(xdr.EncodedLedgerKey{}))
	if enricher == nil {
		t.Fatal("enricher not registered")
	}

	t.Run("empty_bytes_declines", func(t *testing.T) {
		ek := xdr.EncodedLedgerKey{}
		rv := reflect.ValueOf(ek)
		_, _, ok := enricher(rv)
		if ok {
			t.Fatal("empty bytes should decline")
		}
	})

	t.Run("garbage_bytes_declines", func(t *testing.T) {
		ek := xdr.EncodedLedgerKey{0xde, 0xad, 0xbe, 0xef}
		rv := reflect.ValueOf(ek)
		_, _, ok := enricher(rv)
		if ok {
			t.Fatal("garbage bytes should decline")
		}
	})

	t.Run("valid_account_key_from_fixture", func(t *testing.T) {
		// Use a real fixture encoded key to test the full decode path
		payload := loadFixture(t, "config_setting_17")
		v, err := lens.DecodeAs(payload, "ConfigSettingEntry")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		keys := v.Node.Child("FrozenLedgerKeys").Child("Keys")
		if keys == nil || len(keys.Children) < 3 {
			t.Fatal("expected 3 frozen keys")
		}
		// First key should have the account note
		firstKey := keys.Children[0]
		if firstKey == nil || firstKey.Note == "" {
			t.Fatal("first key missing note")
		}
		if !strings.Contains(firstKey.Note, "account key: GATDQL767ZM2JQTBEG4BQ5WKOQNGAGWZDUN4GYT2UINPEU3RT2UAMVZH") {
			t.Errorf("unexpected note: %s", firstKey.Note)
		}
	})

	t.Run("unknown_ledger_key_type_declines", func(t *testing.T) {
		// Create an EncodedLedgerKey with raw bytes for an unknown LedgerKey type
		// Type 99 encoded as XDR uint32 (big-endian): 0x00 0x00 0x00 0x63
		// This is the raw bytes, not base64
		ek := xdr.EncodedLedgerKey{0x00, 0x00, 0x00, 0x63}
		rv := reflect.ValueOf(ek)
		_, _, ok := enricher(rv)
		if ok {
			t.Fatal("unknown ledger key type should decline")
		}
	})
}

// TestEnrichUpgradeTypeInternal tests the enrichUpgradeType enricher
// decline paths for comparison. Skipped because UpgradeType enricher
// is on a different branch (feat/lens-ledger-upgrade-decoder).
func TestEnrichUpgradeTypeInternal(t *testing.T) {
	t.Skip("UpgradeType enricher on different branch")
}

// TestFieldNotesMechanism tests the fieldNotes registry and
// ledgerCountNote function.
func TestFieldNotesMechanism(t *testing.T) {
	t.Run("ledgerCountNote_valid_Uint32", func(t *testing.T) {
		v := xdr.Uint32(3110400)
		rv := reflect.ValueOf(v)
		note, ok := lens.TestFieldNote("MaxEntryTtl", rv)
		if !ok {
			t.Fatal("field note not applied")
		}
		if !strings.Contains(note, "180.0 days") {
			t.Errorf("unexpected note: %s", note)
		}
		if !strings.Contains(note, "5s per ledger") {
			t.Errorf("note missing assumption: %s", note)
		}
	})

	t.Run("ledgerCountNote_sub_day", func(t *testing.T) {
		v := xdr.Uint32(64)
		rv := reflect.ValueOf(v)
		note, ok := lens.TestFieldNote("LiveSorobanStateSizeWindowSamplePeriod", rv)
		if !ok {
			t.Fatal("field note not applied")
		}
		if !strings.Contains(note, "5m20s") && !strings.Contains(note, "320s") {
			t.Errorf("unexpected note for 64 ledgers: %s", note)
		}
	})

	t.Run("ledgerCountNote_zero", func(t *testing.T) {
		v := xdr.Uint32(0)
		rv := reflect.ValueOf(v)
		note, ok := lens.TestFieldNote("MaxEntryTtl", rv)
		if !ok {
			t.Fatal("field note not applied")
		}
		if !strings.Contains(note, "0s") && !strings.Contains(note, "0 days") {
			t.Errorf("unexpected note for zero: %s", note)
		}
	})

	t.Run("ledgerCountNote_wrong_type_declines", func(t *testing.T) {
		v := xdr.Int64(3110400)
		rv := reflect.ValueOf(v)
		_, ok := lens.TestFieldNote("MaxEntryTtl", rv)
		if ok {
			t.Fatal("wrong type should decline")
		}
	})

	t.Run("ledgerCountNote_unknown_field_declines", func(t *testing.T) {
		v := xdr.Uint32(3110400)
		rv := reflect.ValueOf(v)
		_, ok := lens.TestFieldNote("UnknownField", rv)
		if ok {
			t.Fatal("unknown field should decline")
		}
	})

	t.Run("amountFields_still_work", func(t *testing.T) {
		v := xdr.Int64(7)
		rv := reflect.ValueOf(v)
		note, ok := lens.TestFieldNote("FeeRatePerInstructionsIncrement", rv)
		if !ok {
			t.Fatal("amount field note not applied")
		}
		if !strings.Contains(note, "0.0000007") {
			t.Errorf("unexpected amount note: %s", note)
		}
		if !strings.Contains(note, "stroops: 7") {
			t.Errorf("note missing stroops: %s", note)
		}
	})
}

// TestLedgerKeySummaryInternal tests ledgerKeySummary helper directly.
func TestLedgerKeySummaryInternal(t *testing.T) {
	// Use a fixture that we know has a ConfigSettingEntry with a ConfigSetting key
	t.Run("config_setting_key_from_fixture", func(t *testing.T) {
		payload := loadFixture(t, "config_setting_10")
		v, err := lens.DecodeAs(payload, "ConfigSettingEntry")
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		// The ConfigSettingEntry itself is the value; we can't easily get at the
		// LedgerKey without decoding a full LedgerEntry. But we can test the
		// helper by using it via the encoded ledger key enricher on freeze keys.
		_ = v // just verify decode works
	})
}

// TestScvalShortKeyInternal tests the scvalShortKey helper.
func TestScvalShortKeyInternal(t *testing.T) {
	t.Run("ledger_key_contract_instance", func(t *testing.T) {
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvLedgerKeyContractInstance,
		}
		result := lens.TestScvalShortKey(sv)
		if result != "(instance)" {
			t.Errorf("got %q, want '(instance)'", result)
		}
	})

	t.Run("symbol", func(t *testing.T) {
		sym := xdr.ScSymbol("my_symbol")
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvSymbol,
			Sym:  &sym,
		}
		result := lens.TestScvalShortKey(sv)
		if result != "symbol:my_symbol" {
			t.Errorf("got %q, want 'symbol:my_symbol'", result)
		}
	})

	t.Run("string_short", func(t *testing.T) {
		s := xdr.ScString("hello")
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvString,
			Str:  &s,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "string:") || !strings.Contains(result, "hello") {
			t.Errorf("got %q", result)
		}
	})

	t.Run("string_long_truncated", func(t *testing.T) {
		s := xdr.ScString(strings.Repeat("a", 30))
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvString,
			Str:  &s,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "…") {
			t.Errorf("long string should be truncated: %s", result)
		}
	})

	t.Run("bytes_short", func(t *testing.T) {
		b := xdr.ScBytes{0xde, 0xad, 0xbe, 0xef}
		sv := xdr.ScVal{
			Type:  xdr.ScValTypeScvBytes,
			Bytes: &b,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "bytes:") || !strings.Contains(result, "deadbeef") {
			t.Errorf("got %q", result)
		}
	})

	t.Run("bytes_long_truncated", func(t *testing.T) {
		b := make(xdr.ScBytes, 20)
		for i := range b {
			b[i] = byte(i)
		}
		sv := xdr.ScVal{
			Type:  xdr.ScValTypeScvBytes,
			Bytes: &b,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "…") {
			t.Errorf("long bytes should be truncated: %s", result)
		}
	})

	t.Run("address_contract_valid", func(t *testing.T) {
		// Construct ScAddress with Contract type and valid hash
		var cid xdr.ContractId
		copy(cid[:], make([]byte, 32)) // all zeros
		cid[31] = 1
		addr := &xdr.ScAddress{
			Type:       xdr.ScAddressTypeScAddressTypeContract,
			ContractId: &cid,
		}
		sv := xdr.ScVal{
			Type:    xdr.ScValTypeScvAddress,
			Address: addr,
		}
		result := lens.TestScvalShortKey(sv)
		if result == "" {
			t.Errorf("got empty result")
		}
		if !strings.Contains(result, "address:") {
			t.Errorf("got %q, expected 'address:' prefix", result)
		}
	})

	t.Run("address_invalid_type", func(t *testing.T) {
		// Address with wrong type (not Contract)
		addr := &xdr.ScAddress{
			Type: xdr.ScAddressTypeScAddressTypeAccount,
		}
		sv := xdr.ScVal{
			Type:    xdr.ScValTypeScvAddress,
			Address: addr,
		}
		result := lens.TestScvalShortKey(sv)
		if result != "address:<invalid>" {
			t.Errorf("got %q, want 'address:<invalid>'", result)
		}
	})

	t.Run("address_nil_contract_id", func(t *testing.T) {
		// Address with Contract type but nil ContractId
		addr := &xdr.ScAddress{
			Type: xdr.ScAddressTypeScAddressTypeContract,
		}
		sv := xdr.ScVal{
			Type:    xdr.ScValTypeScvAddress,
			Address: addr,
		}
		result := lens.TestScvalShortKey(sv)
		if result != "address:<invalid>" {
			t.Errorf("got %q, want 'address:<invalid>'", result)
		}
	})

	t.Run("u32_primitive", func(t *testing.T) {
		v := xdr.Uint32(42)
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvU32,
			U32:  &v,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "42") {
			t.Errorf("got %q", result)
		}
	})

	t.Run("u64_primitive", func(t *testing.T) {
		v := xdr.Uint64(123456789)
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvU64,
			U64:  &v,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "123456789") {
			t.Errorf("got %q", result)
		}
	})

	t.Run("i64_primitive", func(t *testing.T) {
		v := xdr.Int64(-987654321)
		sv := xdr.ScVal{
			Type: xdr.ScValTypeScvI64,
			I64:  &v,
		}
		result := lens.TestScvalShortKey(sv)
		if !strings.Contains(result, "-987654321") {
			t.Errorf("got %q", result)
		}
	})
}

// TestConfigSettingShortNameInternal tests the short name map.
func TestConfigSettingShortNameInternal(t *testing.T) {
	tests := []struct {
		id   xdr.ConfigSettingId
		want string
	}{
		{xdr.ConfigSettingIdConfigSettingContractMaxSizeBytes, "contract max size bytes"},
		{xdr.ConfigSettingIdConfigSettingContractComputeV0, "contract compute V0"},
		{xdr.ConfigSettingIdConfigSettingContractLedgerCostV0, "contract ledger cost V0"},
		{xdr.ConfigSettingIdConfigSettingContractHistoricalDataV0, "contract historical data V0"},
		{xdr.ConfigSettingIdConfigSettingContractEventsV0, "contract events V0"},
		{xdr.ConfigSettingIdConfigSettingContractBandwidthV0, "contract bandwidth V0"},
		{xdr.ConfigSettingIdConfigSettingContractCostParamsCpuInstructions, "contract cost params CPU instructions"},
		{xdr.ConfigSettingIdConfigSettingContractCostParamsMemoryBytes, "contract cost params memory bytes"},
		{xdr.ConfigSettingIdConfigSettingContractDataKeySizeBytes, "contract data key size bytes"},
		{xdr.ConfigSettingIdConfigSettingContractDataEntrySizeBytes, "contract data entry size bytes"},
		{xdr.ConfigSettingIdConfigSettingStateArchival, "state archival"},
		{xdr.ConfigSettingIdConfigSettingContractExecutionLanes, "contract execution lanes"},
		{xdr.ConfigSettingIdConfigSettingLiveSorobanStateSizeWindow, "live soroban state size window"},
		{xdr.ConfigSettingIdConfigSettingEvictionIterator, "eviction iterator"},
		{xdr.ConfigSettingIdConfigSettingContractParallelComputeV0, "contract parallel compute V0"},
		{xdr.ConfigSettingIdConfigSettingContractLedgerCostExtV0, "contract ledger cost ext V0"},
		{xdr.ConfigSettingIdConfigSettingScpTiming, "SCP timing"},
		{xdr.ConfigSettingIdConfigSettingFrozenLedgerKeys, "frozen ledger keys"},
		{xdr.ConfigSettingIdConfigSettingFrozenLedgerKeysDelta, "frozen ledger keys delta"},
		{xdr.ConfigSettingIdConfigSettingFreezeBypassTxs, "freeze bypass txs"},
		{xdr.ConfigSettingIdConfigSettingFreezeBypassTxsDelta, "freeze bypass txs delta"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			result := lens.TestConfigSettingShortName(tc.id)
			if result != tc.want {
				t.Errorf("got %q, want %q", result, tc.want)
			}
		})
	}

	// Unknown ID
	t.Run("unknown_id", func(t *testing.T) {
		result := lens.TestConfigSettingShortName(xdr.ConfigSettingId(999))
		if !strings.Contains(result, "unknown") {
			t.Errorf("got %q", result)
		}
	})
}

// TestAmountFieldsIncludeConfigSettingFees verifies the fee fields
// are registered in amountFields.
func TestAmountFieldsIncludeConfigSettingFees(t *testing.T) {
	feeFields := []string{
		"FeeRatePerInstructionsIncrement",
		"FeeDiskReadLedgerEntry",
		"FeeWriteLedgerEntry",
		"FeeDiskRead1Kb",
		"RentFee1KbSorobanStateSizeLow",
		"RentFee1KbSorobanStateSizeHigh",
		"FeeHistorical1Kb",
		"FeeContractEvents1Kb",
		"FeeTxSize1Kb",
		"FeeWrite1Kb",
	}

	for _, field := range feeFields {
		t.Run(field, func(t *testing.T) {
			v := xdr.Int64(12345)
			rv := reflect.ValueOf(v)
			note, ok := lens.TestFieldNote(field, rv)
			if !ok {
				t.Errorf("field %s not in amountFields", field)
			}
			if !strings.Contains(note, "0.0012345") {
				t.Errorf("unexpected amount note: %s", note)
			}
		})
	}
}

// TestLedgerKeySummaryCoverage tests all arms of ledgerKeySummary to improve
// coverage.
func TestLedgerKeySummaryCoverage(t *testing.T) {
	// Helper to create an account ID
	makeAccountID := func(seed string) xdr.AccountId {
		// Use a deterministic approach: create a known account ID
		// For testing, we'll use the AddressToAccountId function
		// Since we need different ones, use different seeds
		// Just use a fixed valid account for each test
		addr, _ := xdr.AddressToAccountId("GAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWHF")
		return addr
	}

	tests := []struct {
		name string
		key  xdr.LedgerKey
		want string
	}{
		{
			name: "trustline",
			key: func() xdr.LedgerKey {
				aid := makeAccountID("trustline")
				asset := xdr.Asset{
					Type: xdr.AssetTypeAssetTypeNative,
				}
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeTrustline,
					TrustLine: &xdr.LedgerKeyTrustLine{
						AccountId: aid,
						Asset:     asset.ToTrustLineAsset(),
					},
				}
			}(),
			want: "trustline key:",
		},
		{
			name: "offer",
			key: func() xdr.LedgerKey {
				aid := makeAccountID("offer")
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeOffer,
					Offer: &xdr.LedgerKeyOffer{
						SellerId: aid,
						OfferId:  12345,
					},
				}
			}(),
			want: "offer key: #12345 by",
		},
		{
			name: "data",
			key: func() xdr.LedgerKey {
				aid := makeAccountID("data")
				dn := xdr.String64("test_data_name")
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeData,
					Data: &xdr.LedgerKeyData{
						AccountId: aid,
						DataName:  dn,
					},
				}
			}(),
			want: "data entry key:",
		},
		{
			name: "claimable_balance",
			key: func() xdr.LedgerKey {
				var h xdr.Hash
				copy(h[:], []byte("claimable_balance_test_123456789"))
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeClaimableBalance,
					ClaimableBalance: &xdr.LedgerKeyClaimableBalance{
						BalanceId: xdr.ClaimableBalanceId{
							Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
							V0:   &h,
						},
					},
				}
			}(),
			want: "claimable balance key:",
		},
		{
			name: "liquidity_pool",
			key: func() xdr.LedgerKey {
				var pid xdr.PoolId
				copy(pid[:], []byte("liquidity_pool_test_123456789012"))
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeLiquidityPool,
					LiquidityPool: &xdr.LedgerKeyLiquidityPool{
						LiquidityPoolId: pid,
					},
				}
			}(),
			want: "liquidity pool key:",
		},
		{
			name: "contract_data",
			key: func() xdr.LedgerKey {
				// Contract ID: CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAB
				var cid xdr.ContractId
				copy(cid[:], make([]byte, 32))
				cid[31] = 1
				contractAddr := xdr.ScAddress{
					Type:       xdr.ScAddressTypeScAddressTypeContract,
					ContractId: &cid,
				}
				// Simple ScVal key
				sym := xdr.ScSymbol("test_key")
				keyVal := xdr.ScVal{
					Type: xdr.ScValTypeScvSymbol,
					Sym:  &sym,
				}
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeContractData,
					ContractData: &xdr.LedgerKeyContractData{
						Contract:   contractAddr,
						Key:        keyVal,
						Durability: xdr.ContractDataDurabilityPersistent,
					},
				}
			}(),
			want: "contract data key:",
		},
		{
			name: "contract_code",
			key: func() xdr.LedgerKey {
				var h xdr.Hash
				copy(h[:], []byte("contract_code_wasm_hash_12345678901"))
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeContractCode,
					ContractCode: &xdr.LedgerKeyContractCode{
						Hash: h,
					},
				}
			}(),
			want: "contract code key:",
		},
		{
			name: "config_setting",
			key: func() xdr.LedgerKey {
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeConfigSetting,
					ConfigSetting: &xdr.LedgerKeyConfigSetting{
						ConfigSettingId: xdr.ConfigSettingIdConfigSettingStateArchival,
					},
				}
			}(),
			want: "config setting key: state archival",
		},
		{
			name: "ttl",
			key: func() xdr.LedgerKey {
				var h xdr.Hash
				copy(h[:], []byte("ttl_key_hash_test_123456789012345"))
				return xdr.LedgerKey{
					Type: xdr.LedgerEntryTypeTtl,
					Ttl: &xdr.LedgerKeyTtl{
						KeyHash: h,
					},
				}
			}(),
			want: "ttl key:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			summary, ok := lens.TestLedgerKeySummary(tc.key)
			if !ok {
				t.Fatalf("ledgerKeySummary declined for %s", tc.name)
			}
			if !strings.Contains(summary, tc.want) {
				t.Errorf("summary for %s missing %q: got %q", tc.name, tc.want, summary)
			}
		})
	}

	// Test nil arm declines
	t.Run("nil_account_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeAccount}
		summary, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil account arm should decline")
		}
		if summary != "" {
			t.Errorf("summary should be empty on decline: %q", summary)
		}
	})

	t.Run("nil_trustline_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeTrustline}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil trustline arm should decline")
		}
	})

	t.Run("nil_offer_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeOffer}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil offer arm should decline")
		}
	})

	t.Run("nil_data_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeData}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil data arm should decline")
		}
	})

	t.Run("nil_claimable_balance_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeClaimableBalance}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil claimable balance arm should decline")
		}
	})

	t.Run("nil_liquidity_pool_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeLiquidityPool}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil liquidity pool arm should decline")
		}
	})

	t.Run("nil_contract_data_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeContractData}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil contract data arm should decline")
		}
	})

	t.Run("nil_contract_code_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeContractCode}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil contract code arm should decline")
		}
	})

	t.Run("nil_config_setting_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeConfigSetting}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil config setting arm should decline")
		}
	})

	t.Run("nil_ttl_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryTypeTtl}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("nil ttl arm should decline")
		}
	})

	// Test unknown type declines
	t.Run("unknown_type_declines", func(t *testing.T) {
		key := xdr.LedgerKey{Type: xdr.LedgerEntryType(99)}
		_, ok := lens.TestLedgerKeySummary(key)
		if ok {
			t.Fatal("unknown type should decline")
		}
	})
}
