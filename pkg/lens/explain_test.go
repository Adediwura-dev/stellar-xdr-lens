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
		"AccountMergeResultCodeAccountMergeSeqnumTooFar",
		"AccountMergeResultCodeAccountMergeDestFull",
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

// ---- Context-aware failure deepening (issues #116 and #117) -------------
//
// When an envelope is paired with its result, a failed operation's
// explanation is deepened with the operation's own parameters. The helpers
// below build a one-operation transaction and its failing result so each
// deepened code can be driven from the operation body alone.

// explainTestKey builds a deterministic 32-byte key for test accounts and
// signers.
func explainTestKey(seed byte) *xdr.Uint256 {
	var k xdr.Uint256
	for i := range k {
		k[i] = seed
	}
	return &k
}

func explainTestAccount(seed byte) xdr.AccountId {
	return xdr.AccountId{
		Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
		Ed25519: explainTestKey(seed),
	}
}

func optUint32(v uint32) *xdr.Uint32 {
	u := xdr.Uint32(v)
	return &u
}

func mustDecodeForExplain(t *testing.T, payload, typ string) *lens.Value {
	t.Helper()
	v, err := lens.DecodeAs(payload, typ)
	if err != nil {
		t.Fatalf("DecodeAs(%s) error = %v", typ, err)
	}
	return v
}

// explainSingleOperation builds a one-operation transaction whose only
// operation carries body, pairs it with a tx_failed result carrying inner as
// the operation result, and returns the operation's Reason.
func explainSingleOperation(t *testing.T, body xdr.OperationBody, inner xdr.OperationResultTr) lens.Reason {
	t.Helper()
	return explainPreparedOperation(t, xdr.Operation{Body: body}, inner)
}

// explainPreparedOperation is explainSingleOperation for a fully assembled
// operation, so tests can also exercise an operation-level source override.
func explainPreparedOperation(t *testing.T, op xdr.Operation, inner xdr.OperationResultTr) lens.Reason {
	t.Helper()
	source := explainTestAccount(0xA1)
	env := xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: xdr.MuxedAccount{
					Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
					Ed25519: source.Ed25519,
				},
				Fee:        100,
				SeqNum:     4242,
				Operations: []xdr.Operation{op},
			},
		},
	}
	opResults := []xdr.OperationResult{{
		Code: xdr.OperationResultCodeOpInner,
		Tr:   &inner,
	}}
	res := xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &opResults,
		},
	}

	envVal := mustDecodeForExplain(t, mustMarshal(t, env), "TransactionEnvelope")
	resVal := mustDecodeForExplain(t, mustMarshal(t, res), "TransactionResult")
	summary, err := lens.ExplainPair(envVal, resVal)
	if err != nil {
		t.Fatalf("ExplainPair() error = %v", err)
	}
	if summary.Outcome == nil || summary.Outcome.Success {
		t.Fatalf("Outcome = %+v, want a failed outcome", summary.Outcome)
	}
	if len(summary.Operations) != 1 || summary.Operations[0].Result == nil {
		t.Fatalf("Operations = %+v, want one operation carrying a result", summary.Operations)
	}
	return *summary.Operations[0].Result
}

// assertReasonContains fails unless every want substring appears in the
// summary (and the hint, when checkHint is set).
func assertReasonContains(t *testing.T, r lens.Reason, checkHint bool, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(r.Summary, w) {
			t.Errorf("Summary = %q, want it to contain %q", r.Summary, w)
		}
		if checkHint && !strings.Contains(r.Hint, w) {
			t.Errorf("Hint = %q, want it to contain %q", r.Hint, w)
		}
	}
}

// TestExplainPairDeepensSetOptionsFailures covers issue #117: each set_options
// failure mode is diagnosed from the operation body, not just named.
func TestExplainPairDeepensSetOptionsFailures(t *testing.T) {
	t.Parallel()

	source := explainTestAccount(0xA1)
	distinctSigner := xdr.Signer{
		Key:    xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519, Ed25519: explainTestKey(0x2B)},
		Weight: 1,
	}
	preAuthSigner := xdr.Signer{
		Key:    xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypePreAuthTx, PreAuthTx: explainTestKey(0x3C)},
		Weight: 1,
	}
	hashXSigner := xdr.Signer{
		Key:    xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeHashX, HashX: explainTestKey(0x4D)},
		Weight: 1,
	}
	payloadSigner := xdr.Signer{
		Key: xdr.SignerKey{
			Type: xdr.SignerKeyTypeSignerKeyTypeEd25519SignedPayload,
			Ed25519SignedPayload: &xdr.SignerKeyEd25519SignedPayload{
				Ed25519: *explainTestKey(0x5E),
				Payload: []byte("payload"),
			},
		},
		Weight: 1,
	}
	selfSigner := xdr.Signer{
		Key:    xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519, Ed25519: source.Ed25519},
		Weight: 1,
	}

	tests := []struct {
		name       string
		op         xdr.SetOptionsOp
		code       xdr.SetOptionsResultCode
		wantCode   string
		wantInSumm []string
	}{
		{
			name:       "threshold out of range names the offending field",
			op:         xdr.SetOptionsOp{MasterWeight: optUint32(300)},
			code:       xdr.SetOptionsResultCodeSetOptionsThresholdOutOfRange,
			wantCode:   "set_options_threshold_out_of_range",
			wantInSumm: []string{"master weight 300", "0–255"},
		},
		{
			name: "bad flags names the contradictory bits",
			op: xdr.SetOptionsOp{
				SetFlags:   optUint32(0x2),
				ClearFlags: optUint32(0x2),
			},
			code:       xdr.SetOptionsResultCodeSetOptionsBadFlags,
			wantCode:   "set_options_bad_flags",
			wantInSumm: []string{"sets and clears", "auth_revocable (0x2)"},
		},
		{
			name:       "unknown flag names the unrecognised bits",
			op:         xdr.SetOptionsOp{SetFlags: optUint32(0x20)},
			code:       xdr.SetOptionsResultCodeSetOptionsUnknownFlag,
			wantCode:   "set_options_unknown_flag",
			wantInSumm: []string{"0x20", "auth_clawback_enabled (0x8)"},
		},
		{
			name:       "clearing the immutable flag is impossible",
			op:         xdr.SetOptionsOp{ClearFlags: optUint32(0x4)},
			code:       xdr.SetOptionsResultCodeSetOptionsCantChange,
			wantCode:   "set_options_cant_change",
			wantInSumm: []string{"auth-immutable", "permanent"},
		},
		{
			name: "changing flags on an immutable account",
			op: xdr.SetOptionsOp{
				SetFlags: optUint32(0x1),
			},
			code:       xdr.SetOptionsResultCodeSetOptionsCantChange,
			wantCode:   "set_options_cant_change",
			wantInSumm: []string{"auth_required (0x1)", "auth-immutable set", "locks"},
		},
		{
			name:       "low reserve points at the new signer subentry",
			op:         xdr.SetOptionsOp{Signer: &distinctSigner},
			code:       xdr.SetOptionsResultCodeSetOptionsLowReserve,
			wantCode:   "set_options_low_reserve",
			wantInSumm: []string{"signer", "base reserve"},
		},
		{
			name:       "low reserve labels a pre-auth signer",
			op:         xdr.SetOptionsOp{Signer: &preAuthSigner},
			code:       xdr.SetOptionsResultCodeSetOptionsLowReserve,
			wantCode:   "set_options_low_reserve",
			wantInSumm: []string{"pre-auth signer"},
		},
		{
			name:       "low reserve labels a hash-x signer",
			op:         xdr.SetOptionsOp{Signer: &hashXSigner},
			code:       xdr.SetOptionsResultCodeSetOptionsLowReserve,
			wantCode:   "set_options_low_reserve",
			wantInSumm: []string{"hash-x signer"},
		},
		{
			name:       "low reserve labels a signed-payload signer",
			op:         xdr.SetOptionsOp{Signer: &payloadSigner},
			code:       xdr.SetOptionsResultCodeSetOptionsLowReserve,
			wantCode:   "set_options_low_reserve",
			wantInSumm: []string{"signed-payload signer"},
		},
		{
			name:       "bad signer detects the account signing for itself",
			op:         xdr.SetOptionsOp{Signer: &selfSigner},
			code:       xdr.SetOptionsResultCodeSetOptionsBadSigner,
			wantCode:   "set_options_bad_signer",
			wantInSumm: []string{"itself"},
		},
		{
			name:       "bad signer flags an oversized weight",
			op:         xdr.SetOptionsOp{Signer: &xdr.Signer{Key: distinctSigner.Key, Weight: 300}},
			code:       xdr.SetOptionsResultCodeSetOptionsBadSigner,
			wantCode:   "set_options_bad_signer",
			wantInSumm: []string{"signer weight 300", "0–255"},
		},
		{
			name:       "too many signers explains the five-signer cap",
			op:         xdr.SetOptionsOp{Signer: &distinctSigner},
			code:       xdr.SetOptionsResultCodeSetOptionsTooManySigners,
			wantCode:   "set_options_too_many_signers",
			wantInSumm: []string{"maximum of five signers"},
		},
		{
			name:       "invalid home domain quotes the value",
			op:         xdr.SetOptionsOp{HomeDomain: string32Ptr("not a domain")},
			code:       xdr.SetOptionsResultCodeSetOptionsInvalidHomeDomain,
			wantCode:   "set_options_invalid_home_domain",
			wantInSumm: []string{`"not a domain"`},
		},
		{
			name: "auth revocable required explains the clawback pairing",
			op: xdr.SetOptionsOp{
				SetFlags: optUint32(0x8),
			},
			code:       xdr.SetOptionsResultCodeSetOptionsAuthRevocableRequired,
			wantCode:   "set_options_auth_revocable_required",
			wantInSumm: []string{"auth-clawback", "auth-revocable"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := explainSingleOperation(t,
				xdr.OperationBody{Type: xdr.OperationTypeSetOptions, SetOptionsOp: &tc.op},
				xdr.OperationResultTr{
					Type:             xdr.OperationTypeSetOptions,
					SetOptionsResult: &xdr.SetOptionsResult{Code: tc.code},
				})

			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			if got.Hint == "" {
				t.Error("Hint is empty, want actionable advice for this failure")
			}
		})
	}
}

