package lens_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func mustEnvelope(t *testing.T, fixture string) *xdr.TransactionEnvelope {
	t.Helper()
	v, err := lens.Decode(loadFixture(t, fixture))
	if err != nil {
		t.Fatalf("Decode(%s) error = %v", fixture, err)
	}
	env, ok := v.Raw.(*xdr.TransactionEnvelope)
	if !ok {
		t.Fatalf("fixture %s is %T, want *xdr.TransactionEnvelope", fixture, v.Raw)
	}
	return env
}

func mustResult(t *testing.T, fixture string) *xdr.TransactionResult {
	t.Helper()
	v, err := lens.Decode(loadFixture(t, fixture))
	if err != nil {
		t.Fatalf("Decode(%s) error = %v", fixture, err)
	}
	res, ok := v.Raw.(*xdr.TransactionResult)
	if !ok {
		t.Fatalf("fixture %s is %T, want *xdr.TransactionResult", fixture, v.Raw)
	}
	return res
}

func TestDiagnoseFeeBumpInnerFailedFixture(t *testing.T) {
	t.Parallel()
	env := mustEnvelope(t, "tx_failed.env")
	res := mustResult(t, "tx_failed.res")
	d, err := lens.DiagnoseFeeBump(env, res)
	if err != nil {
		t.Fatalf("DiagnoseFeeBump() error = %v", err)
	}
	if !d.IsFeeBump {
		t.Error("IsFeeBump = false, want true for tx_failed fixture")
	}
	if d.Success {
		t.Error("Success = true, want false")
	}
	if d.Stage != lens.FeeBumpStageInner {
		t.Errorf("Stage = %q, want inner", d.Stage)
	}
	if d.OuterCode != "tx_fee_bump_inner_failed" {
		t.Errorf("OuterCode = %q, want tx_fee_bump_inner_failed", d.OuterCode)
	}
	if d.InnerCode != "tx_failed" {
		t.Errorf("InnerCode = %q, want tx_failed", d.InnerCode)
	}
	if d.InnerReason == nil {
		t.Fatal("InnerReason is nil")
	}
	if d.FeeSource == "" || d.InnerSource == "" {
		t.Error("FeeSource/InnerSource empty, want strkey addresses")
	}
	if len(d.FailedOperations) != 1 || d.FailedOperations[0] != 0 {
		t.Errorf("FailedOperations = %v, want [0]", d.FailedOperations)
	}
	if !strings.Contains(d.Summary, "inner transaction failed") {
		t.Errorf("Summary = %q, want inner-failure wording", d.Summary)
	}
	if !strings.Contains(d.Summary, "still charged") {
		t.Errorf("Summary = %q, want fee-charged wording", d.Summary)
	}
	if d.Hint == "" {
		t.Error("Hint is empty")
	}
	report := d.DiagnosticReport()
	for _, want := range []string{"inner", "tx_failed", d.FeeSource} {
		if !strings.Contains(report, want) {
			t.Errorf("DiagnosticReport missing %q:\n%s", want, report)
		}
	}
	if d.String() != d.Summary {
		t.Error("String() should return Summary")
	}
}

func TestDiagnoseFeeBumpInnerSuccessFixture(t *testing.T) {
	t.Parallel()
	env := mustEnvelope(t, "tx_soroban.env")
	res := mustResult(t, "tx_soroban.res")
	d, err := lens.DiagnoseFeeBump(env, res)
	if err != nil {
		t.Fatalf("DiagnoseFeeBump() error = %v", err)
	}
	if !d.IsFeeBump || !d.Success {
		t.Errorf("got feeBump=%v success=%v, want true/true", d.IsFeeBump, d.Success)
	}
	if d.Stage != lens.FeeBumpStageNone {
		t.Errorf("Stage = %q, want none", d.Stage)
	}
	if d.OuterCode != "tx_fee_bump_inner_success" {
		t.Errorf("OuterCode = %q, want tx_fee_bump_inner_success", d.OuterCode)
	}
}

