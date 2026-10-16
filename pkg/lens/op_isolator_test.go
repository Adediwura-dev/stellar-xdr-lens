package lens_test

import (
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestIsolateFailureSuccess(t *testing.T) {
	t.Parallel()

	resPayload := loadFixture(t, "tx_success.res")
	decodedRes, err := lens.Decode(resPayload)
	if err != nil {
		t.Fatalf("decoding res: %v", err)
	}

	res := decodedRes.Raw.(*xdr.TransactionResult)
	isolated, err := lens.IsolateResult(res)
	if err != nil {
		t.Fatalf("IsolateResult failed: %v", err)
	}

	if !isolated.IsSuccess {
		t.Errorf("expected IsSuccess == true, got false")
	}
	if isolated.Stage != lens.StageNone {
		t.Errorf("expected StageNone, got %s", isolated.Stage)
	}
	if isolated.FailingOpIndex != -1 {
		t.Errorf("expected FailingOpIndex == -1, got %d", isolated.FailingOpIndex)
	}
	if isolated.PreOpCheck || isolated.ExecutionFailure {
		t.Errorf("expected both PreOpCheck and ExecutionFailure to be false for success")
	}
}

func TestIsolateFailureCanonicalFailedFixture(t *testing.T) {
	t.Parallel()

	envPayload := loadFixture(t, "tx_failed.env")
	resPayload := loadFixture(t, "tx_failed.res")

	decodedEnv, err := lens.Decode(envPayload)
	if err != nil {
		t.Fatalf("decoding env: %v", err)
	}
	decodedRes, err := lens.Decode(resPayload)
	if err != nil {
		t.Fatalf("decoding res: %v", err)
	}

	// Test IsolateFailureFromValue
	isolated, err := lens.IsolateFailureFromValue(decodedRes, decodedEnv)
	if err != nil {
		t.Fatalf("IsolateFailureFromValue failed: %v", err)
	}

	if isolated.IsSuccess {
		t.Error("expected IsSuccess == false")
	}
	if isolated.Stage != lens.StageOperation {
		t.Errorf("expected StageOperation, got %s", isolated.Stage)
	}
	if !isolated.ExecutionFailure {
		t.Error("expected ExecutionFailure == true")
	}
	if isolated.PreOpCheck {
		t.Error("expected PreOpCheck == false")
	}
	if isolated.FailingOpIndex != 0 {
		t.Errorf("expected FailingOpIndex == 0, got %d", isolated.FailingOpIndex)
	}
	if isolated.ResultCode != "invoke_host_function_trapped" {
		t.Errorf("expected invoke_host_function_trapped, got %q", isolated.ResultCode)
	}

	report := isolated.DiagnosticReport()
	if !strings.Contains(report, "invoke_host_function_trapped") {
		t.Errorf("expected DiagnosticReport to mention invoke_host_function_trapped: %s", report)
	}
	if !strings.Contains(isolated.String(), "operation 0") {
		t.Errorf("expected String() to mention operation 0: %s", isolated.String())
	}
}

func TestIsolateFailurePreOperationCheck(t *testing.T) {
	t.Parallel()

	// Construct a synthetic transaction result failing at transaction level (pre-op)
	res := &xdr.TransactionResult{
		FeeCharged: 100,
		Result: xdr.TransactionResultResult{
			Code: xdr.TransactionResultCodeTxBadSeq,
		},
	}

	isolated, err := lens.IsolateFailure(res, nil)
	if err != nil {
		t.Fatalf("IsolateFailure failed: %v", err)
	}

	if isolated.Stage != lens.StagePreOperation {
		t.Errorf("expected StagePreOperation, got %s", isolated.Stage)
	}
	if !isolated.PreOpCheck {
		t.Error("expected PreOpCheck == true")
	}
	if isolated.ExecutionFailure {
		t.Error("expected ExecutionFailure == false")
	}
	if isolated.FailingOpIndex != -1 {
		t.Errorf("expected FailingOpIndex == -1, got %d", isolated.FailingOpIndex)
	}
	if isolated.ResultCode != "tx_bad_seq" {
		t.Errorf("expected tx_bad_seq, got %q", isolated.ResultCode)
	}
	if !strings.Contains(isolated.Summary, "Pre-operation check failed") {
		t.Errorf("expected summary to indicate pre-operation check failure: %s", isolated.Summary)
	}
}

func TestIsolateFailureMultiOperationPinpoint(t *testing.T) {
	t.Parallel()

	// Construct a multi-operation synthetic transaction result:
	// op 0: success (payment)
	// op 1: failed (payment_underfunded)
	// op 2: failed (op_bad_auth)
	successTr := &xdr.OperationResultTr{
		Type: xdr.OperationTypePayment,
		PaymentResult: &xdr.PaymentResult{
			Code: xdr.PaymentResultCodePaymentSuccess,
		},
	}
	failTr := &xdr.OperationResultTr{
		Type: xdr.OperationTypePayment,
		PaymentResult: &xdr.PaymentResult{
			Code: xdr.PaymentResultCodePaymentUnderfunded,
		},
	}

	opResults := []xdr.OperationResult{
		{
			Code: xdr.OperationResultCodeOpInner,
			Tr:   successTr,
		},
		{
			Code: xdr.OperationResultCodeOpInner,
			Tr:   failTr,
		},
		{
			Code: xdr.OperationResultCodeOpBadAuth,
		},
	}

	res := &xdr.TransactionResult{
		FeeCharged: 300,
		Result: xdr.TransactionResultResult{
			Code:    xdr.TransactionResultCodeTxFailed,
			Results: &opResults,
		},
	}

	isolated, err := lens.IsolateFailure(res, nil)
	if err != nil {
		t.Fatalf("IsolateFailure failed: %v", err)
	}

	if isolated.Stage != lens.StageOperation {
		t.Errorf("expected StageOperation, got %s", isolated.Stage)
	}
	if isolated.FailingOpIndex != 1 {
		t.Errorf("expected FailingOpIndex == 1, got %d", isolated.FailingOpIndex)
	}
	if isolated.FailingOpType != "payment" {
		t.Errorf("expected FailingOpType == payment, got %s", isolated.FailingOpType)
	}
	if isolated.ResultCode != "payment_underfunded" {
		t.Errorf("expected payment_underfunded, got %q", isolated.ResultCode)
	}
	if len(isolated.AllFailedOps) != 2 || isolated.AllFailedOps[0] != 1 || isolated.AllFailedOps[1] != 2 {
		t.Errorf("expected AllFailedOps == [1, 2], got %v", isolated.AllFailedOps)
	}
}

func TestIsolateFailureNilHandling(t *testing.T) {
	t.Parallel()

	_, err := lens.IsolateFailure(nil, nil)
	if err == nil {
		t.Error("expected error on nil TransactionResult")
	}

	_, err = lens.IsolateFailureFromValue(nil, nil)
	if err == nil {
		t.Error("expected error on nil Value")
	}

	var nilIsolated *lens.IsolatedFailure
	if nilIsolated.String() != "<nil>" {
		t.Errorf("expected <nil>, got %s", nilIsolated.String())
	}
	if nilIsolated.DiagnosticReport() != "" {
		t.Errorf("expected empty report for nil, got %s", nilIsolated.DiagnosticReport())
	}
}
