package format_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens/format"
)

func TestTxRepSEP0011SpecVector(t *testing.T) {
	// Canonical base64 payload from SEP-0011 specification
	payload := "AAAAAgAAAAArFkuQQ4QuQY6SkLc5xxSdwpFOvl7VqKVvrfkPSqB+0AAAAGQApSmNAAAAAQAAAAEAAAAAW4nJgAAAAABdav0AAAAAAQAAABZFbmpveSB0aGlzIHRyYW5zYWN0aW9uAAAAAAABAAAAAAAAAAEAAAAAQF827djPIu+/gHK5hbakwBVRw03TjBN6yNQNQCzR97QAAAABVVNEAAAAAAAyUlQyIZKfbs+tUWuvK7N0nGSCII0/Go1/CpHXNW3tCwAAAAAX15OgAAAAAAAAAAFKoH7QAAAAQN77Tx+tHCeTJ7Va8YT9zd9z9Peoy0Dn5TSnHXOgUSS6Np23ptMbR8r9EYWSJGqFdebCSauU7Ddo3ttikiIc5Qw="

	val, err := lens.Decode(payload)
	if err != nil {
		t.Fatalf("decoding SEP-0011 fixture: %v", err)
	}

	f := format.NewTxRepFormatter()
	var buf bytes.Buffer
	if err := f.Format(&buf, val); err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	out := buf.String()

	expectedLines := []string{
		"type: ENVELOPE_TYPE_TX",
		"tx.sourceAccount: GAVRMS4QIOCC4QMOSKILOOOHCSO4FEKOXZPNLKFFN6W7SD2KUB7NBPLN",
		"tx.fee: 100",
		"tx.seqNum: 46489056724385793",
		"tx.timeBounds._present: true",
		"tx.timeBounds.minTime: 1535756672",
		"tx.timeBounds.maxTime: 1567292672",
		"tx.memo.type: MEMO_TEXT",
		`tx.memo.text: "Enjoy this transaction"`,
		"tx.operations.len: 1",
		"tx.operations[0].sourceAccount._present: false",
		"tx.operations[0].body.type: PAYMENT",
		"tx.operations[0].body.paymentOp.destination: GBAF6NXN3DHSF357QBZLTBNWUTABKUODJXJYYE32ZDKA2QBM2H33IK6O",
		"tx.operations[0].body.paymentOp.asset: USD:GAZFEVBSEGJJ63WPVVIWXLZLWN2JYZECECGT6GUNP4FJDVZVNXWQWMYI",
		"tx.operations[0].body.paymentOp.amount: 400004000",
		"tx.ext.v: 0",
		"signatures.len: 1",
		"signatures[0].hint: 4aa07ed0",
	}

	for _, line := range expectedLines {
		if !strings.Contains(out, line) {
			t.Errorf("missing expected line %q in txrep output:\n%s", line, out)
		}
	}
}

func TestTxRepCanonicalFixtures(t *testing.T) {
	fixtures := []struct {
		name     string
		expected []string
	}{
		{
			name: "tx_success.env.txt",
			expected: []string{
				"type: ENVELOPE_TYPE_TX",
				"tx.sourceAccount: ",
				"tx.fee: ",
				"tx.operations.len: 1",
				"signatures.len: 1",
			},
		},
		{
			name: "tx_failed.env.txt",
			expected: []string{
				"type: ENVELOPE_TYPE_TX",
				"tx.operations.len: 1",
			},
		},
		{
			name: "tx_feebump.env.txt",
			expected: []string{
				"type: ENVELOPE_TYPE_TX_FEE_BUMP",
				"feeBump.tx.feeSource: ",
				"feeBump.tx.innerTx.type: ENVELOPE_TYPE_TX",
				"feeBump.tx.innerTx.tx.sourceAccount: ",
			},
		},
		{
			name: "tx_soroban.env.txt",
			expected: []string{
				"type: ENVELOPE_TYPE_TX_FEE_BUMP",
				"feeBump.tx.innerTx.tx.operations.len: 1",
			},
		},
	}

	f := format.NewTxRepFormatter()

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "testdata", tc.name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading fixture %s: %v", tc.name, err)
			}
			val, err := lens.Decode(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("decoding fixture %s: %v", tc.name, err)
			}

			var buf bytes.Buffer
			if err := f.Format(&buf, val); err != nil {
				t.Fatalf("formatting %s: %v", tc.name, err)
			}

			out := buf.String()
			for _, exp := range tc.expected {
				if !strings.Contains(out, exp) {
					t.Errorf("fixture %s missing expected substring %q in output:\n%s", tc.name, exp, out)
				}
			}
		})
	}
}