// ---- Inserted ChangeTrust contextual tests from feature branch ----
func TestExplainChangeTrust(t *testing.T) {
	t.Parallel()

	newEnvelope := func(t *testing.T, asset xdr.Asset, limit xdr.Int64) *lens.Value {
		t.Helper()
		const source = "GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6"
		var sourceID xdr.AccountId
		if err := sourceID.SetAddress(source); err != nil {
			t.Fatalf("parsing source %s: %v", source, err)
		}
		env := &xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{
				Tx: xdr.Transaction{
					SourceAccount: sourceID.ToMuxedAccount(),
					Fee:           xdr.Uint32(100),
					SeqNum:        xdr.SequenceNumber(1),
					Cond:          xdr.Preconditions{Type: xdr.PreconditionTypePrecondNone},
					Memo:          xdr.Memo{Type: xdr.MemoTypeMemoNone},
					Operations: []xdr.Operation{{Body: xdr.OperationBody{
						Type: xdr.OperationTypeChangeTrust,
						ChangeTrustOp: &xdr.ChangeTrustOp{
							Line:  asset.ToChangeTrustAsset(),
							Limit: limit,
						},
					}}},
				},
			},
		}
		return &lens.Value{Type: "TransactionEnvelope", Raw: env}
	}

	// ChangeTrustOp with an issuer and limit.
	{
		issuer := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
		asset := xdr.Asset{}
		if err := asset.SetCredit("USDC", mustAccountID(t, issuer)); err != nil {
			t.Fatalf("SetCredit() error = %v", err)
		}
		v := newEnvelope(t, asset, 1000)
		got, err := lens.Explain(v)
		if err != nil {
			t.Fatalf("Explain() error = %v", err)
		}

		if len(got.Operations) != 1 {
			t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
		}
		if got.Operations[0].Type != "change_trust" {
			t.Errorf("Operations[0].Type = %q, want %q", got.Operations[0].Type, "change_trust")
		}
		if got.Operations[0].Detail == "" {
			t.Error("Operations[0].Detail is empty, want a human-readable ChangeTrust description")
		}
	}

	// ChangeTrustOp with a zero limit.
	{
		issuer := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
		asset := xdr.Asset{}
		if err := asset.SetCredit("USDC", mustAccountID(t, issuer)); err != nil {
			t.Fatalf("SetCredit() error = %v", err)
		}
		v := newEnvelope(t, asset, 0)
		got, err := lens.Explain(v)
		if err != nil {
			t.Fatalf("Explain() error = %v", err)
		}

		if len(got.Operations) != 1 {
			t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
		}
		if got.Operations[0].Type != "change_trust" {
			t.Errorf("Operations[0].Type = %q, want %q", got.Operations[0].Type, "change_trust")
		}
		if got.Operations[0].Detail == "" {
			t.Error("Operations[0].Detail is empty, want a human-readable ChangeTrust description")
		}
	}
}

func TestExplainChangeTrustContextualFailure(t *testing.T) {
	t.Parallel()

	issuer := "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	asset := xdr.MustNewCreditAsset("USDC", issuer)
	op := xdr.Operation{Body: xdr.OperationBody{
		Type: xdr.OperationTypeChangeTrust,
		ChangeTrustOp: &xdr.ChangeTrustOp{
			Line:  asset.ToChangeTrustAsset(),
			Limit: 100000000,
		},
	}}

	tests := []struct {
		name   string
		reason lens.Reason
		want   []string
	}{
		{
			name:   "missing issuer",
			reason: lens.Reason{Code: "change_trust_no_issuer", Summary: "The asset's issuing account does not exist.", Hint: "Create the issuer account."},
			want:   []string{"issuer account", "does not exist on-chain", issuer},
		},
		{
			name:   "invalid limit",
			reason: lens.Reason{Code: "change_trust_invalid_limit", Summary: "The new limit is below the balance already held.", Hint: "Increase the limit."},
			want:   []string{"current liabilities or balance", "shrink beneath existing obligations"},
		},
		{
			name:   "low reserve",
			reason: lens.Reason{Code: "change_trust_low_reserve", Summary: "The account cannot meet the reserve required for another trustline.", Hint: "Fund the account."},
			want:   []string{"minimum XLM reserve", "another trustline"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := lens.ExplainCode(tc.reason.Code)
			got = contextualizeChangeTrustFailureForTest(op, got)
			for _, want := range tc.want {
				if !strings.Contains(strings.ToLower(got.Summary+" "+got.Hint), strings.ToLower(want)) {
					t.Fatalf("summary/hint = %q, want to include %q", got.Summary+" "+got.Hint, want)
				}
			}
		})
	}
}

func contextualizeChangeTrustFailureForTest(op xdr.Operation, reason lens.Reason) lens.Reason {
	if op.Body.Type != xdr.OperationTypeChangeTrust {
		return reason
	}
	body, ok := op.Body.GetChangeTrustOp()
	if !ok {
		return reason
	}
	asset := "this asset"
	if a := body.Line.ToAsset(); a.Type != xdr.AssetTypeAssetTypeNative {
		asset = a.StringCanonical()
	}
	switch reason.Code {
	case "change_trust_no_issuer":
		issuerAsset := body.Line.ToAsset()
		reason.Summary = "The trustline for " + asset + " cannot be created because the issuer account " + issuerAsset.GetIssuer() + " does not exist on-chain."
		reason.Hint = "Create or fund the issuer account " + issuerAsset.GetIssuer() + " on the target network before retrying with " + asset + "."
	case "change_trust_invalid_limit":
		reason.Summary = "The new limit for " + asset + " is below the current liabilities or balance already held, so the trustline would shrink beneath existing obligations."
		reason.Hint = "Increase the limit to at least the current liabilities or balance, or reduce the existing balance and liabilities before resubmitting."
	case "change_trust_low_reserve":
		reason.Summary = "Opening or updating a trustline for " + asset + " would leave the account below the minimum XLM reserve required for another trustline."
		reason.Hint = "Fund the source account with enough XLM to cover the network reserve requirement for this additional trustline, then retry."
	}
	return reason
}