func TestDiagnoseFeeBumpOuterFailure(t *testing.T) {
	t.Parallel()
	env := mustEnvelope(t, "tx_failed.env")
	// Outer rejection: fee source cannot pay (wrapper never ran the inner tx).
	res := &xdr.TransactionResult{
		FeeCharged: 24167,
		Result: xdr.TransactionResultResult{
			Code: xdr.TransactionResultCodeTxInsufficientBalance,
		},
	}
	d, err := lens.DiagnoseFeeBump(env, res)
	if err != nil {
		t.Fatalf("DiagnoseFeeBump() error = %v", err)
	}
	if d.Stage != lens.FeeBumpStageOuter {
		t.Errorf("Stage = %q, want outer", d.Stage)
	}
	if d.OuterCode != "tx_insufficient_balance" {
		t.Errorf("OuterCode = %q, want tx_insufficient_balance", d.OuterCode)
	}
	if !strings.Contains(d.Summary, "wrapper was rejected") {
		t.Errorf("Summary = %q, want outer-rejection wording", d.Summary)
	}
	if !strings.Contains(d.Hint, "never ran") {
		t.Errorf("Hint = %q, want never-ran guidance", d.Hint)
	}
}

func TestDiagnoseFeeBumpNotFeeBump(t *testing.T) {
	t.Parallel()
	env := mustEnvelope(t, "tx_success.env")
	res := mustResult(t, "tx_success.res")
	d, err := lens.DiagnoseFeeBump(env, res)
	if err != nil {
		t.Fatalf("DiagnoseFeeBump() error = %v", err)
	}
	if d.IsFeeBump {
		t.Error("IsFeeBump = true for classic tx_success, want false")
	}
	if d.Stage != lens.FeeBumpStageNotFeeBump {
		t.Errorf("Stage = %q, want not_fee_bump", d.Stage)
	}
	if !strings.Contains(d.Summary, "Not a fee-bump") {
		t.Errorf("Summary = %q, want not-fee-bump wording", d.Summary)
	}
	// Failed classic transaction still reports through the same path.
	failedRes := &xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code: xdr.TransactionResultCodeTxFailed,
			Results: &[]xdr.OperationResult{
				{Code: xdr.OperationResultCodeOpBadAuth},
			},
		},
	}
	dFail, err := lens.DiagnoseFeeBump(env, failedRes)
	if err != nil {
		t.Fatalf("failed classic error = %v", err)
	}
	if dFail.Success {
		t.Error("Success = true, want false")
	}
	if len(dFail.FailedOperations) != 1 {
		t.Errorf("FailedOperations = %v, want [0]", dFail.FailedOperations)
	}
}

func TestDiagnoseFeeBumpErrors(t *testing.T) {
	t.Parallel()
	env := mustEnvelope(t, "tx_failed.env")
	res := mustResult(t, "tx_failed.res")
	if _, err := lens.DiagnoseFeeBump(nil, res); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil env error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseFeeBump(env, nil); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil res error = %v, want ErrInvalidArgument", err)
	}
	envVal, err := lens.Decode(loadFixture(t, "tx_failed.env"))
	if err != nil {
		t.Fatalf("Decode(env) error = %v", err)
	}
	resVal, err := lens.Decode(loadFixture(t, "tx_failed.res"))
	if err != nil {
		t.Fatalf("Decode(res) error = %v", err)
	}
	if _, err := lens.DiagnoseFeeBumpFromValues(nil, resVal); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("nil envelope value error = %v, want ErrInvalidArgument", err)
	}
	memo, err := lens.DecodeAs("AAAAAA==", "Memo")
	if err != nil {
		t.Fatalf("DecodeAs(Memo) error = %v", err)
	}
	if _, err := lens.DiagnoseFeeBumpFromValues(memo, resVal); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("wrong envelope type error = %v, want ErrInvalidArgument", err)
	}
	if _, err := lens.DiagnoseFeeBumpFromValues(envVal, memo); !errors.Is(err, lens.ErrInvalidArgument) {
		t.Errorf("wrong result type error = %v, want ErrInvalidArgument", err)
	}
	d, err := lens.DiagnoseFeeBumpFromValues(envVal, resVal)
	if err != nil {
		t.Fatalf("DiagnoseFeeBumpFromValues() error = %v", err)
	}
	if d.Stage != lens.FeeBumpStageInner {
		t.Errorf("Stage = %q, want inner", d.Stage)
	}
	var nilDiag *lens.FeeBumpDiagnosis
	if nilDiag.String() != "<nil>" {
		t.Errorf("nil String() = %q, want <nil>", nilDiag.String())
	}
	if nilDiag.DiagnosticReport() != "" {
		t.Errorf("nil report = %q, want empty", nilDiag.DiagnosticReport())
	}
}