func TestTxRepV0Envelope(t *testing.T) {
	v0 := xdr.TransactionV0Envelope{
		Tx: xdr.TransactionV0{
			SourceAccountEd25519: xdr.Uint256{1, 2, 3},
			Fee:                  100,
			SeqNum:               1,
			TimeBounds: &xdr.TimeBounds{
				MinTime: 100,
				MaxTime: 200,
			},
			Memo: xdr.Memo{Type: xdr.MemoTypeMemoNone},
			Operations: []xdr.Operation{
				{
					Body: xdr.OperationBody{
						Type: xdr.OperationTypePayment,
						PaymentOp: &xdr.PaymentOp{
							Destination: xdr.MuxedAccount{
								Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
								Ed25519: &xdr.Uint256{4, 5, 6},
							},
							Asset:  xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
							Amount: 500,
						},
					},
				},
			},
		},
	}
	env := xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTxV0,
		V0:   &v0,
	}

	val := &lens.Value{
		Type: "TransactionEnvelope",
		Raw:  &env,
	}

	f := format.NewTxRepFormatter()
	var buf bytes.Buffer
	if err := f.Format(&buf, val); err != nil {
		t.Fatalf("formatting v0: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "type: ENVELOPE_TYPE_TX_V0") {
		t.Errorf("expected ENVELOPE_TYPE_TX_V0 in output:\n%s", out)
	}
	if !strings.Contains(out, "tx.timeBounds.minTime: 100") {
		t.Errorf("expected minTime in output:\n%s", out)
	}
}