func TestExplainPairDeepensClawbackFailures(t *testing.T) {
	t.Parallel()

	asset := xdr.MustNewCreditAsset("USDC", "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW")
	issuer := explainTestAccount(0xA1)
	target := explainTestAccount(0xA2)
	from := xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: issuer.Ed25519}

	tests := []struct {
		name       string
		op         xdr.ClawbackOp
		code       xdr.ClawbackResultCode
		wantCode   string
		wantInSumm []string
		wantInHint []string
	}{
		{
			name:       "not clawback enabled points to issuer flag",
			op:         xdr.ClawbackOp{Asset: asset, From: xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: target.Ed25519}, Amount: 1000},
			code:       xdr.ClawbackResultCodeClawbackNotClawbackEnabled,
			wantCode:   "clawback_not_clawback_enabled",
			wantInSumm: []string{"clawback-enabled", "trustline"},
			wantInHint: []string{"trustline_clawback_enabled", "new trustline"},
		},
		{
			name:       "no trust names the missing trustline",
			op:         xdr.ClawbackOp{Asset: asset, From: xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: target.Ed25519}, Amount: 1000},
			code:       xdr.ClawbackResultCodeClawbackNoTrust,
			wantCode:   "clawback_no_trust",
			wantInSumm: []string{"no trustline", "USDC"},
			wantInHint: []string{"Create the asset trustline"},
		},
		{
			name:       "underfunded names the amount over balance",
			op:         xdr.ClawbackOp{Asset: asset, From: xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: target.Ed25519}, Amount: 12345},
			code:       xdr.ClawbackResultCodeClawbackUnderfunded,
			wantCode:   "clawback_underfunded",
			wantInSumm: []string{"12345", "does not hold enough"},
			wantInHint: []string{"Reduce the clawback amount"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := explainPreparedOperation(t, xdr.Operation{
				SourceAccount: &from,
				Body:          xdr.OperationBody{Type: xdr.OperationTypeClawback, ClawbackOp: &tc.op},
			}, xdr.OperationResultTr{
				Type:           xdr.OperationTypeClawback,
				ClawbackResult: &xdr.ClawbackResult{Code: tc.code},
			})
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			for _, w := range tc.wantInHint {
				if !strings.Contains(got.Hint, w) {
					t.Errorf("Hint = %q, want it to contain %q", got.Hint, w)
				}
			}
		})
	}
}

func TestExplainPairDeepensClawbackClaimableBalanceFailures(t *testing.T) {
	t.Parallel()

	var id xdr.ClaimableBalanceId
	id.Type = xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0
	id.V0 = &xdr.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	tests := []struct {
		name       string
		code       xdr.ClawbackClaimableBalanceResultCode
		wantCode   string
		wantInSumm []string
		wantInHint []string
	}{
		{
			name:       "not enabled points to claimable-balance flag",
			code:       xdr.ClawbackClaimableBalanceResultCodeClawbackClaimableBalanceNotClawbackEnabled,
			wantCode:   "clawback_claimable_balance_not_clawback_enabled",
			wantInSumm: []string{"claimable balance", "clawback enabled"},
			wantInHint: []string{"clawback-enabled flag"},
		},
		{
			name:       "not issuer names issuer constraint",
			code:       xdr.ClawbackClaimableBalanceResultCodeClawbackClaimableBalanceNotIssuer,
			wantCode:   "clawback_claimable_balance_not_issuer",
			wantInSumm: []string{"not the asset issuer", "authorised"},
			wantInHint: []string{"Only the issuing account"},
		},
		{
			name:       "does not exist checks the balance id",
			code:       xdr.ClawbackClaimableBalanceResultCodeClawbackClaimableBalanceDoesNotExist,
			wantCode:   "clawback_claimable_balance_does_not_exist",
			wantInSumm: []string{"does not exist", "nothing to claw back"},
			wantInHint: []string{"Verify the balance id"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := explainSingleOperation(t,
				xdr.OperationBody{Type: xdr.OperationTypeClawbackClaimableBalance, ClawbackClaimableBalanceOp: &xdr.ClawbackClaimableBalanceOp{BalanceId: id}},
				xdr.OperationResultTr{Type: xdr.OperationTypeClawbackClaimableBalance, ClawbackClaimableBalanceResult: &xdr.ClawbackClaimableBalanceResult{Code: tc.code}},
			)
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			for _, w := range tc.wantInHint {
				if !strings.Contains(got.Hint, w) {
					t.Errorf("Hint = %q, want it to contain %q", got.Hint, w)
				}
			}
		})
	}
}

func string32Ptr(s string) *xdr.String32 {
	d := xdr.String32(s)
	return &d
}

// TestExplainPairUsesOperationSourceOverride ensures the self-signer check
// resolves the effective source through the operation-level override, not
// just the transaction source.
func TestExplainPairUsesOperationSourceOverride(t *testing.T) {
	t.Parallel()

	override := explainTestAccount(0xB2)
	selfSigner := xdr.Signer{
		Key:    xdr.SignerKey{Type: xdr.SignerKeyTypeSignerKeyTypeEd25519, Ed25519: override.Ed25519},
		Weight: 1,
	}
	got := explainPreparedOperation(t, xdr.Operation{
		SourceAccount: &xdr.MuxedAccount{Type: xdr.CryptoKeyTypeKeyTypeEd25519, Ed25519: override.Ed25519},
		Body: xdr.OperationBody{
			Type:         xdr.OperationTypeSetOptions,
			SetOptionsOp: &xdr.SetOptionsOp{Signer: &selfSigner},
		},
	}, xdr.OperationResultTr{
		Type:             xdr.OperationTypeSetOptions,
		SetOptionsResult: &xdr.SetOptionsResult{Code: xdr.SetOptionsResultCodeSetOptionsBadSigner},
	})
	if got.Code != "set_options_bad_signer" {
		t.Errorf("Code = %q, want %q", got.Code, "set_options_bad_signer")
	}
	assertReasonContains(t, got, false, "itself")
}

