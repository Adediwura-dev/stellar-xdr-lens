package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestExplainEnvelope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fixture     string
		wantKind    string
		wantOps     int
		wantFeeBump bool
		wantOpType  string
	}{
		{
			name:        "soroban invoke",
			fixture:     "tx_soroban.env",
			wantKind:    "transaction_envelope",
			wantOps:     1,
			wantFeeBump: true,
			wantOpType:  "invoke_host_function",
		},
		{
			name:        "failed soroban invoke",
			fixture:     "tx_failed.env",
			wantKind:    "transaction_envelope",
			wantOps:     1,
			wantFeeBump: true,
			wantOpType:  "invoke_host_function",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, err := lens.Decode(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			got, err := lens.Explain(v)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}

			if got.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, tc.wantKind)
			}
			if len(got.Operations) != tc.wantOps {
				t.Errorf("len(Operations) = %d, want %d", len(got.Operations), tc.wantOps)
			}
			if tc.wantFeeBump && got.FeeBumpSource == "" {
				t.Error("FeeBumpSource is empty, want a fee-bump source address")
			}
			if !strings.HasPrefix(got.Source, "G") {
				t.Errorf("Source = %q, want a strkey address", got.Source)
			}
			if got.Headline == "" {
				t.Error("Headline is empty")
			}
			if len(got.Operations) > 0 && got.Operations[0].Type != tc.wantOpType {
				t.Errorf("Operations[0].Type = %q, want %q", got.Operations[0].Type, tc.wantOpType)
			}
			// An envelope alone carries no outcome; claiming one would be wrong.
			if got.Outcome != nil {
				t.Error("Outcome is set for an envelope-only explanation, want nil")
			}
		})
	}
}

func TestExplainResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fixture     string
		wantSuccess bool
		wantCode    string
	}{
		{"successful result", "tx_soroban.res", true, "tx_fee_bump_inner_success"},
		{"failed result", "tx_failed.res", false, "tx_fee_bump_inner_failed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, err := lens.Decode(loadFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			got, err := lens.Explain(v)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}

			if got.Outcome == nil {
				t.Fatal("Outcome is nil, want an outcome for a result")
			}
			if got.Outcome.Success != tc.wantSuccess {
				t.Errorf("Outcome.Success = %v, want %v", got.Outcome.Success, tc.wantSuccess)
			}
			if got.Outcome.Reason.Code != tc.wantCode {
				t.Errorf("Outcome.Reason.Code = %q, want %q", got.Outcome.Reason.Code, tc.wantCode)
			}
			if got.Outcome.Reason.Summary == "" {
				t.Error("Outcome.Reason.Summary is empty")
			}
		})
	}
}

// TestExplainPairAttributesFailureToOperation is the core assertion for the
// feature that distinguishes this tool: pairing an envelope with its result
// must name which operation failed and why.
func TestExplainPairAttributesFailureToOperation(t *testing.T) {
	t.Parallel()

	env, err := lens.Decode(loadFixture(t, "tx_failed.env"))
	if err != nil {
		t.Fatalf("decoding envelope: %v", err)
	}
	res, err := lens.Decode(loadFixture(t, "tx_failed.res"))
	if err != nil {
		t.Fatalf("decoding result: %v", err)
	}

	got, err := lens.ExplainPair(env, res)
	if err != nil {
		t.Fatalf("ExplainPair() error = %v", err)
	}

	if got.Kind != "transaction" {
		t.Errorf("Kind = %q, want %q", got.Kind, "transaction")
	}
	if got.Outcome == nil {
		t.Fatal("Outcome is nil")
	}
	if got.Outcome.Success {
		t.Error("Outcome.Success = true, want false for a failed transaction")
	}

	// The failing operation must be identified by index.
	if len(got.Outcome.FailedOps) != 1 || got.Outcome.FailedOps[0] != 0 {
		t.Fatalf("FailedOps = %v, want [0]", got.Outcome.FailedOps)
	}

	// That operation must carry a specific, non-generic reason.
	op := got.Operations[0]
	if op.Result == nil {
		t.Fatal("Operations[0].Result is nil, want the operation's outcome")
	}
	if op.Result.Success {
		t.Error("Operations[0].Result.Success = true, want false")
	}
	if op.Result.Code != "invoke_host_function_trapped" {
		t.Errorf("Operations[0].Result.Code = %q, want %q",
			op.Result.Code, "invoke_host_function_trapped")
	}
	if op.Result.Hint == "" {
		t.Error("Operations[0].Result.Hint is empty, want actionable advice")
	}

	// The headline must name the operation and its cause, since that is what
	// a developer reads first.
	for _, want := range []string{"operation 0", "invoke_host_function", "trapped"} {
		if !strings.Contains(strings.ToLower(got.Headline), strings.ToLower(want)) {
			t.Errorf("Headline = %q, want it to mention %q", got.Headline, want)
		}
	}

	// A fee bump must surface the inner transaction's real cause.
	if got.Outcome.InnerReason == nil {
		t.Fatal("Outcome.InnerReason is nil, want the inner result for a fee bump")
	}
	if got.Outcome.InnerReason.Code != "tx_failed" {
		t.Errorf("InnerReason.Code = %q, want %q", got.Outcome.InnerReason.Code, "tx_failed")
	}
}

