package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stellar/go-stellar-sdk/amount"
	"github.com/stellar/go-stellar-sdk/xdr"
)

var outDir = flag.String("out", "testdata", "directory to write fixtures into (relative paths resolve from the invocation directory)")

func mustMarshal(v any) string {
	b, err := xdr.MarshalBase64(v)
	if err != nil {
		panic(err)
	}
	return b
}

func mustAccountID(addr string) xdr.AccountId {
	var aid xdr.AccountId
	if err := aid.SetAddress(addr); err != nil {
		panic(err)
	}
	return aid
}

func creditAsset(code, issuer string) xdr.Asset {
	var a xdr.Asset
	if err := a.SetCredit(code, mustAccountID(issuer)); err != nil {
		panic(err)
	}
	return a
}

func nativeAsset() xdr.Asset {
	return xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
}

func writeFixture(name, content string) {
	path := filepath.Join(*outDir, name+".txt")
	if err := os.WriteFile(path, []byte(content+"\n"), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Wrote %s\n", path)
}

func ptr[T any](v T) *T {
	return &v
}

func makePoolId() xdr.PoolId {
	var h xdr.Hash
	copy(h[:], strings.Repeat("A", 32))
	return xdr.PoolId(h)
}

func makeClaimableBalanceId(prefix string) xdr.ClaimableBalanceId {
	var h xdr.Hash
	copy(h[:], strings.Repeat(prefix, 32/len(prefix)))
	return xdr.ClaimableBalanceId{
		Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
		V0:   &h,
	}
}

func makeContractId() xdr.ContractId {
	var h xdr.Hash
	copy(h[:], strings.Repeat("C", 32))
	return xdr.ContractId(h)
}

func main() {
	flag.Parse()
	fmt.Printf("Writing constructed fixtures to %s\n", *outDir)

	// Test addresses
	const (
		src     = "GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6"
		dest    = "GAPB7YMUQ6MPQ23PTFDNBZP7HGGTZPI2DT5JJURUOIXNPLGHW2O3WYXZ"
		issuer  = "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
		issuer2 = "GB6ZLIKYNLINUD3HMU6BHNL3OOWOSBS4YFCJJ4KWU6G6T5V5KJ6D5JQA"
	)

	// ============================================================
	// 1. LiquidityPoolEntry with ConstantProduct parameters
	// ============================================================
	{
		poolID := makePoolId()

		assetA := nativeAsset()
		assetB := creditAsset("USDC", issuer)
		// Ensure assetA < assetB for canonical ordering
		if assetB.StringCanonical() < assetA.StringCanonical() {
			assetA, assetB = assetB, assetA
		}

		entry := xdr.LiquidityPoolEntry{
			LiquidityPoolId: poolID,
			Body: xdr.LiquidityPoolEntryBody{
				Type: xdr.LiquidityPoolTypeLiquidityPoolConstantProduct,
				ConstantProduct: &xdr.LiquidityPoolEntryConstantProduct{
					Params: xdr.LiquidityPoolConstantProductParameters{
						AssetA: assetA,
						AssetB: assetB,
						Fee:    xdr.Int32(30), // 0.3%
					},
					ReserveA:                 xdr.Int64(1_000_000_000_000), // 10000 XLM
					ReserveB:                 xdr.Int64(5_000_000_000_000), // 50000 USDC
					TotalPoolShares:          xdr.Int64(2_000_000_000_000), // 20000 pool shares
					PoolSharesTrustLineCount: xdr.Int64(150),
				},
			},
		}
		writeFixture("liquidity_pool_entry", mustMarshal(entry))
	}

	// ============================================================
	// 2. ClaimableBalanceEntry with claimants and predicates
	// ============================================================
	{
		balanceID := makeClaimableBalanceId("B")

		claimant1 := xdr.Claimant{
			Type: xdr.ClaimantTypeClaimantTypeV0,
			V0: &xdr.ClaimantV0{
				Destination: mustAccountID(dest),
				Predicate: xdr.ClaimPredicate{
					Type:      xdr.ClaimPredicateTypeClaimPredicateBeforeAbsoluteTime,
					AbsBefore: ptr(xdr.Int64(1735689600)), // 2025-01-01
				},
			},
		}

		claimant2 := xdr.Claimant{
			Type: xdr.ClaimantTypeClaimantTypeV0,
			V0: &xdr.ClaimantV0{
				Destination: mustAccountID(src),
				Predicate: xdr.ClaimPredicate{
					Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional,
				},
			},
		}

		// Complex predicate: (before absolute time) OR (unconditional)
		orPred := xdr.ClaimPredicate{
			Type: xdr.ClaimPredicateTypeClaimPredicateOr,
			OrPredicates: &[]xdr.ClaimPredicate{
				{Type: xdr.ClaimPredicateTypeClaimPredicateBeforeAbsoluteTime, AbsBefore: ptr(xdr.Int64(1767225600))}, // 2026-01-01
				{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional},
			},
		}

		claimant3 := xdr.Claimant{
			Type: xdr.ClaimantTypeClaimantTypeV0,
			V0: &xdr.ClaimantV0{
				Destination: mustAccountID(issuer),
				Predicate:   orPred,
			},
		}

		entry := xdr.ClaimableBalanceEntry{
			BalanceId: balanceID,
			Claimants: []xdr.Claimant{claimant1, claimant2, claimant3},
			Asset:     creditAsset("USDC", issuer),
			Amount:    xdr.Int64(1000 * amount.One), // 1000 USDC
			Ext: xdr.ClaimableBalanceEntryExt{
				V: 1,
				V1: &xdr.ClaimableBalanceEntryExtensionV1{
					Flags: xdr.Uint32(0),
				},
			},
		}
		writeFixture("claimable_balance_entry", mustMarshal(entry))
	}

	// ============================================================
	// 3. TrustLineEntry with V1 extension (liabilities)
	// ============================================================
	{
		alphaNum4 := xdr.AlphaNum4{
			AssetCode: xdr.AssetCode4([4]byte{'U', 'S', 'D', 'C'}),
			Issuer:    mustAccountID(issuer),
		}

		entry := xdr.TrustLineEntry{
			AccountId: mustAccountID(src),
			Asset: xdr.TrustLineAsset{
				Type:      xdr.AssetTypeAssetTypeCreditAlphanum4,
				AlphaNum4: &alphaNum4,
			},
			Balance: xdr.Int64(5000 * amount.One),
			Limit:   xdr.Int64(10000 * amount.One),
			Flags:   xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag | xdr.TrustLineFlagsAuthorizedToMaintainLiabilitiesFlag),
			Ext: xdr.TrustLineEntryExt{
				V: 1,
				V1: &xdr.TrustLineEntryV1{
					Liabilities: xdr.Liabilities{
						Buying:  xdr.Int64(100 * amount.One),
						Selling: xdr.Int64(200 * amount.One),
					},
					Ext: xdr.TrustLineEntryV1Ext{V: 0},
				},
			},
		}
		writeFixture("trustline_entry_v1", mustMarshal(entry))
	}

	// ============================================================
	// 4. TrustLineEntry with V2 extension (liquidity pool use count)
	// ============================================================
	{
		poolID := makePoolId()
		entry := xdr.TrustLineEntry{
			AccountId: mustAccountID(dest),
			Asset: xdr.TrustLineAsset{
				Type:            xdr.AssetTypeAssetTypePoolShare,
				LiquidityPoolId: &poolID,
			},
			Balance: xdr.Int64(2000 * amount.One),
			Limit:   xdr.Int64(5000 * amount.One),
			Flags:   xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
			Ext: xdr.TrustLineEntryExt{
				V: 1,
				V1: &xdr.TrustLineEntryV1{
					Liabilities: xdr.Liabilities{
						Buying:  xdr.Int64(0),
						Selling: xdr.Int64(0),
					},
					Ext: xdr.TrustLineEntryV1Ext{
						V: 2,
						V2: &xdr.TrustLineEntryExtensionV2{
							LiquidityPoolUseCount: xdr.Int32(5),
							Ext:                   xdr.TrustLineEntryExtensionV2Ext{V: 0},
						},
					},
				},
			},
		}
		writeFixture("trustline_entry_v2", mustMarshal(entry))
	}

	// ============================================================
	// 5. DiagnosticEvent with ContractEvent
	// ============================================================
	{
		contractID := makeContractId()

		// Create SCVal for topics
		topic1 := xdr.ScVal{
			Type: xdr.ScValTypeScvString,
			Str:  ptr(xdr.ScString("transfer")),
		}
		topic2 := xdr.ScVal{
			Type: xdr.ScValTypeScvAddress,
			Address: &xdr.ScAddress{
				Type:      xdr.ScAddressTypeScAddressTypeAccount,
				AccountId: ptr(mustAccountID(src)),
			},
		}

		// Create SCVal map for data
		mapEntries := []xdr.ScMapEntry{
			{
				Key: xdr.ScVal{Type: xdr.ScValTypeScvString, Str: ptr(xdr.ScString("from"))},
				Val: xdr.ScVal{
					Type: xdr.ScValTypeScvAddress,
					Address: &xdr.ScAddress{
						Type:      xdr.ScAddressTypeScAddressTypeAccount,
						AccountId: ptr(mustAccountID(src)),
					},
				},
			},
			{
				Key: xdr.ScVal{Type: xdr.ScValTypeScvString, Str: ptr(xdr.ScString("to"))},
				Val: xdr.ScVal{
					Type: xdr.ScValTypeScvAddress,
					Address: &xdr.ScAddress{
						Type:      xdr.ScAddressTypeScAddressTypeAccount,
						AccountId: ptr(mustAccountID(dest)),
					},
				},
			},
			{
				Key: xdr.ScVal{Type: xdr.ScValTypeScvString, Str: ptr(xdr.ScString("amount"))},
				Val: xdr.ScVal{
					Type: xdr.ScValTypeScvI128,
					I128: ptr(xdr.Int128Parts{Hi: xdr.Int64(0), Lo: xdr.Uint64(1000000000)}),
				},
			},
		}

		event := xdr.DiagnosticEvent{
			InSuccessfulContractCall: true,
			Event: xdr.ContractEvent{
				Ext:        xdr.ExtensionPoint{V: 0},
				ContractId: &contractID,
				Type:       xdr.ContractEventTypeContract,
				Body: xdr.ContractEventBody{
					V: 0,
					V0: &xdr.ContractEventV0{
						Topics: []xdr.ScVal{topic1, topic2},
						Data: xdr.ScVal{
							Type: xdr.ScValTypeScvMap,
							Map:  ptr(ptr(xdr.ScMap(mapEntries))),
						},
					},
				},
			},
		}
		writeFixture("diagnostic_event", mustMarshal(event))
	}

	// ============================================================
	// 6. Simple ClaimableBalanceEntry (no extension)
	// ============================================================
	{
		balanceID := makeClaimableBalanceId("D")

		entry := xdr.ClaimableBalanceEntry{
			BalanceId: balanceID,
			Claimants: []xdr.Claimant{
				{
					Type: xdr.ClaimantTypeClaimantTypeV0,
					V0: &xdr.ClaimantV0{
						Destination: mustAccountID(dest),
						Predicate: xdr.ClaimPredicate{
							Type:      xdr.ClaimPredicateTypeClaimPredicateBeforeRelativeTime,
							RelBefore: ptr(xdr.Int64(86400)), // 1 day
						},
					},
				},
			},
			Asset:  nativeAsset(),
			Amount: xdr.Int64(50 * amount.One),
			Ext:    xdr.ClaimableBalanceEntryExt{V: 0},
		}
		writeFixture("claimable_balance_entry_simple", mustMarshal(entry))
	}

	// ============================================================
	// 7. TrustLineEntry simple (no extension)
	// ============================================================
	{
		alphaNum4 := xdr.AlphaNum4{
			AssetCode: xdr.AssetCode4([4]byte{'U', 'S', 'D', 'C'}),
			Issuer:    mustAccountID(issuer),
		}

		entry := xdr.TrustLineEntry{
			AccountId: mustAccountID(src),
			Asset: xdr.TrustLineAsset{
				Type:      xdr.AssetTypeAssetTypeCreditAlphanum4,
				AlphaNum4: &alphaNum4,
			},
			Balance: xdr.Int64(1000 * amount.One),
			Limit:   xdr.Int64(5000 * amount.One),
			Flags:   xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
			Ext:     xdr.TrustLineEntryExt{V: 0},
		}
		writeFixture("trustline_entry_simple", mustMarshal(entry))
	}

	fmt.Println("All fixtures generated successfully!")
}