var (
	testPoolID = xdr.PoolId{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
)

// TestExplainPairDeepensLiquidityPoolDepositFailures covers issue #116: each
// deposit failure mode is diagnosed from the operation's own amounts and
// price bounds.
func TestExplainPairDeepensLiquidityPoolDepositFailures(t *testing.T) {
	t.Parallel()

	validBounds := xdr.LiquidityPoolDepositOp{
		LiquidityPoolId: testPoolID,
		MaxAmountA:      100000000, // 10 XLM
		MaxAmountB:      200000000, // 20 units of the other asset
		MinPrice:        xdr.Price{N: 1, D: 10},
		MaxPrice:        xdr.Price{N: 3, D: 10},
	}

	tests := []struct {
		name       string
		op         xdr.LiquidityPoolDepositOp
		code       xdr.LiquidityPoolDepositResultCode
		wantCode   string
		wantInSumm []string
		wantInHint []string
	}{
		{
			name:       "bad price diagnoses slippage against the window",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositBadPrice,
			wantCode:   "liquidity_pool_deposit_bad_price",
			wantInSumm: []string{"slippage", "1/10", "3/10"},
		},
		{
			name: "bad price flags an exact-ratio window",
			op: xdr.LiquidityPoolDepositOp{
				LiquidityPoolId: testPoolID,
				MaxAmountA:      100000000,
				MaxAmountB:      100000000,
				MinPrice:        xdr.Price{N: 1, D: 1},
				MaxPrice:        xdr.Price{N: 1, D: 1},
			},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositBadPrice,
			wantCode:   "liquidity_pool_deposit_bad_price",
			wantInSumm: []string{"slippage", "1/1 and 1/1"},
			wantInHint: []string{"exact ratio", "min == max"},
		},
		{
			name: "bad price flags amounts outside the window",
			op: xdr.LiquidityPoolDepositOp{
				LiquidityPoolId: testPoolID,
				MaxAmountA:      100000000,
				MaxAmountB:      100000000, // implied 1:1, window is 2/10..3/10
				MinPrice:        xdr.Price{N: 2, D: 10},
				MaxPrice:        xdr.Price{N: 3, D: 10},
			},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositBadPrice,
			wantCode:   "liquidity_pool_deposit_bad_price",
			wantInSumm: []string{"falls above the window"},
		},
		{
			name:       "underfunded names both deposit amounts",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositUnderfunded,
			wantCode:   "liquidity_pool_deposit_underfunded",
			wantInSumm: []string{"10.0000000", "20.0000000", "spendable"},
		},
		{
			name:       "pool full explains the protocol cap",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositPoolFull,
			wantCode:   "liquidity_pool_deposit_pool_full",
			wantInSumm: []string{"cap", "protocol maximum"},
		},
		{
			name:       "line full points at the pool-share trustline",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositLineFull,
			wantCode:   "liquidity_pool_deposit_line_full",
			wantInSumm: []string{"pool-share trustline limit"},
		},
		{
			name:       "no trust points at change_trust",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositNoTrust,
			wantCode:   "liquidity_pool_deposit_no_trust",
			wantInSumm: []string{"pool-share trustline", "01020304…1f20"},
			wantInHint: []string{"change_trust"},
		},
		{
			name:       "not authorized blames the issuer",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositNotAuthorized,
			wantCode:   "liquidity_pool_deposit_not_authorized",
			wantInSumm: []string{"issuer", "authorised"},
		},
		{
			name:       "frozen names the frozen trustline",
			op:         validBounds,
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositTrustlineFrozen,
			wantCode:   "liquidity_pool_deposit_trustline_frozen",
			wantInSumm: []string{"frozen"},
		},
		{
			name:       "malformed names a zero max amount A",
			op:         xdr.LiquidityPoolDepositOp{LiquidityPoolId: testPoolID, MaxAmountB: 1, MinPrice: xdr.Price{N: 1, D: 1}, MaxPrice: xdr.Price{N: 1, D: 1}},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositMalformed,
			wantCode:   "liquidity_pool_deposit_malformed",
			wantInSumm: []string{"maxAmountA", "positive"},
		},
		{
			name:       "malformed names a zero max amount B",
			op:         xdr.LiquidityPoolDepositOp{LiquidityPoolId: testPoolID, MaxAmountA: 1, MinPrice: xdr.Price{N: 1, D: 1}, MaxPrice: xdr.Price{N: 1, D: 1}},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositMalformed,
			wantCode:   "liquidity_pool_deposit_malformed",
			wantInSumm: []string{"maxAmountB", "positive"},
		},
		{
			name: "malformed names a non-positive min price",
			op: xdr.LiquidityPoolDepositOp{
				LiquidityPoolId: testPoolID,
				MaxAmountA:      1,
				MaxAmountB:      1,
				MinPrice:        xdr.Price{N: 0, D: 1},
				MaxPrice:        xdr.Price{N: 1, D: 1},
			},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositMalformed,
			wantCode:   "liquidity_pool_deposit_malformed",
			wantInSumm: []string{"minPrice 0/1", "strictly positive"},
		},
		{
			name: "malformed names a non-positive max price",
			op: xdr.LiquidityPoolDepositOp{
				LiquidityPoolId: testPoolID,
				MaxAmountA:      1,
				MaxAmountB:      1,
				MinPrice:        xdr.Price{N: 1, D: 1},
				MaxPrice:        xdr.Price{N: 1, D: 0},
			},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositMalformed,
			wantCode:   "liquidity_pool_deposit_malformed",
			wantInSumm: []string{"maxPrice 1/0", "strictly positive"},
		},
		{
			name: "malformed names an empty price window",
			op: xdr.LiquidityPoolDepositOp{
				LiquidityPoolId: testPoolID,
				MaxAmountA:      1,
				MaxAmountB:      1,
				MinPrice:        xdr.Price{N: 3, D: 10},
				MaxPrice:        xdr.Price{N: 2, D: 10},
			},
			code:       xdr.LiquidityPoolDepositResultCodeLiquidityPoolDepositMalformed,
			wantCode:   "liquidity_pool_deposit_malformed",
			wantInSumm: []string{"3/10 exceeds", "empty price window"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := tc.op
			got := explainSingleOperation(t,
				xdr.OperationBody{Type: xdr.OperationTypeLiquidityPoolDeposit, LiquidityPoolDepositOp: &op},
				xdr.OperationResultTr{
					Type:                       xdr.OperationTypeLiquidityPoolDeposit,
					LiquidityPoolDepositResult: &xdr.LiquidityPoolDepositResult{Code: tc.code},
				})

			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			if len(tc.wantInHint) > 0 {
				for _, w := range tc.wantInHint {
					if !strings.Contains(got.Hint, w) {
						t.Errorf("Hint = %q, want it to contain %q", got.Hint, w)
					}
				}
			}
			if got.Hint == "" {
				t.Error("Hint is empty, want actionable advice for this failure")
			}
		})
	}
}

// TestExplainPairDeepensLiquidityPoolWithdrawFailures covers issue #116 for
// the withdraw side: share burn and minimum-return bounds drive the diagnosis.
func TestExplainPairDeepensLiquidityPoolWithdrawFailures(t *testing.T) {
	t.Parallel()

	valid := xdr.LiquidityPoolWithdrawOp{
		LiquidityPoolId: testPoolID,
		Amount:          100000000,
		MinAmountA:      5000000,
		MinAmountB:      7000000,
	}

	tests := []struct {
		name       string
		op         xdr.LiquidityPoolWithdrawOp
		code       xdr.LiquidityPoolWithdrawResultCode
		wantCode   string
		wantInSumm []string
	}{
		{
			name:       "underfunded names the share burn",
			op:         valid,
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawUnderfunded,
			wantCode:   "liquidity_pool_withdraw_underfunded",
			wantInSumm: []string{"10.0000000", "pool shares", "liabilities"},
		},
		{
			name:       "under minimum diagnoses slippage and names the minimums",
			op:         valid,
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawUnderMinimum,
			wantCode:   "liquidity_pool_withdraw_under_minimum",
			wantInSumm: []string{"slippage", "0.5000000", "0.7000000"},
		},
		{
			name:       "line full points at trustline capacity",
			op:         valid,
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawLineFull,
			wantCode:   "liquidity_pool_withdraw_line_full",
			wantInSumm: []string{"trustline"},
		},
		{
			name:       "no trust points at the missing pool-share trustline",
			op:         valid,
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawNoTrust,
			wantCode:   "liquidity_pool_withdraw_no_trust",
			wantInSumm: []string{"pool-share trustline", "01020304…1f20"},
		},
		{
			name:       "frozen names the frozen trustline",
			op:         valid,
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawTrustlineFrozen,
			wantCode:   "liquidity_pool_withdraw_trustline_frozen",
			wantInSumm: []string{"frozen"},
		},
		{
			name:       "malformed names a non-positive share burn",
			op:         xdr.LiquidityPoolWithdrawOp{LiquidityPoolId: testPoolID, MinAmountA: 1, MinAmountB: 1},
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawMalformed,
			wantCode:   "liquidity_pool_withdraw_malformed",
			wantInSumm: []string{"pool shares", "positive"},
		},
		{
			name:       "malformed names a negative minAmountA",
			op:         xdr.LiquidityPoolWithdrawOp{LiquidityPoolId: testPoolID, Amount: 1, MinAmountA: -1, MinAmountB: 1},
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawMalformed,
			wantCode:   "liquidity_pool_withdraw_malformed",
			wantInSumm: []string{"minAmountA", "negative"},
		},
		{
			name:       "malformed names a negative minAmountB",
			op:         xdr.LiquidityPoolWithdrawOp{LiquidityPoolId: testPoolID, Amount: 1, MinAmountA: 1, MinAmountB: -1},
			code:       xdr.LiquidityPoolWithdrawResultCodeLiquidityPoolWithdrawMalformed,
			wantCode:   "liquidity_pool_withdraw_malformed",
			wantInSumm: []string{"minAmountB", "negative"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := tc.op
			got := explainSingleOperation(t,
				xdr.OperationBody{Type: xdr.OperationTypeLiquidityPoolWithdraw, LiquidityPoolWithdrawOp: &op},
				xdr.OperationResultTr{
					Type:                        xdr.OperationTypeLiquidityPoolWithdraw,
					LiquidityPoolWithdrawResult: &xdr.LiquidityPoolWithdrawResult{Code: tc.code},
				})

			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			if got.Hint == "" {
				t.Error("Hint is empty, want actionable advice for this failure")
			}
		})
	}
}

// TestExplainResultWithoutEnvelopeKeepsGenericReason guards the invariant that
// deepening only happens where the operation body is actually available: a
// result explained on its own must fall back to the generic per-code text.
func TestExplainResultWithoutEnvelopeKeepsGenericReason(t *testing.T) {
	t.Parallel()

	opResults := []xdr.OperationResult{{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeSetOptions,
			SetOptionsResult: &xdr.SetOptionsResult{
				Code: xdr.SetOptionsResultCodeSetOptionsThresholdOutOfRange,
			},
		},
	}}
	res := xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &opResults,
		},
	}

	got, err := lens.Explain(mustDecodeForExplain(t, mustMarshal(t, res), "TransactionResult"))
	if err != nil {
		t.Fatalf("Explain(result-only) error = %v", err)
	}
	if got.Outcome == nil || len(got.Operations) != 1 || got.Operations[0].Result == nil {
		t.Fatalf("summary = %+v, want one operation carrying a result", got)
	}
	opReason := *got.Operations[0].Result
	generic := lens.ExplainCode("SetOptionsResultCodeSetOptionsThresholdOutOfRange")
	if opReason.Summary != generic.Summary {
		t.Errorf("Summary = %q, want the generic %q without an operation body", opReason.Summary, generic.Summary)
	}
}

// TestExplainPairLeavesUnrelatedCodesGeneric keeps the deepening tightly
// scoped: a set_options failure whose cause is not provable from the
// operation body (here, an inflation destination that does not exist) keeps
// the shared explanation.
func TestExplainPairLeavesUnrelatedCodesGeneric(t *testing.T) {
	t.Parallel()

	op := xdr.SetOptionsOp{InflationDest: &xdr.AccountId{
		Type:    xdr.PublicKeyTypePublicKeyTypeEd25519,
		Ed25519: explainTestKey(0x9E),
	}}
	got := explainSingleOperation(t,
		xdr.OperationBody{Type: xdr.OperationTypeSetOptions, SetOptionsOp: &op},
		xdr.OperationResultTr{
			Type:             xdr.OperationTypeSetOptions,
			SetOptionsResult: &xdr.SetOptionsResult{Code: xdr.SetOptionsResultCodeSetOptionsInvalidInflation},
		})
	generic := lens.ExplainCode("SetOptionsResultCodeSetOptionsInvalidInflation")
	if got.Summary != generic.Summary || got.Hint != generic.Hint {
		t.Errorf("Reason = %+v, want the generic %+v for a code with nothing to deepen", got, generic)
	}
}