func TestExplainPairSuccessfulTransaction(t *testing.T) {
	t.Parallel()

	env, err := lens.Decode(loadFixture(t, "tx_soroban.env"))
	if err != nil {
		t.Fatalf("decoding envelope: %v", err)
	}
	res, err := lens.Decode(loadFixture(t, "tx_soroban.res"))
	if err != nil {
		t.Fatalf("decoding result: %v", err)
	}

	got, err := lens.ExplainPair(env, res)
	if err != nil {
		t.Fatalf("ExplainPair() error = %v", err)
	}

	if got.Outcome == nil || !got.Outcome.Success {
		t.Fatal("Outcome.Success = false, want true")
	}
	if len(got.Outcome.FailedOps) != 0 {
		t.Errorf("FailedOps = %v, want empty", got.Outcome.FailedOps)
	}
	for i, op := range got.Operations {
		if op.Result == nil {
			t.Errorf("Operations[%d].Result is nil, want a per-operation outcome", i)
			continue
		}
		if !op.Result.Success {
			t.Errorf("Operations[%d].Result.Success = false, want true", i)
		}
	}
	if got.FeeCharged == 0 {
		t.Error("FeeCharged = 0, want the charged fee from the result")
	}
}

func TestExplainRejectsUnsupportedTypes(t *testing.T) {
	t.Parallel()

	v, err := lens.DecodeAs("AAAAAA==", "Memo")
	if err != nil {
		t.Fatalf("DecodeAs() error = %v", err)
	}
	if _, err := lens.Explain(v); err == nil {
		t.Error("Explain(Memo) error = nil, want an error for an unsupported type")
	}
}

func TestExplainPairRejectsMismatchedArguments(t *testing.T) {
	t.Parallel()

	env, err := lens.Decode(loadFixture(t, "tx_failed.env"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	res, err := lens.Decode(loadFixture(t, "tx_failed.res"))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	tests := []struct {
		name          string
		first, second *lens.Value
	}{
		{"swapped order", res, env},
		{"nil first", nil, res},
		{"nil second", env, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := lens.ExplainPair(tc.first, tc.second); err == nil {
				t.Error("ExplainPair() error = nil, want an error")
			}
		})
	}
}

func TestExplainCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		constant    string
		wantCode    string
		wantSuccess bool
		wantHint    bool
	}{
		{
			name:        "transaction success",
			constant:    "TransactionResultCodeTxSuccess",
			wantCode:    "tx_success",
			wantSuccess: true,
		},
		{
			name:     "bad sequence carries a hint",
			constant: "TransactionResultCodeTxBadSeq",
			wantCode: "tx_bad_seq",
			wantHint: true,
		},
		{
			name:     "payment underfunded",
			constant: "PaymentResultCodePaymentUnderfunded",
			wantCode: "payment_underfunded",
			wantHint: true,
		},
		{
			name:     "soroban trap",
			constant: "InvokeHostFunctionResultCodeInvokeHostFunctionTrapped",
			wantCode: "invoke_host_function_trapped",
			wantHint: true,
		},
		{
			name:     "unknown code degrades gracefully",
			constant: "SomeFutureResultCodeSomeFutureThing",
			wantCode: "some_future_thing",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := lens.ExplainCode(tc.constant)
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			if got.Success != tc.wantSuccess {
				t.Errorf("Success = %v, want %v", got.Success, tc.wantSuccess)
			}
			if got.Summary == "" {
				t.Error("Summary is empty; every code must say something useful")
			}
			if tc.wantHint && got.Hint == "" {
				t.Error("Hint is empty, want actionable advice for this code")
			}
			if got.Constant != tc.constant {
				t.Errorf("Constant = %q, want %q", got.Constant, tc.constant)
			}
		})
	}
}