func TestTxRepOptionsAndErrors(t *testing.T) {
	t.Run("NonEnvelopeError", func(t *testing.T) {
		val := &lens.Value{
			Type: "TransactionResult",
			Raw:  &xdr.TransactionResult{},
		}
		f := format.NewTxRepFormatter()
		err := f.Format(&bytes.Buffer{}, val)
		if err == nil {
			t.Fatal("expected error for non-envelope")
		}
		if !errors.Is(err, format.ErrNotTransactionEnvelope) {
			t.Errorf("expected ErrNotTransactionEnvelope, got: %v", err)
		}
	})

	t.Run("NilValue", func(t *testing.T) {
		f := format.NewTxRepFormatter()
		if err := f.Format(&bytes.Buffer{}, nil); err == nil {
			t.Error("expected error for nil value")
		}
	})

	t.Run("WithOptions", func(t *testing.T) {
		f := format.NewTxRepFormatter(
			format.WithTxRepPalette(format.ColorPalette),
			format.WithTxRepNetwork("Public Global Stellar Network ; September 2015"),
			format.WithTxRepComments(true),
		)
		v0 := xdr.TransactionV0Envelope{
			Tx: xdr.TransactionV0{
				SourceAccountEd25519: xdr.Uint256{1, 2, 3},
				Fee:                  100,
				SeqNum:               1,
				Operations: []xdr.Operation{
					{
						Body: xdr.OperationBody{
							Type: xdr.OperationTypePayment,
							PaymentOp: &xdr.PaymentOp{
								Destination: xdr.MuxedAccount{
									Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
									Ed25519: &xdr.Uint256{4, 5, 6},
								},
								Asset:  xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
								Amount: 500,
							},
						},
					},
				},
			},
		}
		val := &lens.Value{
			Type: "TransactionEnvelope",
			Raw:  &xdr.TransactionEnvelope{Type: xdr.EnvelopeTypeEnvelopeTypeTxV0, V0: &v0},
		}

		var buf bytes.Buffer
		if err := f.Format(&buf, val); err != nil {
			t.Fatalf("format error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "\x1b[") {
			t.Errorf("expected colour escapes in output")
		}
		if !strings.Contains(out, "XLM") {
			t.Errorf("expected XLM for native asset on public network, got:\n%s", out)
		}

		// Also test TestXLM
		fTest := format.NewTxRepFormatter(format.WithTxRepNetwork("Test SDF Network ; September 2015"))
		var bufTest bytes.Buffer
		if err := fTest.Format(&bufTest, val); err != nil {
			t.Fatalf("format error: %v", err)
		}
		if !strings.Contains(bufTest.String(), "TestXLM") {
			t.Errorf("expected TestXLM for native asset on test network, got:\n%s", bufTest.String())
		}
	})
}

func TestTxRepAllOperationTypesAndPreconditions(t *testing.T) {
	aid := xdr.MustAddress("GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6")
	med := xdr.MuxedAccount{
		Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
		Ed25519: aid.Ed25519,
	}
	hash := [32]byte{1, 2, 3, 4}
	seqNum := xdr.SequenceNumber(123)
	intVal := xdr.Uint32(7)
	strVal := xdr.String32("example.com")
	dataBytes := xdr.DataValue([]byte("data"))

	a4 := xdr.Asset{
		Type: xdr.AssetTypeAssetTypeCreditAlphanum4,
		AlphaNum4: &xdr.AlphaNum4{
			AssetCode: [4]byte{'U', 'S', 'D', 0},
			Issuer:    aid,
		},
	}
	a12 := xdr.Asset{
		Type: xdr.AssetTypeAssetTypeCreditAlphanum12,
		AlphaNum12: &xdr.AlphaNum12{
			AssetCode: [12]byte{'S', 'T', 'E', 'L', 'L', 'A', 'R', 0, 0, 0, 0, 0},
			Issuer:    aid,
		},
	}
	cta4 := xdr.ChangeTrustAsset{
		Type: xdr.AssetTypeAssetTypeCreditAlphanum4,
		AlphaNum4: &xdr.AlphaNum4{
			AssetCode: [4]byte{'E', 'U', 'R', 0},
			Issuer:    aid,
		},
	}
	cta12 := xdr.ChangeTrustAsset{
		Type: xdr.AssetTypeAssetTypeCreditAlphanum12,
		AlphaNum12: &xdr.AlphaNum12{
			AssetCode: [12]byte{'L', 'O', 'N', 'G', 'C', 'O', 'D', 'E', 0, 0, 0, 0},
			Issuer:    aid,
		},
	}
	ctaPool := xdr.ChangeTrustAsset{
		Type: xdr.AssetTypeAssetTypePoolShare,
		LiquidityPool: &xdr.LiquidityPoolParameters{
			Type: xdr.LiquidityPoolTypeLiquidityPoolConstantProduct,
			ConstantProduct: &xdr.LiquidityPoolConstantProductParameters{
				AssetA: a4,
				AssetB: a12,
				Fee:    30,
			},
		},
	}
	ac4 := xdr.AssetCode{
		Type:       xdr.AssetTypeAssetTypeCreditAlphanum4,
		AssetCode4: &xdr.AssetCode4{'B', 'T', 'C', 0},
	}
	ac12 := xdr.AssetCode{
		Type:        xdr.AssetTypeAssetTypeCreditAlphanum12,
		AssetCode12: &xdr.AssetCode12{'E', 'T', 'H', 'E', 'R', 'E', 'U', 'M', 0, 0, 0, 0},
	}

	ops := []xdr.Operation{
		{
			SourceAccount: &med,
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeCreateAccount,
				CreateAccountOp: &xdr.CreateAccountOp{
					Destination:     aid,
					StartingBalance: 1000,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypePathPaymentStrictReceive,
				PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{
					SendAsset:   a4,
					SendMax:     100,
					Destination: med,
					DestAsset:   a12,
					DestAmount:  90,
					Path:        []xdr.Asset{a4, a12},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypePathPaymentStrictSend,
				PathPaymentStrictSendOp: &xdr.PathPaymentStrictSendOp{
					SendAsset:   a12,
					SendAmount:  50,
					Destination: med,
					DestAsset:   a4,
					DestMin:     40,
					Path:        []xdr.Asset{a12},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeManageSellOffer,
				ManageSellOfferOp: &xdr.ManageSellOfferOp{
					Selling: a4,
					Buying:  a12,
					Amount:  10,
					Price:   xdr.Price{N: 1, D: 2},
					OfferId: 99,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeManageBuyOffer,
				ManageBuyOfferOp: &xdr.ManageBuyOfferOp{
					Selling:   a12,
					Buying:    a4,
					BuyAmount: 15,
					Price:     xdr.Price{N: 2, D: 1},
					OfferId:   101,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeCreatePassiveSellOffer,
				CreatePassiveSellOfferOp: &xdr.CreatePassiveSellOfferOp{
					Selling: a4,
					Buying:  a12,
					Amount:  20,
					Price:   xdr.Price{N: 3, D: 4},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeSetOptions,
				SetOptionsOp: &xdr.SetOptionsOp{
					InflationDest: &aid,
					ClearFlags:    &intVal,
					SetFlags:      &intVal,
					MasterWeight:  &intVal,
					LowThreshold:  &intVal,
					MedThreshold:  &intVal,
					HighThreshold: &intVal,
					HomeDomain:    &strVal,
					Signer: &xdr.Signer{
						Key:    xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519, Ed25519: aid.Ed25519},
						Weight: 5,
					},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeChangeTrust,
				ChangeTrustOp: &xdr.ChangeTrustOp{
					Line:  cta4,
					Limit: 1000000,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeChangeTrust,
				ChangeTrustOp: &xdr.ChangeTrustOp{
					Line:  cta12,
					Limit: 2000000,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeChangeTrust,
				ChangeTrustOp: &xdr.ChangeTrustOp{
					Line:  ctaPool,
					Limit: 3000000,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeAllowTrust,
				AllowTrustOp: &xdr.AllowTrustOp{
					Trustor:   aid,
					Asset:     ac4,
					Authorize: 1,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeAllowTrust,
				AllowTrustOp: &xdr.AllowTrustOp{
					Trustor:   aid,
					Asset:     ac12,
					Authorize: 2,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type:        xdr.OperationTypeAccountMerge,
				Destination: &med,
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeInflation,
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeManageData,
				ManageDataOp: &xdr.ManageDataOp{
					DataName:  "mykey",
					DataValue: &dataBytes,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeBumpSequence,
				BumpSequenceOp: &xdr.BumpSequenceOp{
					BumpTo: 9999,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeCreateClaimableBalance,
				CreateClaimableBalanceOp: &xdr.CreateClaimableBalanceOp{
					Asset:     a4,
					Amount:    5000,
					Claimants: []xdr.Claimant{},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeClaimClaimableBalance,
				ClaimClaimableBalanceOp: &xdr.ClaimClaimableBalanceOp{
					BalanceId: xdr.ClaimableBalanceId{
						Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
						V0:   &xdr.Hash{1},
					},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeBeginSponsoringFutureReserves,
				BeginSponsoringFutureReservesOp: &xdr.BeginSponsoringFutureReservesOp{
					SponsoredId: aid,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeEndSponsoringFutureReserves,
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeRevokeSponsorship,
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeClawback,
				ClawbackOp: &xdr.ClawbackOp{
					Asset:  a4,
					From:   med,
					Amount: 700,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeClawbackClaimableBalance,
				ClawbackClaimableBalanceOp: &xdr.ClawbackClaimableBalanceOp{
					BalanceId: xdr.ClaimableBalanceId{
						Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
						V0:   &xdr.Hash{2},
					},
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeSetTrustLineFlags,
				SetTrustLineFlagsOp: &xdr.SetTrustLineFlagsOp{
					Trustor:    aid,
					Asset:      a4,
					ClearFlags: 1,
					SetFlags:   2,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeLiquidityPoolDeposit,
				LiquidityPoolDepositOp: &xdr.LiquidityPoolDepositOp{
					LiquidityPoolId: xdr.PoolId{1, 2},
					MaxAmountA:      1000,
					MaxAmountB:      2000,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeLiquidityPoolWithdraw,
				LiquidityPoolWithdrawOp: &xdr.LiquidityPoolWithdrawOp{
					LiquidityPoolId: xdr.PoolId{3, 4},
					Amount:          500,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeExtendFootprintTtl,
				ExtendFootprintTtlOp: &xdr.ExtendFootprintTtlOp{
					Ext:      xdr.ExtensionPoint{V: 0},
					ExtendTo: 1000,
				},
			},
		},
		{
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeRestoreFootprint,
				RestoreFootprintOp: &xdr.RestoreFootprintOp{
					Ext: xdr.ExtensionPoint{V: 0},
				},
			},
		},
	}

	precondV2 := xdr.Preconditions{
		Type: xdr.PreconditionTypePrecondV2,
		V2: &xdr.PreconditionsV2{
			TimeBounds:      &xdr.TimeBounds{MinTime: 10, MaxTime: 20},
			LedgerBounds:    &xdr.LedgerBounds{MinLedger: 5, MaxLedger: 10},
			MinSeqNum:       &seqNum,
			MinSeqAge:       100,
			MinSeqLedgerGap: 2,
			ExtraSigners: []xdr.SignerKey{
				{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519, Ed25519: aid.Ed25519},
			},
		},
	}

	uint64Val := xdr.Uint64(100)
	for _, memo := range []xdr.Memo{
		{Type: xdr.MemoTypeMemoId, Id: &uint64Val},
		{Type: xdr.MemoTypeMemoHash, Hash: (*xdr.Hash)(&hash)},
		{Type: xdr.MemoTypeMemoReturn, RetHash: (*xdr.Hash)(&hash)},
	} {
		tx := xdr.Transaction{
			SourceAccount: med,
			Fee:           100,
			SeqNum:        1,
			Cond:          precondV2,
			Memo:          memo,
			Operations:    ops,
		}
		env := xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{
				Tx: tx,
			},
		}
		val := &lens.Value{
			Type: "TransactionEnvelope",
			Raw:  &env,
		}
		f := format.NewTxRepFormatter()
		var buf bytes.Buffer
		if err := f.Format(&buf, val); err != nil {
			t.Fatalf("format error: %v", err)
		}
		if !strings.Contains(buf.String(), "PRECOND_V2") {
			t.Errorf("expected PRECOND_V2 in output")
		}
	}
}