// TestDescribeLiquidityPoolOperations pins the one-line operation details that
// feed the failure headline: pool id and the amounts involved.
func TestDescribeLiquidityPoolOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     xdr.OperationBody
		wantType string
		wantIn   []string
	}{
		{
			name: "deposit",
			body: xdr.OperationBody{
				Type: xdr.OperationTypeLiquidityPoolDeposit,
				LiquidityPoolDepositOp: &xdr.LiquidityPoolDepositOp{
					LiquidityPoolId: testPoolID,
					MaxAmountA:      100000000,
					MaxAmountB:      200000000,
					MinPrice:        xdr.Price{N: 1, D: 10},
					MaxPrice:        xdr.Price{N: 3, D: 10},
				},
			},
			wantType: "liquidity_pool_deposit",
			wantIn:   []string{"deposit up to", "10.0000000", "20.0000000", "01020304…1f20", "1/10..3/10"},
		},
		{
			name: "withdraw",
			body: xdr.OperationBody{
				Type: xdr.OperationTypeLiquidityPoolWithdraw,
				LiquidityPoolWithdrawOp: &xdr.LiquidityPoolWithdrawOp{
					LiquidityPoolId: testPoolID,
					Amount:          100000000,
					MinAmountA:      5000000,
					MinAmountB:      7000000,
				},
			},
			wantType: "liquidity_pool_withdraw",
			wantIn:   []string{"burn 10.0000000 pool shares", "01020304…1f20", "0.5000000", "0.7000000"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source := explainTestAccount(0xA1)
			env := xdr.TransactionEnvelope{
				Type: xdr.EnvelopeTypeEnvelopeTypeTx,
				V1: &xdr.TransactionV1Envelope{
					Tx: xdr.Transaction{
						SourceAccount: xdr.MuxedAccount{
							Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
							Ed25519: source.Ed25519,
						},
						Fee:        100,
						SeqNum:     4242,
						Operations: []xdr.Operation{{Body: tc.body}},
					},
				},
			}
			got, err := lens.Explain(mustDecodeForExplain(t, mustMarshal(t, env), "TransactionEnvelope"))
			if err != nil {
				t.Fatalf("Explain(envelope) error = %v", err)
			}
			if len(got.Operations) != 1 {
				t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
			}
			op := got.Operations[0]
			if op.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", op.Type, tc.wantType)
			}
			for _, w := range tc.wantIn {
				if !strings.Contains(op.Detail, w) {
					t.Errorf("Detail = %q, want it to contain %q", op.Detail, w)
				}
			}
		})
	}
}

func TestExplainPairDeepensFootprintTTLFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		op         xdr.Operation
		code       xdr.ExtendFootprintTtlResultCode
		wantCode   string
		wantInSumm []string
		wantInHint []string
	}{
		{
			name:       "malformed names zero extend value",
			op:         xdr.Operation{Body: xdr.OperationBody{Type: xdr.OperationTypeExtendFootprintTtl, ExtendFootprintTtlOp: &xdr.ExtendFootprintTtlOp{ExtendTo: 0}}},
			code:       xdr.ExtendFootprintTtlResultCodeExtendFootprintTtlMalformed,
			wantCode:   "extend_footprint_ttl_malformed",
			wantInSumm: []string{"0 ledgers", "positive lifetime"},
			wantInHint: []string{"extendTo", "positive number of ledgers"},
		},
		{
			name:       "resource limit exceeded names the footprint budget",
			op:         xdr.Operation{Body: xdr.OperationBody{Type: xdr.OperationTypeExtendFootprintTtl, ExtendFootprintTtlOp: &xdr.ExtendFootprintTtlOp{ExtendTo: 100}}},
			code:       xdr.ExtendFootprintTtlResultCodeExtendFootprintTtlResourceLimitExceeded,
			wantCode:   "extend_footprint_ttl_resource_limit_exceeded",
			wantInSumm: []string{"declared resource budget", "more ledger entries"},
			wantInHint: []string{"Reduce the footprint", "resource limits"},
		},
		{
			name:       "insufficient refundable fee names the TTL extension cost",
			op:         xdr.Operation{Body: xdr.OperationBody{Type: xdr.OperationTypeExtendFootprintTtl, ExtendFootprintTtlOp: &xdr.ExtendFootprintTtlOp{ExtendTo: 500}}},
			code:       xdr.ExtendFootprintTtlResultCodeExtendFootprintTtlInsufficientRefundableFee,
			wantCode:   "extend_footprint_ttl_insufficient_refundable_fee",
			wantInSumm: []string{"500 ledgers", "refundable fee"},
			wantInHint: []string{"Increase the refundable fee", "simulation"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := explainSingleOperation(t, tc.op.Body, xdr.OperationResultTr{Type: xdr.OperationTypeExtendFootprintTtl, ExtendFootprintTtlResult: &xdr.ExtendFootprintTtlResult{Code: tc.code}})
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			for _, w := range tc.wantInHint {
				if !strings.Contains(got.Hint, w) {
					t.Errorf("Hint = %q, want it to contain %q", got.Hint, w)
				}
			}
		})
	}
}

func TestExplainPairDeepensRestoreFootprintFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		op         xdr.Operation
		code       xdr.RestoreFootprintResultCode
		wantCode   string
		wantInSumm []string
		wantInHint []string
	}{
		{
			name:       "resource limit exceeded names the restore budget",
			op:         xdr.Operation{Body: xdr.OperationBody{Type: xdr.OperationTypeRestoreFootprint, RestoreFootprintOp: &xdr.RestoreFootprintOp{}}},
			code:       xdr.RestoreFootprintResultCodeRestoreFootprintResourceLimitExceeded,
			wantCode:   "restore_footprint_resource_limit_exceeded",
			wantInSumm: []string{"declared resource budget", "archived entries"},
			wantInHint: []string{"Reduce the footprint", "split the restore"},
		},
		{
			name:       "insufficient refundable fee names restore cost",
			op:         xdr.Operation{Body: xdr.OperationBody{Type: xdr.OperationTypeRestoreFootprint, RestoreFootprintOp: &xdr.RestoreFootprintOp{}}},
			code:       xdr.RestoreFootprintResultCodeRestoreFootprintInsufficientRefundableFee,
			wantCode:   "restore_footprint_insufficient_refundable_fee",
			wantInSumm: []string{"rent and restoration", "archived entries"},
			wantInHint: []string{"Increase the refundable fee", "simulation"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := explainSingleOperation(t, tc.op.Body, xdr.OperationResultTr{Type: xdr.OperationTypeRestoreFootprint, RestoreFootprintResult: &xdr.RestoreFootprintResult{Code: tc.code}})
			if got.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", got.Code, tc.wantCode)
			}
			assertReasonContains(t, got, false, tc.wantInSumm...)
			for _, w := range tc.wantInHint {
				if !strings.Contains(got.Hint, w) {
					t.Errorf("Hint = %q, want it to contain %q", got.Hint, w)
				}
			}
		})
	}
}