// TestExplainCodeNeverRestatesTheConstant guards against the failure mode
// where an "explanation" just re-spells the code name, which would make the
// whole feature worthless.
func TestExplainCodeNeverRestatesTheConstant(t *testing.T) {
	t.Parallel()

	samples := []string{
		"TransactionResultCodeTxBadSeq",
		"PaymentResultCodePaymentUnderfunded",
		"ChangeTrustResultCodeChangeTrustLowReserve",
		"AccountMergeResultCodeAccountMergeHasSubEntries",
	}

	for _, c := range samples {
		got := lens.ExplainCode(c)
		if len(got.Summary) < 25 {
			t.Errorf("ExplainCode(%q).Summary = %q, too short to be an explanation", c, got.Summary)
		}
		if strings.EqualFold(strings.ReplaceAll(got.Summary, " ", ""), got.Code) {
			t.Errorf("ExplainCode(%q).Summary merely restates the code", c)
		}
	}
}

// manageDataEnvelope builds a one-operation envelope for a manage_data set
// (value non-nil) or delete (value nil).
func manageDataEnvelope(t *testing.T, key string, value *xdr.DataValue) *xdr.TransactionEnvelope {
	t.Helper()
	return &xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: mustMuxed(t, testSource),
				Fee:           xdr.Uint32(100),
				SeqNum:        xdr.SequenceNumber(12345),
				Cond:          xdr.Preconditions{Type: xdr.PreconditionTypePrecondNone},
				Operations: []xdr.Operation{{
					Body: xdr.OperationBody{
						Type:         xdr.OperationTypeManageData,
						ManageDataOp: &xdr.ManageDataOp{DataName: xdr.String64(key), DataValue: value},
					},
				}},
			},
		},
	}
}

// manageDataFailureResult builds a failed transaction result whose single
// manage_data operation produced code.
func manageDataFailureResult(code xdr.ManageDataResultCode) *xdr.TransactionResult {
	results := []xdr.OperationResult{{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type:             xdr.OperationTypeManageData,
			ManageDataResult: &xdr.ManageDataResult{Code: code},
		},
	}}
	return &xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &results,
		},
	}
}

func explainSyntheticPair(t *testing.T, env, res any) *lens.Summary {
	t.Helper()
	envVal, err := lens.Decode(mustMarshal(t, env))
	if err != nil {
		t.Fatalf("Decode(envelope) error = %v", err)
	}
	resVal, err := lens.Decode(mustMarshal(t, res))
	if err != nil {
		t.Fatalf("Decode(result) error = %v", err)
	}
	got, err := lens.ExplainPair(envVal, resVal)
	if err != nil {
		t.Fatalf("ExplainPair() error = %v", err)
	}
	return got
}

// TestExplainManageDataFailureContext covers the diagnostics that a result
// code alone cannot carry: the entry key from the envelope, and the reserve a
// new subentry locks.
func TestExplainManageDataFailureContext(t *testing.T) {
	t.Parallel()

	setValue := xdr.DataValue([]byte("enabled"))

	tests := []struct {
		name        string
		code        xdr.ManageDataResultCode
		value       *xdr.DataValue
		wantCode    string
		wantContext []string
		wantHint    string
	}{
		{
			name:        "low reserve names the key and the 0.5 XLM subentry reserve",
			code:        xdr.ManageDataResultCodeManageDataLowReserve,
			value:       &setValue,
			wantCode:    "manage_data_low_reserve",
			wantContext: []string{`"erc20:config"`, "0.5 XLM", "subentry", "reserve"},
			wantHint:    "0.5 XLM",
		},
		{
			name:        "name not found names the entry being deleted",
			code:        xdr.ManageDataResultCodeManageDataNameNotFound,
			value:       nil,
			wantCode:    "manage_data_name_not_found",
			wantContext: []string{`"erc20:config"`, "nothing to delete"},
		},
		{
			name:        "invalid name states the 1-64 byte limit",
			code:        xdr.ManageDataResultCodeManageDataInvalidName,
			value:       &setValue,
			wantCode:    "manage_data_invalid_name",
			wantContext: []string{`"erc20:config"`, "1-64 bytes"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env := manageDataEnvelope(t, "erc20:config", tc.value)
			got := explainSyntheticPair(t, env, manageDataFailureResult(tc.code))

			if len(got.Operations) != 1 {
				t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
			}
			op := got.Operations[0]
			if op.Type != "manage_data" {
				t.Fatalf("Operations[0].Type = %q, want manage_data", op.Type)
			}
			if op.Result == nil {
				t.Fatal("Operations[0].Result is nil")
			}
			if op.Result.Code != tc.wantCode {
				t.Errorf("Result.Code = %q, want %q", op.Result.Code, tc.wantCode)
			}
			if op.Result.Context == "" {
				t.Fatal("Result.Context is empty, want entry-key diagnostics from the envelope")
			}
			for _, want := range tc.wantContext {
				if !strings.Contains(op.Result.Context, want) {
					t.Errorf("Result.Context = %q, want it to contain %q", op.Result.Context, want)
				}
			}
			if tc.wantHint != "" && !strings.Contains(op.Result.Hint, tc.wantHint) {
				t.Errorf("Result.Hint = %q, want it to contain %q", op.Result.Hint, tc.wantHint)
			}

			// The headline is the first thing a developer reads, so it must
			// already name the entry the failure was about.
			if !strings.Contains(got.Headline, "erc20:config") {
				t.Errorf("Headline = %q, want it to name the entry key", got.Headline)
			}
		})
	}
}

