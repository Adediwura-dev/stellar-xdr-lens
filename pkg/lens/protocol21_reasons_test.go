package lens_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar-optics/stellar-xdr-lens/pkg/lens"
)

func TestIsProtocol21Code(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  bool
	}{
		{"TransactionResultCodeTxFrozenKeyAccessed", true},
		{"tx_frozen_key_accessed", true},
		{"InvokeHostFunctionResultCodeInvokeHostFunctionEntryArchived", true},
		{"invoke_host_function_entry_archived", true},
		{"InvokeHostFunctionResultCodeInvokeHostFunctionInsufficientRefundableFee", true},
		{"invoke_host_function_insufficient_refundable_fee", true},
		{"ExtendFootprintTtlResultCodeExtendFootprintTtlInsufficientRefundableFee", true},
		{"restore_footprint_insufficient_refundable_fee", true},
		{"tx_success", false},
		{"payment_underfunded", false},
		{"unknown_random_code", false},
	}

	for _, tt := range tests {
		got := lens.IsProtocol21Code(tt.input)
		if got != tt.want {
			t.Errorf("IsProtocol21Code(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestProtocol21Reasons(t *testing.T) {
	t.Parallel()

	reasons := lens.Protocol21Reasons()
	if len(reasons) == 0 {
		t.Fatal("Protocol21Reasons returned empty map")
	}

	expectedCodes := []string{
		"tx_frozen_key_accessed",
		"invoke_host_function_entry_archived",
		"invoke_host_function_insufficient_refundable_fee",
		"extend_footprint_ttl_insufficient_refundable_fee",
		"restore_footprint_insufficient_refundable_fee",
	}

	for _, code := range expectedCodes {
		r, ok := reasons[code]
		if !ok {
			t.Errorf("expected Protocol21Reasons to contain %q", code)
			continue
		}
		if r.Summary == "" {
			t.Errorf("reason %q has empty summary", code)
		}
		if r.Hint == "" {
			t.Errorf("reason %q has empty hint", code)
		}
	}
}

func TestExplainProtocol21Code(t *testing.T) {
	t.Parallel()

	reason, ok := lens.ExplainProtocol21Code("tx_frozen_key_accessed")
	if !ok {
		t.Fatal("expected ExplainProtocol21Code to find tx_frozen_key_accessed")
	}
	if reason.Code != "tx_frozen_key_accessed" {
		t.Errorf("got code %q, want tx_frozen_key_accessed", reason.Code)
	}

	// Test by constant name
	reasonConst, ok := lens.ExplainProtocol21Code("TransactionResultCodeTxFrozenKeyAccessed")
	if !ok {
		t.Fatal("expected ExplainProtocol21Code to find by constant name")
	}
	if reasonConst.Code != "tx_frozen_key_accessed" {
		t.Errorf("got code %q, want tx_frozen_key_accessed", reasonConst.Code)
	}

	_, ok = lens.ExplainProtocol21Code("tx_bad_seq")
	if ok {
		t.Errorf("expected ExplainProtocol21Code to return false for legacy code tx_bad_seq")
	}
}

func TestExplainHostErrorType(t *testing.T) {
	t.Parallel()

	types := []xdr.ScErrorType{
		xdr.ScErrorTypeSceContract,
		xdr.ScErrorTypeSceWasmVm,
		xdr.ScErrorTypeSceContext,
		xdr.ScErrorTypeSceStorage,
		xdr.ScErrorTypeSceObject,
		xdr.ScErrorTypeSceCrypto,
		xdr.ScErrorTypeSceEvents,
		xdr.ScErrorTypeSceBudget,
		xdr.ScErrorTypeSceValue,
		xdr.ScErrorTypeSceAuth,
		xdr.ScErrorType(999), // unknown
	}

	for _, ty := range types {
		desc := lens.ExplainHostErrorType(ty)
		if desc == "" {
			t.Errorf("ExplainHostErrorType(%v) returned empty string", ty)
		}
	}
}

func TestExplainHostErrorCode(t *testing.T) {
	t.Parallel()

	codes := []xdr.ScErrorCode{
		xdr.ScErrorCodeScecArithDomain,
		xdr.ScErrorCodeScecIndexBounds,
		xdr.ScErrorCodeScecInvalidInput,
		xdr.ScErrorCodeScecMissingValue,
		xdr.ScErrorCodeScecExistingValue,
		xdr.ScErrorCodeScecExceededLimit,
		xdr.ScErrorCodeScecInvalidAction,
		xdr.ScErrorCodeScecInternalError,
		xdr.ScErrorCodeScecUnexpectedType,
		xdr.ScErrorCodeScecUnexpectedSize,
		xdr.ScErrorCode(999), // unknown
	}

	for _, c := range codes {
		r := lens.ExplainHostErrorCode(c)
		if r.Summary == "" {
			t.Errorf("ExplainHostErrorCode(%v) returned empty summary", c)
		}
	}
}

func TestExplainScError(t *testing.T) {
	t.Parallel()

	// 1. Contract error
	code42 := xdr.Uint32(42)
	contractErr := xdr.ScError{
		Type:         xdr.ScErrorTypeSceContract,
		ContractCode: &code42,
	}
	r1 := lens.ExplainScError(contractErr)
	if r1.Code != "contract_error_42" {
		t.Errorf("expected contract_error_42, got %q", r1.Code)
	}

	bareContractErr := xdr.ScError{
		Type: xdr.ScErrorTypeSceContract,
	}
	r1Bare := lens.ExplainScError(bareContractErr)
	if r1Bare.Code != "contract_error" {
		t.Errorf("expected contract_error, got %q", r1Bare.Code)
	}

	// 2. Host error
	limitCode := xdr.ScErrorCodeScecExceededLimit
	hostErr := xdr.ScError{
		Type: xdr.ScErrorTypeSceBudget,
		Code: &limitCode,
	}
	r2 := lens.ExplainScError(hostErr)
	if r2.Code != "scec_exceeded_limit" {
		t.Errorf("expected scec_exceeded_limit, got %q", r2.Code)
	}

	// 3. Host error without inner code
	bareErr := xdr.ScError{
		Type: xdr.ScErrorTypeSceWasmVm,
	}
	r3 := lens.ExplainScError(bareErr)
	if r3.Code != "host_error" {
		t.Errorf("expected host_error, got %q", r3.Code)
	}
}

func ptrUint32(v uint32) *uint32 {
	return &v
}

func TestProtocol21ReasonsAgainstCanonicalFixtures(t *testing.T) {
	t.Parallel()

	// Load tx_soroban.env.txt and tx_soroban.res.txt fixtures
	readFixture := func(filename string) []byte {
		for _, prefix := range []string{"../../testdata/", "testdata/", "../testdata/"} {
			if b, err := os.ReadFile(prefix + filename); err == nil {
				return b
			}
		}
		t.Skipf("cannot read fixture %s", filename)
		return nil
	}

	envBytes := readFixture("tx_soroban.env.txt")
	resBytes := readFixture("tx_soroban.res.txt")

	decodedEnv, err := lens.Decode(strings.TrimSpace(string(envBytes)))
	if err != nil {
		t.Fatalf("decoding env fixture failed: %v", err)
	}
	decodedRes, err := lens.Decode(strings.TrimSpace(string(resBytes)))
	if err != nil {
		t.Fatalf("decoding res fixture failed: %v", err)
	}

	summary, err := lens.ExplainPair(decodedEnv, decodedRes)
	if err != nil {
		t.Fatalf("explaining paired fixture failed: %v", err)
	}

	if len(summary.Operations) == 0 {
		t.Fatal("expected operations in soroban fixture")
	}
	op := summary.Operations[0]
	if op.Type != "invoke_host_function" {
		t.Errorf("expected invoke_host_function, got %s", op.Type)
	}
	if op.Result == nil || op.Result.Code != "invoke_host_function_success" {
		t.Errorf("expected invoke_host_function_success, got %+v", op.Result)
	}
}