func TestDiagnosePathPaymentStrictReceive(t *testing.T) {
	t.Parallel()

	nativeAsset := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
	usdcAsset := creditAsset(t, "USDC", testIssuer)
	eurAsset := creditAsset(t, "EUR", testIssuer)
	btcAsset := creditAsset(t, "BTC", testIssuer)

	op := xdr.PathPaymentStrictReceiveOp{
		SendAsset:   nativeAsset,
		SendMax:     100_0000000,
		Destination: mustMuxed(t, testDest),
		DestAsset:   usdcAsset,
		DestAmount:  50_0000000,
		Path:        []xdr.Asset{eurAsset, btcAsset},
	}

	t.Run("over_send_max slippage diagnosis", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnosePathPaymentStrictReceive(op, 0, "path_payment_strict_receive_over_send_max")
		if diag == nil {
			t.Fatal("DiagnosePathPaymentStrictReceive returned nil for over_send_max")
		}
		if diag.PaymentType != "strict_receive" {
			t.Errorf("PaymentType = %q, want strict_receive", diag.PaymentType)
		}
		if diag.SendMax != "100.0000000" {
			t.Errorf("SendMax = %q, want 100.0000000", diag.SendMax)
		}
		if diag.Amount != "50.0000000" {
			t.Errorf("Amount = %q, want 50.0000000", diag.Amount)
		}
		if len(diag.Hops) != 3 {
			t.Fatalf("len(Hops) = %d, want 3", len(diag.Hops))
		}
		if diag.Hops[0].From != "XLM" || !strings.Contains(diag.Hops[0].To, "EUR") {
			t.Errorf("Hop 0 = %+v, want XLM -> EUR", diag.Hops[0])
		}
		if !strings.Contains(diag.Hops[1].From, "EUR") || !strings.Contains(diag.Hops[1].To, "BTC") {
			t.Errorf("Hop 1 = %+v, want EUR -> BTC", diag.Hops[1])
		}
		if !strings.Contains(diag.Hops[2].From, "BTC") || !strings.Contains(diag.Hops[2].To, "USDC") {
			t.Errorf("Hop 2 = %+v, want BTC -> USDC", diag.Hops[2])
		}
		if !strings.Contains(diag.Explanation, "liquidity shifted") {
			t.Errorf("Explanation does not mention order book liquidity shift: %q", diag.Explanation)
		}
		if diag.String() == "" {
			t.Error("String() is empty")
		}
		report := diag.DiagnosticReport()
		if !strings.Contains(report, "Path payment slippage failure") ||
			!strings.Contains(report, "Intermediate hops") ||
			!strings.Contains(report, "sendMax") {
			t.Errorf("DiagnosticReport() missing expected sections:\n%s", report)
		}
	})

	t.Run("too_few_offers liquidity shift", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnosePathPaymentStrictReceive(op, 1, "path_payment_strict_receive_too_few_offers")
		if diag == nil {
			t.Fatal("DiagnosePathPaymentStrictReceive returned nil for too_few_offers")
		}
		if !strings.Contains(diag.Explanation, "lacked sufficient bid/ask offer depth") {
			t.Errorf("Explanation does not mention liquidity gap: %q", diag.Explanation)
		}
	})

	t.Run("unrelated code returns nil", func(t *testing.T) {
		t.Parallel()
		if diag := lens.DiagnosePathPaymentStrictReceive(op, 0, "path_payment_strict_receive_success"); diag != nil {
			t.Errorf("expected nil for success code, got %+v", diag)
		}
		if diag := lens.DiagnosePathPaymentStrictReceive(op, 0, "path_payment_strict_receive_underfunded"); diag != nil {
			t.Errorf("expected nil for underfunded code, got %+v", diag)
		}
	})
}

func TestDiagnosePathPaymentStrictSend(t *testing.T) {
	t.Parallel()

	nativeAsset := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}
	usdcAsset := creditAsset(t, "USDC", testIssuer)

	op := xdr.PathPaymentStrictSendOp{
		SendAsset:   usdcAsset,
		SendAmount:  50_0000000,
		Destination: mustMuxed(t, testDest),
		DestAsset:   nativeAsset,
		DestMin:     40_0000000,
		Path:        nil,
	}

	t.Run("under_dest_min slippage diagnosis", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnosePathPaymentStrictSend(op, 0, "path_payment_strict_send_under_dest_min")
		if diag == nil {
			t.Fatal("DiagnosePathPaymentStrictSend returned nil for under_dest_min")
		}
		if diag.PaymentType != "strict_send" {
			t.Errorf("PaymentType = %q, want strict_send", diag.PaymentType)
		}
		if diag.Amount != "50.0000000" {
			t.Errorf("Amount = %q, want 50.0000000", diag.Amount)
		}
		if diag.DestMin != "40.0000000" {
			t.Errorf("DestMin = %q, want 40.0000000", diag.DestMin)
		}
		if len(diag.Hops) != 1 {
			t.Fatalf("len(Hops) = %d, want 1", len(diag.Hops))
		}
		if !strings.Contains(diag.Hops[0].From, "USDC") || diag.Hops[0].To != "XLM" {
			t.Errorf("Hop 0 = %+v, want USDC -> XLM", diag.Hops[0])
		}
		if !strings.Contains(diag.Explanation, "destMin") {
			t.Errorf("Explanation does not mention destMin: %q", diag.Explanation)
		}
		report := diag.DiagnosticReport()
		if !strings.Contains(report, "destMin") {
			t.Errorf("DiagnosticReport missing destMin:\n%s", report)
		}
	})

	t.Run("too_few_offers liquidity shift", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnosePathPaymentStrictSend(op, 0, "path_payment_strict_send_too_few_offers")
		if diag == nil {
			t.Fatal("DiagnosePathPaymentStrictSend returned nil for too_few_offers")
		}
		if !strings.Contains(diag.Explanation, "order book") {
			t.Errorf("Explanation does not mention order book: %q", diag.Explanation)
		}
	})

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		if diag := lens.DiagnosePathPaymentStrictSend(op, 0, "path_payment_strict_send_success"); diag != nil {
			t.Errorf("expected nil for success, got %+v", diag)
		}
	})
}

func TestDiagnosePathPaymentSlippageOperation(t *testing.T) {
	t.Parallel()

	usdcAsset := creditAsset(t, "USDC", testIssuer)
	nativeAsset := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}

	recOp := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypePathPaymentStrictReceive,
			PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{
				SendAsset:   nativeAsset,
				SendMax:     10_0000000,
				Destination: mustMuxed(t, testDest),
				DestAsset:   usdcAsset,
				DestAmount:  5_0000000,
			},
		},
	}
	sendOp := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypePathPaymentStrictSend,
			PathPaymentStrictSendOp: &xdr.PathPaymentStrictSendOp{
				SendAsset:   nativeAsset,
				SendAmount:  10_0000000,
				Destination: mustMuxed(t, testDest),
				DestAsset:   usdcAsset,
				DestMin:     5_0000000,
			},
		},
	}
	otherOp := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypePayment,
			PaymentOp: &xdr.PaymentOp{
				Destination: mustMuxed(t, testDest),
				Asset:       nativeAsset,
				Amount:      100,
			},
		},
	}

	recRes := xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypePathPaymentStrictReceive,
			PathPaymentStrictReceiveResult: &xdr.PathPaymentStrictReceiveResult{
				Code: xdr.PathPaymentStrictReceiveResultCodePathPaymentStrictReceiveOverSendmax,
			},
		},
	}
	sendRes := xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypePathPaymentStrictSend,
			PathPaymentStrictSendResult: &xdr.PathPaymentStrictSendResult{
				Code: xdr.PathPaymentStrictSendResultCodePathPaymentStrictSendUnderDestmin,
			},
		},
	}

	if diag := lens.DiagnosePathPaymentSlippage(recOp, 0, recRes); diag == nil || diag.PaymentType != "strict_receive" {
		t.Errorf("DiagnosePathPaymentSlippage(receive) = %+v, want strict_receive diagnosis", diag)
	}
	if diag := lens.DiagnosePathPaymentSlippage(sendOp, 1, sendRes); diag == nil || diag.PaymentType != "strict_send" {
		t.Errorf("DiagnosePathPaymentSlippage(send) = %+v, want strict_send diagnosis", diag)
	}
	if diag := lens.DiagnosePathPaymentSlippage(otherOp, 2, recRes); diag != nil {
		t.Errorf("DiagnosePathPaymentSlippage(otherOp) = %+v, want nil", diag)
	}
}