// TestExplainManageDataResultOnlyHasNoContext pins the offline-first contract:
// without an envelope the key simply is not available, and lens must not invent
// one.
func TestExplainManageDataResultOnlyHasNoContext(t *testing.T) {
	t.Parallel()

	resVal, err := lens.Decode(mustMarshal(t, manageDataFailureResult(xdr.ManageDataResultCodeManageDataLowReserve)))
	if err != nil {
		t.Fatalf("Decode(result) error = %v", err)
	}
	got, err := lens.Explain(resVal)
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if len(got.Operations) != 1 || got.Operations[0].Result == nil {
		t.Fatalf("Operations = %+v, want one operation with a result", got.Operations)
	}
	if got.Operations[0].Result.Context != "" {
		t.Errorf("Result.Context = %q, want empty without an envelope", got.Operations[0].Result.Context)
	}
}

// TestExplainSecondaryDescriptions covers the describeOperation branches that
// the common fixtures do not exercise: optional set-options fields, the
// non-invoke Soroban host functions, and the v2 precondition extras.
func TestExplainSecondaryDescriptions(t *testing.T) {
	t.Parallel()

	native := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
	hash := xdr.Hash{0x01}

	tests := []struct {
		name        string
		op          xdr.Operation
		cond        xdr.Preconditions
		wantDetails []string
		wantPrecond []string
	}{
		{
			name: "set options with every field",
			op: xdr.Operation{Body: xdr.OperationBody{
				Type: xdr.OperationTypeSetOptions,
				SetOptionsOp: &xdr.SetOptionsOp{
					InflationDest: ptr(mustAccountID(t, testDest)),
					ClearFlags:    ptr(xdr.Uint32(1)),
					SetFlags:      ptr(xdr.Uint32(2)),
					MasterWeight:  ptr(xdr.Uint32(3)),
					LowThreshold:  ptr(xdr.Uint32(1)),
					MedThreshold:  ptr(xdr.Uint32(2)),
					HighThreshold: ptr(xdr.Uint32(3)),
					HomeDomain:    ptr(xdr.String32("example.org")),
				},
			}},
			wantDetails: []string{
				"set inflation destination", "clear flags 1", "set flags 2",
				"master weight 3", "low threshold 1", "medium threshold 2",
				"high threshold 3", "home domain example.org",
			},
		},
		{
			name: "set options with no changes",
			op: xdr.Operation{Body: xdr.OperationBody{
				Type:         xdr.OperationTypeSetOptions,
				SetOptionsOp: &xdr.SetOptionsOp{},
			}},
			wantDetails: []string{"set options (no changes)"},
		},
		{
			name: "create contract host function",
			op: xdr.Operation{Body: xdr.OperationBody{
				Type: xdr.OperationTypeInvokeHostFunction,
				InvokeHostFunctionOp: &xdr.InvokeHostFunctionOp{
					HostFunction: xdr.HostFunction{
						Type: xdr.HostFunctionTypeHostFunctionTypeCreateContract,
						CreateContract: &xdr.CreateContractArgs{
							ContractIdPreimage: xdr.ContractIdPreimage{
								Type:      xdr.ContractIdPreimageTypeContractIdPreimageFromAsset,
								FromAsset: &native,
							},
							Executable: xdr.ContractExecutable{
								Type:     xdr.ContractExecutableTypeContractExecutableWasm,
								WasmHash: &hash,
							},
						},
					},
				},
			}},
			wantDetails: []string{"create a contract instance"},
		},
		{
			name: "upload contract wasm host function",
			op: xdr.Operation{Body: xdr.OperationBody{
				Type: xdr.OperationTypeInvokeHostFunction,
				InvokeHostFunctionOp: &xdr.InvokeHostFunctionOp{
					HostFunction: xdr.HostFunction{
						Type: xdr.HostFunctionTypeHostFunctionTypeUploadContractWasm,
						Wasm: &[]byte{0x00, 0x61},
					},
				},
			}},
			wantDetails: []string{"upload contract wasm (2 bytes)"},
		},
		{
			name: "v2 preconditions extras",
			op: xdr.Operation{Body: xdr.OperationBody{
				Type:           xdr.OperationTypeBumpSequence,
				BumpSequenceOp: &xdr.BumpSequenceOp{BumpTo: 1},
			}},
			cond: xdr.Preconditions{
				Type: xdr.PreconditionTypePrecondV2,
				V2: &xdr.PreconditionsV2{
					LedgerBounds:    &xdr.LedgerBounds{MinLedger: 10, MaxLedger: 20},
					MinSeqNum:       ptr(xdr.SequenceNumber(5)),
					MinSeqAge:       xdr.Duration(60),
					MinSeqLedgerGap: xdr.Uint32(2),
					ExtraSigners: []xdr.SignerKey{{
						Type:    xdr.SignerKeyTypeSignerKeyTypeEd25519,
						Ed25519: &xdr.Uint256{0x01},
					}},
				},
			},
			wantPrecond: []string{
				"ledger bounds 10..20",
				"minimum sequence number 5",
				"minimum sequence age 60s",
				"minimum sequence ledger gap 2",
				"1 extra signer(s) required",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cond := tc.cond
			if cond.Type == xdr.PreconditionTypePrecondNone && cond.V2 == nil {
				cond = noCond()
			}
			v, err := lens.DecodeAs(buildEnvelope(t, noMemo(), cond, tc.op), "TransactionEnvelope")
			if err != nil {
				t.Fatalf("DecodeAs() error = %v", err)
			}
			got, err := lens.Explain(v)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}

			if len(got.Operations) != 1 {
				t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
			}
			for _, want := range tc.wantDetails {
				if !strings.Contains(got.Operations[0].Detail, want) {
					t.Errorf("Detail = %q, want it to contain %q", got.Operations[0].Detail, want)
				}
			}
			preconds := strings.Join(got.Preconds, "; ")
			for _, want := range tc.wantPrecond {
				if !strings.Contains(preconds, want) {
					t.Errorf("Preconds = %q, want it to contain %q", preconds, want)
				}
			}
		})
	}
}

