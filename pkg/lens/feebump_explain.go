package lens

import (
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// FeeBumpStage names which half of a fee-bump transaction failed.
type FeeBumpStage string

const (
	// FeeBumpStageNone indicates the transaction (fee-bump or not) succeeded.
	FeeBumpStageNone FeeBumpStage = "none"
	// FeeBumpStageInner indicates the fee-bump wrapper was valid but the inner
	// transaction it paid for failed. The fee was still charged.
	FeeBumpStageInner FeeBumpStage = "inner"
	// FeeBumpStageOuter indicates the fee-bump wrapper itself was rejected
	// before the inner transaction ran. The inner cause is not the problem.
	FeeBumpStageOuter FeeBumpStage = "outer"
	// FeeBumpStageNotFeeBump indicates the transaction is not fee-bumped at all.
	FeeBumpStageNotFeeBump FeeBumpStage = "not_fee_bump"
)

// FeeBumpDiagnosis explains a fee-bump failure by separating the outer
// wrapper's fate from the inner transaction's fate.
type FeeBumpDiagnosis struct {
	// IsFeeBump reports whether the envelope is a fee-bump transaction.
	IsFeeBump bool `json:"isFeeBump"`
	// Success reports whether the transaction as a whole succeeded.
	Success bool `json:"success"`
	// Stage names which half failed: inner, outer, none, or not_fee_bump.
	Stage FeeBumpStage `json:"stage"`
	// OuterCode is the short code of the outer result (e.g. "tx_fee_bump_inner_failed").
	OuterCode string `json:"outerCode"`
	// OuterConstant is the generated XDR constant for the outer code.
	OuterConstant string `json:"outerConstant,omitempty"`
	// OuterReason is the plain-English outer result.
	OuterReason Reason `json:"outerReason"`
	// InnerCode is the short code of the inner result, when this is a fee bump.
	InnerCode string `json:"innerCode,omitempty"`
	// InnerReason is the plain-English inner result, when this is a fee bump.
	InnerReason *Reason `json:"innerReason,omitempty"`
	// FeeSource is the strkey address charged the fee (fee-bump source for
	// fee bumps, transaction source otherwise).
	FeeSource string `json:"feeSource,omitempty"`
	// InnerSource is the strkey address of the inner transaction, for fee bumps.
	InnerSource string `json:"innerSource,omitempty"`
	// FeeCharged is the fee actually taken, in stroops.
	FeeCharged int64 `json:"feeCharged"`
	// FailedOperations lists failing inner operation indexes, when known.
	FailedOperations []int `json:"failedOperations,omitempty"`
	// Summary explains which half failed and what to look at next.
	Summary string `json:"summary"`
	// Hint suggests the next debugging step.
	Hint string `json:"hint,omitempty"`
}

// DiagnoseFeeBump explains a fee-bump (or non-fee-bump) transaction result by
// distinguishing outer wrapper errors from inner transaction errors.
//
// env must be a TransactionEnvelope and res a TransactionResult; anything else
// is an ErrInvalidArgument. The computation is purely local and offline.
func DiagnoseFeeBump(env *xdr.TransactionEnvelope, res *xdr.TransactionResult) (*FeeBumpDiagnosis, error) {
	if env == nil {
		return nil, fmt.Errorf("%w: cannot diagnose fee bump from nil TransactionEnvelope", ErrInvalidArgument)
	}
	if res == nil {
		return nil, fmt.Errorf("%w: cannot diagnose fee bump from nil TransactionResult", ErrInvalidArgument)
	}
	outerName, err := resultCodeName(res)
	if err != nil {
		return nil, err
	}
	outerReason := ExplainCode(outerName)
	d := &FeeBumpDiagnosis{
		IsFeeBump:     env.IsFeeBump(),
		Success:       res.Successful(),
		OuterCode:     outerReason.Code,
		OuterConstant: outerReason.Constant,
		OuterReason:   outerReason,
		FeeCharged:    int64(res.FeeCharged),
	}
	if !d.IsFeeBump {
		d.Stage = FeeBumpStageNotFeeBump
		d.FeeSource = addressOf(env.SourceAccount())
		d.FailedOperations = failedOpIndexes(res)
		d.Summary = fmt.Sprintf("Not a fee-bump transaction (%s): %s", d.OuterCode, outerReason.Summary)
		if !d.Success {
			d.Hint = outerReason.Hint
			if d.Hint == "" {
				d.Hint = "Inspect the transaction result and the per-operation results for the real cause."
			}
		}
		return d, nil
	}

	// Fee-bump path: record who paid and whose transaction it was.
	d.FeeSource = addressOf(env.FeeBumpAccount())
	d.InnerSource = addressOf(env.SourceAccount())
	if innerName, ok := innerResultCodeName(res); ok {
		inner := ExplainCode(innerName)
		d.InnerCode = inner.Code
		d.InnerReason = &inner
		// Keep the generated constant for log correlation.
		d.InnerReason.Constant = inner.Constant
	}
	d.FailedOperations = failedOpIndexes(res)

	switch res.Result.Code {
	case xdr.TransactionResultCodeTxFeeBumpInnerSuccess:
		d.Stage = FeeBumpStageNone
		d.Summary = fmt.Sprintf("Fee-bump transaction succeeded: the wrapper and the inner transaction both applied (fee %d stroops charged to %s).",
			d.FeeCharged, shortAddr(d.FeeSource))
	case xdr.TransactionResultCodeTxFeeBumpInnerFailed:
		d.Stage = FeeBumpStageInner
		innerSummary := ""
		if d.InnerReason != nil {
			innerSummary = d.InnerReason.Summary
		}
		d.Summary = fmt.Sprintf("Fee-bump wrapper was valid, but the inner transaction failed (%s): %s The fee (%d stroops) was still charged to the fee-bump source %s.",
			d.InnerCode, innerSummary, d.FeeCharged, shortAddr(d.FeeSource))
		if len(d.FailedOperations) > 0 {
			d.Summary += fmt.Sprintf(" Failing operation(s): %v.", d.FailedOperations)
		}
		if d.InnerReason != nil && d.InnerReason.Hint != "" {
			d.Hint = d.InnerReason.Hint + " The outer fee-bump envelope is not the problem; do not rebuild it."
		} else {
			d.Hint = "Inspect the inner transaction result for the real cause. The outer fee-bump envelope is not the problem; do not rebuild it."
		}
	default:
		// Any other outer code means the wrapper itself was rejected: bad fee-source
		// sequence, unfunded fee source, bad auth on the wrapper, and so on.
		// The inner transaction never ran.
		d.Stage = FeeBumpStageOuter
		d.Summary = fmt.Sprintf("Fee-bump wrapper was rejected before the inner transaction ran (%s): %s The inner transaction was not applied; fix the fee-bump envelope (fee source %s).",
			d.OuterCode, outerReason.Summary, shortAddr(d.FeeSource))
		d.Hint = outerReason.Hint
		if d.Hint == "" {
			d.Hint = "Check the fee-bump source account (funding, sequence number, signatures) rather than the inner operations."
		} else {
			d.Hint += " The inner operations never ran, so their result codes are not the cause."
		}
	}
	return d, nil
}

// DiagnoseFeeBumpFromValues is the Value-based wrapper around DiagnoseFeeBump:
// envelopeVal must wrap *xdr.TransactionEnvelope and resultVal must wrap
// *xdr.TransactionResult.
func DiagnoseFeeBumpFromValues(envelopeVal *Value, resultVal *Value) (*FeeBumpDiagnosis, error) {
	if envelopeVal == nil || resultVal == nil {
		return nil, fmt.Errorf("%w: envelope and result values are required", ErrInvalidArgument)
	}
	env, ok := envelopeVal.Raw.(*xdr.TransactionEnvelope)
	if !ok {
		return nil, fmt.Errorf("%w: envelope value is %T, want *xdr.TransactionEnvelope", ErrInvalidArgument, envelopeVal.Raw)
	}
	res, ok := resultVal.Raw.(*xdr.TransactionResult)
	if !ok {
		return nil, fmt.Errorf("%w: result value is %T, want *xdr.TransactionResult", ErrInvalidArgument, resultVal.Raw)
	}
	return DiagnoseFeeBump(env, res)
}

// String renders the diagnosis as a single log-friendly line.
func (d *FeeBumpDiagnosis) String() string {
	if d == nil {
		return "<nil>"
	}
	return d.Summary
}

// DiagnosticReport renders a multi-line report suitable for CLI output.
func (d *FeeBumpDiagnosis) DiagnosticReport() string {
	if d == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("=== Fee-Bump Diagnosis ===\n")
	fmt.Fprintf(&b, "Fee bump:          %t\n", d.IsFeeBump)
	fmt.Fprintf(&b, "Stage:             %s\n", d.Stage)
	fmt.Fprintf(&b, "Success:           %t\n", d.Success)
	fmt.Fprintf(&b, "Outer:             %s\n", d.OuterCode)
	if d.OuterConstant != "" {
		fmt.Fprintf(&b, "Outer constant:    %s\n", d.OuterConstant)
	}
	fmt.Fprintf(&b, "Outer summary:     %s\n", d.OuterReason.Summary)
	if d.InnerCode != "" {
		fmt.Fprintf(&b, "Inner:             %s\n", d.InnerCode)
		if d.InnerReason != nil {
			fmt.Fprintf(&b, "Inner summary:     %s\n", d.InnerReason.Summary)
		}
	}
	if d.FeeSource != "" {
		fmt.Fprintf(&b, "Fee source:        %s\n", d.FeeSource)
	}
	if d.InnerSource != "" {
		fmt.Fprintf(&b, "Inner source:      %s\n", d.InnerSource)
	}
	fmt.Fprintf(&b, "Fee charged:       %d\n", d.FeeCharged)
	if len(d.FailedOperations) > 0 {
		fmt.Fprintf(&b, "Failed ops:        %v\n", d.FailedOperations)
	}
	if d.Hint != "" {
		fmt.Fprintf(&b, "Hint:              %s\n", d.Hint)
	}
	return b.String()
}

func failedOpIndexes(res *xdr.TransactionResult) []int {
	opResults, ok := res.OperationResults()
	if !ok {
		return nil
	}
	var out []int
	for i, opRes := range opResults {
		if !explainOperationResult(opRes).Success {
			out = append(out, i)
		}
	}
	return out
}