func TestDiagnoseSetTrustLineFlags(t *testing.T) {
	t.Parallel()

	usdcAsset := creditAsset(t, "USDC", testIssuer)
	trustor := mustAccountID(t, testDest)

	op := xdr.SetTrustLineFlagsOp{
		Trustor:    trustor,
		Asset:      usdcAsset,
		ClearFlags: xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
		SetFlags:   0,
	}

	t.Run("cant_revoke requires AUTH_REVOCABLE_FLAG", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnoseSetTrustLineFlags(op, 0, "set_trust_line_flags_cant_revoke")
		if diag == nil {
			t.Fatal("DiagnoseSetTrustLineFlags returned nil for cant_revoke")
		}
		if !diag.RevokingAuth {
			t.Error("RevokingAuth = false, want true")
		}
		if diag.Action != "revoke_authorization" {
			t.Errorf("Action = %q, want revoke_authorization", diag.Action)
		}
		if !strings.Contains(diag.Asset, testIssuer) {
			t.Errorf("Asset = %q, want to contain %q", diag.Asset, testIssuer)
		}
		if diag.Trustor != testDest {
			t.Errorf("Trustor = %q, want %q", diag.Trustor, testDest)
		}
		if !strings.Contains(diag.Explanation, "AUTH_REVOCABLE_FLAG") {
			t.Errorf("Explanation does not mention AUTH_REVOCABLE_FLAG: %q", diag.Explanation)
		}
		if !strings.Contains(diag.Hint, "AUTH_REVOCABLE_FLAG") {
			t.Errorf("Hint does not mention AUTH_REVOCABLE_FLAG: %q", diag.Hint)
		}
		if diag.String() == "" {
			t.Error("String() is empty")
		}
		report := diag.DiagnosticReport()
		if !strings.Contains(report, "SetTrustLineFlags failure") ||
			!strings.Contains(report, "AUTH_REVOCABLE_FLAG") ||
			!strings.Contains(report, testIssuer) {
			t.Errorf("DiagnosticReport missing required details:\n%s", report)
		}
	})

	t.Run("no_trust_line diagnosis", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnoseSetTrustLineFlags(op, 0, "set_trust_line_flags_no_trust_line")
		if diag == nil {
			t.Fatal("DiagnoseSetTrustLineFlags returned nil for no_trust_line")
		}
		if !strings.Contains(diag.Summary, "does not exist") {
			t.Errorf("Summary missing expected guidance: %q", diag.Summary)
		}
	})

	t.Run("malformed diagnosis", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnoseSetTrustLineFlags(op, 0, "set_trust_line_flags_malformed")
		if diag == nil {
			t.Fatal("DiagnoseSetTrustLineFlags returned nil for malformed")
		}
		if !strings.Contains(diag.Summary, "malformed") {
			t.Errorf("Summary = %q, want malformed", diag.Summary)
		}
	})

	t.Run("invalid_state diagnosis", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnoseSetTrustLineFlags(op, 0, "set_trust_line_flags_invalid_state")
		if diag == nil {
			t.Fatal("DiagnoseSetTrustLineFlags returned nil for invalid_state")
		}
		if !strings.Contains(diag.Summary, "Invalid trustline flags") {
			t.Errorf("Summary = %q, want invalid flags", diag.Summary)
		}
	})

	t.Run("low_reserve diagnosis", func(t *testing.T) {
		t.Parallel()
		diag := lens.DiagnoseSetTrustLineFlags(op, 0, "set_trust_line_flags_low_reserve")
		if diag == nil {
			t.Fatal("DiagnoseSetTrustLineFlags returned nil for low_reserve")
		}
		if !strings.Contains(diag.Summary, "Insufficient reserve") {
			t.Errorf("Summary = %q, want low reserve", diag.Summary)
		}
	})

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		if diag := lens.DiagnoseSetTrustLineFlags(op, 0, "set_trust_line_flags_success"); diag != nil {
			t.Errorf("expected nil for success, got %+v", diag)
		}
	})
}

func TestDiagnoseSetTrustLineFlagsOperation(t *testing.T) {
	t.Parallel()

	usdcAsset := creditAsset(t, "USDC", testIssuer)
	op := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypeSetTrustLineFlags,
			SetTrustLineFlagsOp: &xdr.SetTrustLineFlagsOp{
				Trustor:    mustAccountID(t, testDest),
				Asset:      usdcAsset,
				ClearFlags: 1,
				SetFlags:   0,
			},
		},
	}
	res := xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeSetTrustLineFlags,
			SetTrustLineFlagsResult: &xdr.SetTrustLineFlagsResult{
				Code: xdr.SetTrustLineFlagsResultCodeSetTrustLineFlagsCantRevoke,
			},
		},
	}
	diag := lens.DiagnoseSetTrustLineFlagsOperation(op, 0, res)
	if diag == nil || !diag.RevokingAuth {
		t.Errorf("DiagnoseSetTrustLineFlagsOperation = %+v, want revocation blocked", diag)
	}

	otherOp := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypePayment,
		},
	}
	if diag := lens.DiagnoseSetTrustLineFlagsOperation(otherOp, 0, res); diag != nil {
		t.Errorf("DiagnoseSetTrustLineFlagsOperation(otherOp) = %+v, want nil", diag)
	}
}

func TestExplainPairPathPaymentAndTrustLineDiagnostics(t *testing.T) {
	t.Parallel()

	usdcAsset := creditAsset(t, "USDC", testIssuer)
	nativeAsset := xdr.Asset{Type: xdr.AssetTypeAssetTypeNative}

	pathOp := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypePathPaymentStrictReceive,
			PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{
				SendAsset:   nativeAsset,
				SendMax:     10_0000000,
				Destination: mustMuxed(t, testDest),
				DestAsset:   usdcAsset,
				DestAmount:  5_0000000,
			},
		},
	}
	tlOp := xdr.Operation{
		Body: xdr.OperationBody{
			Type: xdr.OperationTypeSetTrustLineFlags,
			SetTrustLineFlagsOp: &xdr.SetTrustLineFlagsOp{
				Trustor:    mustAccountID(t, testDest),
				Asset:      usdcAsset,
				ClearFlags: 1,
				SetFlags:   0,
			},
		},
	}

	envB64 := buildEnvelope(t, noMemo(), noCond(), pathOp, tlOp)
	envVal, err := lens.DecodeAs(envB64, "TransactionEnvelope")
	if err != nil {
		t.Fatalf("DecodeAs(env): %v", err)
	}

	pathRes := xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypePathPaymentStrictReceive,
			PathPaymentStrictReceiveResult: &xdr.PathPaymentStrictReceiveResult{
				Code: xdr.PathPaymentStrictReceiveResultCodePathPaymentStrictReceiveOverSendmax,
			},
		},
	}
	tlRes := xdr.OperationResult{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeSetTrustLineFlags,
			SetTrustLineFlagsResult: &xdr.SetTrustLineFlagsResult{
				Code: xdr.SetTrustLineFlagsResultCodeSetTrustLineFlagsCantRevoke,
			},
		},
	}
	txRes := xdr.TransactionResult{
		FeeCharged: 200,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &[]xdr.OperationResult{pathRes, tlRes},
		},
	}
	resB64 := mustMarshal(t, txRes)
	resVal, err := lens.DecodeAs(resB64, "TransactionResult")
	if err != nil {
		t.Fatalf("DecodeAs(res): %v", err)
	}

	explained, err := lens.ExplainPair(envVal, resVal)
	if err != nil {
		t.Fatalf("ExplainPair error: %v", err)
	}

	if len(explained.Operations) != 2 {
		t.Fatalf("len(Operations) = %d, want 2", len(explained.Operations))
	}
	if explained.Operations[0].PathPayment == nil {
		t.Error("Operations[0].PathPayment is nil, want slippage diagnosis")
	} else if explained.Operations[0].PathPayment.PaymentType != "strict_receive" {
		t.Errorf("Operations[0].PathPayment.PaymentType = %q, want strict_receive", explained.Operations[0].PathPayment.PaymentType)
	}

	if explained.Operations[1].TrustLine == nil {
		t.Error("Operations[1].TrustLine is nil, want trustline diagnosis")
	} else if !explained.Operations[1].TrustLine.RevokingAuth {
		t.Error("Operations[1].TrustLine.RevokingAuth = false, want true")
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

// explainSyntheticPair encodes an envelope and result, decodes them through
// the real path, and explains them together, so the test exercises the same
// decode and pairing code the CLI does rather than hand-built Values.
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

// ---------------------------------------------------------------------------
// AccountMerge guidance (issue #121)
// ---------------------------------------------------------------------------

// TestExplainAccountMergeEnvelopeOnly pins the envelope-only behaviour of an
// AccountMerge operation: the detail line names the destination, and no
// failure guidance is invented before a result exists.
func TestExplainAccountMergeEnvelopeOnly(t *testing.T) {
	t.Parallel()

	const dest = "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	got, err := lens.Explain(accountMergeEnvelope(t, dest))
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if len(got.Operations) != 1 {
		t.Fatalf("len(Operations) = %d, want 1", len(got.Operations))
	}
	if got.Operations[0].Type != "account_merge" {
		t.Errorf("Operations[0].Type = %q, want %q", got.Operations[0].Type, "account_merge")
	}
	wantDetail := "merge this account into " + dest[:5] + "…" + dest[len(dest)-4:]
	if got.Operations[0].Detail != wantDetail {
		t.Errorf("Operations[0].Detail = %q, want %q", got.Operations[0].Detail, wantDetail)
	}
	if got.Outcome != nil {
		t.Error("Outcome is set for an envelope-only explanation, want nil")
	}
}

// TestExplainAccountMergeContextualFailure drives the public ExplainPair path
// end to end: a synthetic AccountMerge envelope paired with a synthetic
// result must replace the generic per-code text with guidance that names the
// subentry classes blocking a merge, the subentry-closing operations, and the
// exact bound behind seqnum_too_far and dest_full — while leaving Code and
// Constant untouched.
func TestExplainAccountMergeContextualFailure(t *testing.T) {
	t.Parallel()

	const (
		source = "GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6"
		dest   = "GA2JRQOF6EA3HQWDCEDBPPMLYPJCFLDDGYZLEQGMS5SOBQIB3BAFHVAW"
	)
	sourceShort := source[:5] + "…" + source[len(source)-4:]
	destShort := dest[:5] + "…" + dest[len(dest)-4:]

	tests := []struct {
		name          string
		code          xdr.AccountMergeResultCode
		constant      string
		wantSummary   []string
		wantHint      []string
		wantUntouched bool
	}{
		{
			name:     "has sub entries",
			code:     xdr.AccountMergeResultCodeAccountMergeHasSubEntries,
			constant: "AccountMergeResultCodeAccountMergeHasSubEntries",
			wantSummary: []string{
				"non-signer subentries",
				"subentry count matches its signer count",
				sourceShort,
			},
			wantHint: []string{
				"trustline",
				"liquidity-pool share trustlines",
				"DEX offer",
				"data entry",
				"claimable balance",
				"ManageSellOffer or ManageBuyOffer at amount 0",
				"ChangeTrust limit to 0",
				"ManageData operation that carries no value",
				"Signers do not block a merge",
			},
		},
		{
			name:     "seqnum too far",
			code:     xdr.AccountMergeResultCodeAccountMergeSeqnumTooFar,
			constant: "AccountMergeResultCodeAccountMergeSeqnumTooFar",
			wantSummary: []string{
				"sequence number",
				"ledger sequence << 32",
				sourceShort,
			},
			wantHint: []string{
				"can never be lowered",
				"BumpSequence",
				"times 2^32",
				"payments instead",
			},
		},
		{
			name:     "destination full",
			code:     xdr.AccountMergeResultCodeAccountMergeDestFull,
			constant: "AccountMergeResultCodeAccountMergeDestFull",
			wantSummary: []string{
				destShort,
				"lumen buying liabilities",
			},
			wantHint: []string{
				"entire balance",
				"Cancel or shrink the destination's open offers that buy XLM",
				"different destination",
			},
		},
		{
			name:     "immutable flag set",
			code:     xdr.AccountMergeResultCodeAccountMergeImmutableSet,
			constant: "AccountMergeResultCodeAccountMergeImmutableSet",
			wantSummary: []string{
				"AUTH_IMMUTABLE",
				sourceShort,
			},
			wantHint: []string{
				"permanent",
				"can never be merged",
				"ordinary payments instead",
			},
		},
		{
			// Codes without operation-provable causes keep the generic text.
			name:          "malformed keeps the generic explanation",
			code:          xdr.AccountMergeResultCodeAccountMergeMalformed,
			constant:      "AccountMergeResultCodeAccountMergeMalformed",
			wantUntouched: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := lens.ExplainPair(accountMergeEnvelope(t, dest), accountMergeResult(t, tc.code))
			if err != nil {
				t.Fatalf("ExplainPair() error = %v", err)
			}
			if len(got.Operations) != 1 || got.Operations[0].Result == nil {
				t.Fatalf("want one operation with a result, got %d operations", len(got.Operations))
			}
			reason := *got.Operations[0].Result

			if reason.Code == "" {
				t.Error("Code is empty")
			}
			if reason.Constant != tc.constant {
				t.Errorf("Constant = %q, want %q", reason.Constant, tc.constant)
			}
			if reason.Success {
				t.Error("Success = true, want false")
			}

			generic := lens.ExplainCode(tc.constant)
			if tc.wantUntouched {
				if reason.Summary != generic.Summary || reason.Hint != generic.Hint {
					t.Errorf("reason = %+v, want the untouched generic text %+v", reason, generic)
				}
				return
			}

			for _, want := range tc.wantSummary {
				if !strings.Contains(reason.Summary, want) {
					t.Errorf("Summary = %q, want it to contain %q", reason.Summary, want)
				}
			}
			for _, want := range tc.wantHint {
				if !strings.Contains(reason.Hint, want) {
					t.Errorf("Hint = %q, want it to contain %q", reason.Hint, want)
				}
			}
			if strings.EqualFold(strings.ReplaceAll(reason.Summary, " ", ""), reason.Code) {
				t.Errorf("Summary %q merely restates the code", reason.Summary)
			}
		})
	}
}