// TestExplainFailureContextIsOperationSpecific guards the switch that attaches
// manage_data context: other operation types must be left untouched.
func TestExplainFailureContextIsOperationSpecific(t *testing.T) {
	t.Parallel()

	env := &xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: mustMuxed(t, testSource),
				Fee:           xdr.Uint32(100),
				SeqNum:        xdr.SequenceNumber(12345),
				Cond:          xdr.Preconditions{Type: xdr.PreconditionTypePrecondNone},
				Operations: []xdr.Operation{{
					Body: xdr.OperationBody{
						Type: xdr.OperationTypePayment,
						PaymentOp: &xdr.PaymentOp{
							Destination: mustMuxed(t, testSource),
							Asset:       xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
							Amount:      1,
						},
					},
				}},
			},
		},
	}
	results := []xdr.OperationResult{{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type:          xdr.OperationTypePayment,
			PaymentResult: &xdr.PaymentResult{Code: xdr.PaymentResultCodePaymentUnderfunded},
		},
	}}
	res := &xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &results,
		},
	}

	got := explainSyntheticPair(t, env, res)
	if len(got.Operations) != 1 || got.Operations[0].Result == nil {
		t.Fatalf("Operations = %+v, want one operation with a result", got.Operations)
	}
	if got.Operations[0].Result.Context != "" {
		t.Errorf("Result.Context = %q, want empty for a payment failure", got.Operations[0].Result.Context)
	}
}

