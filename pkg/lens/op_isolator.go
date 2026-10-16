package lens

import (
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// FailureStage describes the phase in which a transaction failed.
type FailureStage string

const (
	// StageNone indicates the transaction succeeded.
	StageNone FailureStage = "none"
	// StagePreOperation indicates a failure during transaction validation or
	// precondition checks before any operation was executed.
	StagePreOperation FailureStage = "pre_operation"
	// StageOperation indicates that pre-operation checks passed, but an operation
	// failed during execution.
	StageOperation FailureStage = "operation"
	// StageFeeBumpWrapper indicates that the outer fee-bump wrapper failed.
	StageFeeBumpWrapper FailureStage = "fee_bump_wrapper"
)

// IsolatedFailure pinpoints the failure locus, isolating whether the failure
// occurred in pre-operation checks or during operation execution, and identifying
// the exact failing operation index and reason.
type IsolatedFailure struct {
	// Stage identifies when the failure occurred: pre_operation, operation, or fee_bump_wrapper.
	Stage FailureStage `json:"stage"`
	// IsSuccess reports true if the transaction succeeded.
	IsSuccess bool `json:"isSuccess"`
	// PreOpCheck is true if the failure occurred during transaction header/precondition checks.
	PreOpCheck bool `json:"preOpCheck"`
	// ExecutionFailure is true if the failure occurred during operation execution.
	ExecutionFailure bool `json:"executionFailure"`
	// FailingOpIndex is the 0-based index of the first failing operation, or -1 if pre-operation or success.
	FailingOpIndex int `json:"failingOpIndex"`
	// FailingOpType is the operation type name (e.g. "payment", "manage_buy_offer"), if available.
	FailingOpType string `json:"failingOpType,omitempty"`
	// ResultCode is the canonical short code (e.g. "tx_bad_seq", "payment_underfunded").
	ResultCode string `json:"resultCode"`
	// Constant is the generated XDR enum constant name.
	Constant string `json:"constant"`
	// Reason is the plain-English explanation of the failure.
	Reason Reason `json:"reason"`
	// Summary is a diagnostic sentence explaining the failure and its location.
	Summary string `json:"summary"`
	// AllFailedOps lists all operation indexes that reported a failure.
	AllFailedOps []int `json:"allFailedOps,omitempty"`
}

// IsolateFailure pinpoints the failing operation index and explains pre-operation
// checks vs execution failures from a TransactionResult and an optional TransactionEnvelope.
func IsolateFailure(res *xdr.TransactionResult, env *xdr.TransactionEnvelope) (*IsolatedFailure, error) {
	if res == nil {
		return nil, fmt.Errorf("%w: cannot isolate failure from nil TransactionResult", ErrInvalidArgument)
	}

	// 1. Success check
	if res.Successful() {
		codeName, err := resultCodeName(res)
		if err != nil {
			codeName = "tx_success"
		}
		reason := ExplainCode(codeName)
		return &IsolatedFailure{
			Stage:          StageNone,
			IsSuccess:      true,
			FailingOpIndex: -1,
			ResultCode:     reason.Code,
			Constant:       reason.Constant,
			Reason:         reason,
			Summary:        "Transaction succeeded; all operations completed successfully.",
		}, nil
	}

	// 2. Resolve transaction-level result code
	codeName, err := resultCodeName(res)
	if err != nil {
		return nil, fmt.Errorf("resolving result code name: %w", err)
	}

	// Handle fee-bump wrapper
	if res.Result.Code == xdr.TransactionResultCodeTxFeeBumpInnerFailed {
		if innerName, ok := innerResultCodeName(res); ok {
			if pair, hasPair := res.Result.GetInnerResultPair(); hasPair {
				if pair.Result.Result.Code == xdr.TransactionResultCodeTxFailed {
					if opResults := pair.Result.Result.Results; opResults != nil {
						return isolateOperationFailure(*opResults, env)
					}
				}
				// Inner failed at pre-op level
				codeName = innerName
			}
		}
	}

	// Check if this was an execution failure (txFAILED)
	if res.Result.Code == xdr.TransactionResultCodeTxFailed {
		opResults, _ := res.OperationResults()
		return isolateOperationFailure(opResults, env)
	}

	// Otherwise, it was a pre-operation check failure (tx_bad_seq, tx_bad_auth, etc.)
	reason := ExplainCode(codeName)
	stage := StagePreOperation
	if res.Result.Code == xdr.TransactionResultCodeTxFeeBumpInnerFailed && !strings.HasPrefix(codeName, "TransactionResultCodeTx") {
		stage = StageFeeBumpWrapper
	}

	summary := fmt.Sprintf("Pre-operation check failed before operations ran (%s): %s",
		reason.Code, reason.Summary)

	return &IsolatedFailure{
		Stage:            stage,
		IsSuccess:        false,
		PreOpCheck:       true,
		ExecutionFailure: false,
		FailingOpIndex:   -1,
		ResultCode:       reason.Code,
		Constant:         reason.Constant,
		Reason:           reason,
		Summary:          summary,
	}, nil
}

// isolateOperationFailure examines operation-level results to locate the failing operation.
func isolateOperationFailure(opResults []xdr.OperationResult, env *xdr.TransactionEnvelope) (*IsolatedFailure, error) {
	if len(opResults) == 0 {
		reason := ExplainCode("TransactionResultCodeTxFailed")
		return &IsolatedFailure{
			Stage:            StageOperation,
			IsSuccess:        false,
			PreOpCheck:       false,
			ExecutionFailure: true,
			FailingOpIndex:   -1,
			ResultCode:       reason.Code,
			Constant:         reason.Constant,
			Reason:           reason,
			Summary:          "Transaction failed during execution, but no operation results were returned.",
		}, nil
	}

	var allFailed []int
	firstFailedIdx := -1
	var firstReason Reason

	for i, opRes := range opResults {
		reason := explainOperationResult(opRes)
		if !reason.Success {
			allFailed = append(allFailed, i)
			if firstFailedIdx == -1 {
				firstFailedIdx = i
				firstReason = reason
			}
		}
	}

	if firstFailedIdx == -1 {
		// Fallback if all operations reported success but tx was marked failed
		firstFailedIdx = 0
		firstReason = explainOperationResult(opResults[0])
	}

	opType := resolveOpType(env, opResults, firstFailedIdx)
	summary := fmt.Sprintf("Execution failed at operation %d", firstFailedIdx)
	if opType != "" && opType != "unknown" {
		summary += fmt.Sprintf(" (%s)", opType)
	}
	summary += fmt.Sprintf(": %s", firstReason.Summary)

	return &IsolatedFailure{
		Stage:            StageOperation,
		IsSuccess:        false,
		PreOpCheck:       false,
		ExecutionFailure: true,
		FailingOpIndex:   firstFailedIdx,
		FailingOpType:    opType,
		ResultCode:       firstReason.Code,
		Constant:         firstReason.Constant,
		Reason:           firstReason,
		Summary:          summary,
		AllFailedOps:     allFailed,
	}, nil
}

// resolveOpType extracts the operation type name from the envelope if available,
// or infers it from the operation result.
func resolveOpType(env *xdr.TransactionEnvelope, opResults []xdr.OperationResult, index int) string {
	if env != nil {
		ops := env.Operations()
		if index >= 0 && index < len(ops) {
			typeName := ops[index].Body.Type.String()
			typeName = strings.TrimPrefix(typeName, "OperationType")
			return toSnake(typeName)
		}
	}

	if index >= 0 && index < len(opResults) {
		opRes := opResults[index]
		if tr, ok := opRes.GetTr(); ok {
			typeName := tr.Type.String()
			typeName = strings.TrimPrefix(typeName, "OperationType")
			return toSnake(typeName)
		}
	}

	return "unknown"
}

// IsolateResult isolates failure details from a TransactionResult alone.
func IsolateResult(res *xdr.TransactionResult) (*IsolatedFailure, error) {
	return IsolateFailure(res, nil)
}

// IsolateFailureFromValue isolates failure details from decoded Value instances.
func IsolateFailureFromValue(resultVal *Value, envelopeVal *Value) (*IsolatedFailure, error) {
	if resultVal == nil {
		return nil, fmt.Errorf("%w: result value is required", ErrInvalidArgument)
	}

	res, ok := resultVal.Raw.(*xdr.TransactionResult)
	if !ok {
		return nil, fmt.Errorf("%w: result value is %T, want *xdr.TransactionResult", ErrInvalidArgument, resultVal.Raw)
	}

	var env *xdr.TransactionEnvelope
	if envelopeVal != nil {
		if e, ok := envelopeVal.Raw.(*xdr.TransactionEnvelope); ok {
			env = e
		}
	}

	return IsolateFailure(res, env)
}

// String formats the isolated failure into a clear diagnostic log line.
func (f *IsolatedFailure) String() string {
	if f == nil {
		return "<nil>"
	}
	return f.Summary
}

// DiagnosticReport produces a multi-line diagnostic report suitable for CLI and debugging.
func (f *IsolatedFailure) DiagnosticReport() string {
	if f == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("=== Failure Isolation Report ===\n")
	fmt.Fprintf(&sb, "Stage:             %s\n", f.Stage)
	fmt.Fprintf(&sb, "Pre-op Check:      %t\n", f.PreOpCheck)
	fmt.Fprintf(&sb, "Execution Failure: %t\n", f.ExecutionFailure)
	if f.FailingOpIndex >= 0 {
		fmt.Fprintf(&sb, "Failing Op Index:  %d\n", f.FailingOpIndex)
		if f.FailingOpType != "" {
			fmt.Fprintf(&sb, "Failing Op Type:   %s\n", f.FailingOpType)
		}
	}
	fmt.Fprintf(&sb, "Result Code:       %s\n", f.ResultCode)
	if f.Constant != "" {
		fmt.Fprintf(&sb, "Constant:          %s\n", f.Constant)
	}
	fmt.Fprintf(&sb, "Summary:           %s\n", f.Reason.Summary)
	if f.Reason.Hint != "" {
		fmt.Fprintf(&sb, "Hint:              %s\n", f.Reason.Hint)
	}
	if len(f.AllFailedOps) > 1 {
		fmt.Fprintf(&sb, "All Failed Ops:    %v\n", f.AllFailedOps)
	}
	return sb.String()
}