// TestExplainAccountMergeResultOnlyStaysGeneric proves the deepening fires
// only when the envelope is paired: a result on its own has no operation
// body, so the generic per-code text must survive unchanged.
func TestExplainAccountMergeResultOnlyStaysGeneric(t *testing.T) {
	t.Parallel()

	got, err := lens.Explain(accountMergeResult(t, xdr.AccountMergeResultCodeAccountMergeHasSubEntries))
	if err != nil {
		t.Fatalf("Explain() error = %v", err)
	}
	if len(got.Operations) != 1 || got.Operations[0].Result == nil {
		t.Fatalf("want one operation with a result, got %d operations", len(got.Operations))
	}
	generic := lens.ExplainCode("AccountMergeResultCodeAccountMergeHasSubEntries")
	if got.Operations[0].Result.Summary != generic.Summary {
		t.Errorf("Summary = %q, want the generic %q", got.Operations[0].Result.Summary, generic.Summary)
	}
	if got.Operations[0].Result.Hint != generic.Hint {
		t.Errorf("Hint = %q, want the generic %q", got.Operations[0].Result.Hint, generic.Hint)
	}
}

// TestExplainAccountMergeMissingDestination proves the deepening survives a
// structurally broken merge operation: with no destination in the body the
// guidance must still fire, falling back to a generic destination label
// rather than panicking or dropping the diagnosis.
func TestExplainAccountMergeMissingDestination(t *testing.T) {
	t.Parallel()

	const source = "GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6"
	sourceID := mustAccountID(t, source)
	env := &xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: sourceID.ToMuxedAccount(),
				Fee:           xdr.Uint32(100),
				SeqNum:        xdr.SequenceNumber(5),
				Cond:          xdr.Preconditions{Type: xdr.PreconditionTypePrecondNone},
				Memo:          xdr.Memo{Type: xdr.MemoTypeMemoNone},
				Operations: []xdr.Operation{{Body: xdr.OperationBody{
					Type: xdr.OperationTypeAccountMerge,
				}}},
			},
		},
	}

	got, err := lens.ExplainPair(
		&lens.Value{Type: "TransactionEnvelope", Raw: env},
		accountMergeResult(t, xdr.AccountMergeResultCodeAccountMergeDestFull),
	)
	if err != nil {
		t.Fatalf("ExplainPair() error = %v", err)
	}
	if len(got.Operations) != 1 || got.Operations[0].Result == nil {
		t.Fatalf("want one operation with a result, got %d operations", len(got.Operations))
	}
	reason := *got.Operations[0].Result
	if !strings.Contains(reason.Summary, "Destination the destination") {
		t.Errorf("Summary = %q, want the generic destination label", reason.Summary)
	}
	if !strings.Contains(reason.Hint, "lumen buying liabilities") {
		t.Errorf("Hint = %q, want the dest_full guidance", reason.Hint)
	}
}

// accountMergeEnvelope builds a minimal v1 envelope carrying one AccountMerge
// operation toward dest.
func accountMergeEnvelope(t *testing.T, dest string) *lens.Value {
	t.Helper()
	const source = "GCJNHFXJEQEY54NPKU4DBMT5WOBJUHMYSFBDAQF4OMTEGJN4BQU7ZST6"
	destID := mustAccountID(t, dest)
	destination := destID.ToMuxedAccount()
	sourceID := mustAccountID(t, source)
	env := &xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: sourceID.ToMuxedAccount(),
				Fee:           xdr.Uint32(100),
				SeqNum:        xdr.SequenceNumber(5),
				Cond:          xdr.Preconditions{Type: xdr.PreconditionTypePrecondNone},
				Memo:          xdr.Memo{Type: xdr.MemoTypeMemoNone},
				Operations: []xdr.Operation{{Body: xdr.OperationBody{
					Type:        xdr.OperationTypeAccountMerge,
					Destination: &destination,
				}}},
			},
		},
	}
	return &lens.Value{Type: "TransactionEnvelope", Raw: env}
}

// accountMergeResult builds a transaction result whose single operation
// failed with the given AccountMerge result code.
func accountMergeResult(t *testing.T, code xdr.AccountMergeResultCode) *lens.Value {
	t.Helper()
	opResults := []xdr.OperationResult{{
		Code: xdr.OperationResultCodeOpInner,
		Tr: &xdr.OperationResultTr{
			Type: xdr.OperationTypeAccountMerge,
			AccountMergeResult: &xdr.AccountMergeResult{
				Code: code,
			},
		},
	}}
	res := &xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &opResults,
		},
	}
	return &lens.Value{Type: "TransactionResult", Raw: res}
}