// claimableBalanceClaimant builds a v0 claimant for synthetic claimable
// balance operations.
func claimableBalanceClaimant(t *testing.T, destination string, predicate xdr.ClaimPredicate) xdr.Claimant {
	t.Helper()
	return xdr.Claimant{
		Type: xdr.ClaimantTypeClaimantTypeV0,
		V0: &xdr.ClaimantV0{
			Destination: mustAccountID(t, destination),
			Predicate:   predicate,
		},
	}
}

func unconditionalClaimant(t *testing.T, destination string) xdr.Claimant {
	t.Helper()
	return claimableBalanceClaimant(t, destination, xdr.ClaimPredicate{
		Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional,
	})
}

// createClaimableBalanceEnvelope builds a one-operation envelope carrying a
// create_claimable_balance operation with the supplied arguments.
func createClaimableBalanceEnvelope(t *testing.T, asset xdr.Asset, amount xdr.Int64, claimants ...xdr.Claimant) *xdr.TransactionEnvelope {
	t.Helper()
	return &xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: mustMuxed(t, testSource),
				Fee:           xdr.Uint32(100),
				SeqNum:        xdr.SequenceNumber(12345),
				Cond:          noCond(),
				Operations: []xdr.Operation{{
					Body: xdr.OperationBody{
						Type: xdr.OperationTypeCreateClaimableBalance,
						CreateClaimableBalanceOp: &xdr.CreateClaimableBalanceOp{
							Asset:     asset,
							Amount:    amount,
							Claimants: claimants,
						},
					},
				}},
			},
		},
	}
}

// createClaimableBalanceFailureResult builds a failed transaction result whose
// single create-claimable-balance operation produced code.
func createClaimableBalanceFailureResult(code xdr.CreateClaimableBalanceResultCode) *xdr.TransactionResult {
	results := []xdr.OperationResult{{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type:                         xdr.OperationTypeCreateClaimableBalance,
			CreateClaimableBalanceResult: &xdr.CreateClaimableBalanceResult{Code: code},
		},
	}}
	return &xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &results,
		},
	}
}

// shortAddress mirrors the inline abbreviation lens uses in prose.
func shortAddress(addr string) string {
	if len(addr) <= 12 {
		return addr
	}
	return addr[:5] + "…" + addr[len(addr)-4:]
}

// TestClaimableBalanceReserve pins the protocol's reserve formula: two base
// reserves for the entry plus one per claimant, at 0.5 XLM each.
func TestClaimableBalanceReserve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		claimants int
		want      int64
	}{
		{-1, 10_000_000},
		{0, 10_000_000},
		{1, 15_000_000},
		{2, 20_000_000},
		{10, 60_000_000},
	}
	for _, tc := range tests {
		if got := lens.ClaimableBalanceReserve(tc.claimants); got != tc.want {
			t.Errorf("ClaimableBalanceReserve(%d) = %d stroops, want %d", tc.claimants, got, tc.want)
		}
	}
}

// TestExplainCreateClaimableBalanceFailureContext covers the diagnostics that
// the result code alone cannot carry: which asset, amount and claimants the
// operation was about, which account needs the trustline, and how much reserve
// the balance locks.
func TestExplainCreateClaimableBalanceFailureContext(t *testing.T) {
	t.Parallel()

	usdc := creditAsset(t, "USDC", testIssuer)
	native := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
	unconditionalDest := unconditionalClaimant(t, testDest)
	unconditionalIssuer := unconditionalClaimant(t, testIssuer)

	// A one-child AND predicate is malformed: the protocol requires exactly
	// two operands.
	oneChild := []xdr.ClaimPredicate{{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional}}
	badPredicate := claimableBalanceClaimant(t, testDest, xdr.ClaimPredicate{
		Type:          xdr.ClaimPredicateTypeClaimPredicateAnd,
		AndPredicates: &oneChild,
	})

	tests := []struct {
		name        string
		asset       xdr.Asset
		amount      xdr.Int64
		claimants   []xdr.Claimant
		code        xdr.CreateClaimableBalanceResultCode
		wantCode    string
		wantContext []string
		wantHint    string
	}{
		{
			name:      "low reserve states the claimant count and reserve formula",
			asset:     usdc,
			amount:    10_0000000,
			claimants: []xdr.Claimant{unconditionalDest, unconditionalIssuer},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceLowReserve,
			wantCode:  "create_claimable_balance_low_reserve",
			wantContext: []string{
				"USDC:", "2 claimant", "(2 + 2)", "0.5 XLM", "2 XLM",
				testSource, "reserve",
			},
			wantHint: "two base reserves",
		},
		{
			name:      "no trust names the source account and asset",
			asset:     usdc,
			amount:    10_0000000,
			claimants: []xdr.Claimant{unconditionalDest},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceNoTrust,
			wantCode:  "create_claimable_balance_no_trust",
			wantContext: []string{
				"USDC:", "trustline", "change-trust",
				testSource, "claim",
			},
			wantHint: "trustline",
		},
		{
			name:      "not authorized names the issuer requirement",
			asset:     usdc,
			amount:    10_0000000,
			claimants: []xdr.Claimant{unconditionalDest},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceNotAuthorized,
			wantCode:  "create_claimable_balance_not_authorized",
			wantContext: []string{
				"USDC:", "not authorised", "issuer",
				testSource,
			},
			wantHint: "authorise",
		},
		{
			name:      "underfunded names the amount, asset and claimants",
			asset:     usdc,
			amount:    25_0000000,
			claimants: []xdr.Claimant{unconditionalDest, unconditionalIssuer},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceUnderfunded,
			wantCode:  "create_claimable_balance_underfunded",
			wantContext: []string{
				"25.0000000", "USDC:", testDest,
				testIssuer, "trustline",
			},
			wantHint: "Fund",
		},
		{
			name:      "underfunded native explains available balance",
			asset:     native,
			amount:    5_0000000,
			claimants: []xdr.Claimant{unconditionalDest},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceUnderfunded,
			wantCode:  "create_claimable_balance_underfunded",
			wantContext: []string{
				"XLM", "spendable", "selling liabilities", "minimum reserve",
			},
			wantHint: "Fund",
		},
		{
			name:      "malformed names a duplicate destination",
			asset:     usdc,
			amount:    10_0000000,
			claimants: []xdr.Claimant{unconditionalDest, unconditionalDest},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceMalformed,
			wantCode:  "create_claimable_balance_malformed",
			wantContext: []string{
				"malformed", testDest, "more than once",
			},
		},
		{
			name:      "malformed rejects a non-positive amount",
			asset:     usdc,
			amount:    0,
			claimants: []xdr.Claimant{unconditionalDest},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceMalformed,
			wantCode:  "create_claimable_balance_malformed",
			wantContext: []string{
				"malformed", "amount must be positive",
			},
		},
		{
			name:      "malformed rejects an empty claimant list",
			asset:     usdc,
			amount:    10_0000000,
			claimants: nil,
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceMalformed,
			wantCode:  "create_claimable_balance_malformed",
			wantContext: []string{
				"malformed", "no claimants", "between 1 and 10",
			},
		},
		{
			name:      "malformed names the claimant with an invalid predicate",
			asset:     usdc,
			amount:    10_0000000,
			claimants: []xdr.Claimant{badPredicate},
			code:      xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceMalformed,
			wantCode:  "create_claimable_balance_malformed",
			wantContext: []string{
				testDest, "invalid predicate", "exactly 2 children",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env := createClaimableBalanceEnvelope(t, tc.asset, tc.amount, tc.claimants...)
			got := explainSyntheticPair(t, env, createClaimableBalanceFailureResult(tc.code))

			if len(got.Operations) != 1 {
				t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
			}
			op := got.Operations[0]
			if op.Type != "create_claimable_balance" {
				t.Fatalf("Operations[0].Type = %q, want create_claimable_balance", op.Type)
			}
			if op.Result == nil {
				t.Fatal("Operations[0].Result is nil")
			}
			if op.Result.Code != tc.wantCode {
				t.Errorf("Result.Code = %q, want %q", op.Result.Code, tc.wantCode)
			}
			if op.Result.Context == "" {
				t.Fatal("Result.Context is empty, want envelope-derived diagnostics")
			}
			for _, want := range tc.wantContext {
				if !strings.Contains(op.Result.Context, want) {
					t.Errorf("Result.Context = %q, want it to contain %q", op.Result.Context, want)
				}
			}
			if tc.wantHint != "" && !strings.Contains(op.Result.Hint, tc.wantHint) {
				t.Errorf("Result.Hint = %q, want it to contain %q", op.Result.Hint, tc.wantHint)
			}
			if !strings.Contains(got.Headline, "create_claimable_balance") {
				t.Errorf("Headline = %q, want it to name the failing operation", got.Headline)
			}
		})
	}
}

// TestExplainCreateClaimableBalanceDetail verifies that the operation
// description, which feeds the headline, names the asset and claimants even
// when no result was supplied.
func TestExplainCreateClaimableBalanceDetail(t *testing.T) {
	t.Parallel()

	env := createClaimableBalanceEnvelope(t,
		creditAsset(t, "USDC", testIssuer),
		10_0000000,
		unconditionalClaimant(t, testDest),
		unconditionalClaimant(t, testIssuer),
	)
	got := explainSyntheticPair(t, env, createClaimableBalanceFailureResult(
		xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceNoTrust))

	detail := got.Operations[0].Detail
	for _, want := range []string{"10.0000000", "USDC:", shortAddress(testDest), shortAddress(testIssuer)} {
		if !strings.Contains(detail, want) {
			t.Errorf("Detail = %q, want it to contain %q", detail, want)
		}
	}
	if !strings.Contains(got.Headline, "USDC:") {
		t.Errorf("Headline = %q, want it to name the asset", got.Headline)
	}
}

// TestExplainCreateClaimableBalanceResultOnlyHasNoContext pins the
// offline-first contract: without an envelope there is no asset or claimant to
// name, and lens must not invent one.
func TestExplainCreateClaimableBalanceResultOnlyHasNoContext(t *testing.T) {
	t.Parallel()

	resVal, err := lens.Decode(mustMarshal(t, createClaimableBalanceFailureResult(
		xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceLowReserve)))
	if err != nil {
		t.Fatalf("Decode(result) error = %v", err)
	}
	got, err := lens.Explain(resVal)
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if len(got.Operations) != 1 || got.Operations[0].Result == nil {
		t.Fatalf("Operations = %+v, want one operation with a result", got.Operations)
	}
	if got.Operations[0].Result.Context != "" {
		t.Errorf("Result.Context = %q, want empty without an envelope", got.Operations[0].Result.Context)
	}
}

// TestExplainMalformedClaimableBalancePredicates exercises each structural
// predicate rule separately, since a malformed result names the claimant whose
// predicate failed rather than only the operation as a whole.
func TestExplainMalformedClaimableBalancePredicates(t *testing.T) {
	t.Parallel()

	absNegative := xdr.Int64(-1)
	relNegative := xdr.Int64(-1)
	emptyChildren := []xdr.ClaimPredicate{}
	var nilChild *xdr.ClaimPredicate

	var deep xdr.ClaimPredicate
	deep = xdr.ClaimPredicate{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional}
	for range 5 {
		inner := deep
		innerPtr := &inner
		deep = xdr.ClaimPredicate{
			Type:         xdr.ClaimPredicateTypeClaimPredicateNot,
			NotPredicate: &innerPtr,
		}
	}

	tests := []struct {
		name      string
		predicate xdr.ClaimPredicate
		want      string
	}{
		{
			name: "negative absolute time",
			predicate: xdr.ClaimPredicate{
				Type:      xdr.ClaimPredicateTypeClaimPredicateBeforeAbsoluteTime,
				AbsBefore: &absNegative,
			},
			want: "absolute-time predicate cannot be negative",
		},
		{
			name: "negative relative time",
			predicate: xdr.ClaimPredicate{
				Type:      xdr.ClaimPredicateTypeClaimPredicateBeforeRelativeTime,
				RelBefore: &relNegative,
			},
			want: "relative-time predicate cannot be negative",
		},
		{
			name: "not without a child",
			predicate: xdr.ClaimPredicate{
				Type:         xdr.ClaimPredicateTypeClaimPredicateNot,
				NotPredicate: &nilChild,
			},
			want: "not predicate has no child",
		},
		{
			name: "and without children",
			predicate: xdr.ClaimPredicate{
				Type:          xdr.ClaimPredicateTypeClaimPredicateAnd,
				AndPredicates: &emptyChildren,
			},
			want: "exactly 2 children, not 0",
		},
		{
			name:      "nesting deeper than the protocol limit",
			predicate: deep,
			want:      "at most 4 levels deep",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			claimant := claimableBalanceClaimant(t, testDest, tc.predicate)
			env := createClaimableBalanceEnvelope(t,
				creditAsset(t, "USDC", testIssuer), 10_0000000, claimant)
			got := explainSyntheticPair(t, env, createClaimableBalanceFailureResult(
				xdr.CreateClaimableBalanceResultCodeCreateClaimableBalanceMalformed))

			context := got.Operations[0].Result.Context
			for _, want := range []string{testDest, "invalid predicate", tc.want} {
				if !strings.Contains(context, want) {
					t.Errorf("Result.Context = %q, want it to contain %q", context, want)
				}
			}
		})
	}
}
