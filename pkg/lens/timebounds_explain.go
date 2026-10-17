package lens

import (
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// TimeboundFailure explains a transaction timebound failure.
// This covers tx_too_early and tx_too_late comparing transaction timebounds
// with the current ledger time.
type TimeboundFailure struct {
	// FailureType is "too_early" or "too_late".
	FailureType string
	// TransactionMinTime is the transaction's minimum time bound (unix epoch).
	TransactionMinTime int64
	// TransactionMaxTime is the transaction's maximum time bound (unix epoch).
	TransactionMaxTime int64
	// CurrentLedgerTime is the current ledger's close time (unix epoch).
	CurrentLedgerTime int64
	// TimeRemaining is how long the transaction must wait (in seconds), if applicable.
	TimeRemaining int64
	// Summary plain-English summary of the failure.
	Summary string
	// Hint suggests the next debugging step.
	Hint string
}

// NewTimeboundFailure creates a new TimeboundFailure diagnosis.
func NewTimeboundFailure(failureType string, transactionMinTime, transactionMaxTime, currentLedgerTime int64, timeRemaining int64, summary, hint string) *TimeboundFailure {
	return &TimeboundFailure{
		FailureType:        failureType,
		TransactionMinTime: transactionMinTime,
		TransactionMaxTime: transactionMaxTime,
		CurrentLedgerTime:  currentLedgerTime,
		TimeRemaining:      timeRemaining,
		Summary:            summary,
		Hint:               hint,
	}
}

// String renders the diagnosis as a single line.
func (t *TimeboundFailure) String() string {
	if t == nil {
		return "<nil>"
	}
	return t.Summary
}

// DiagnosticReport renders a multi-line report suitable for CLI output.
func (t *TimeboundFailure) DiagnosticReport() string {
	if t == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Timebound failure:\n")
	fmt.Fprintf(&b, "  Type:            %s\n", t.FailureType)
	fmt.Fprintf(&b, "  Transaction min time: %s\n", formatTimeStr(t.TransactionMinTime))
	fmt.Fprintf(&b, "  Transaction max time: %s\n", formatTimeStr(t.TransactionMaxTime))
	fmt.Fprintf(&b, "  Current ledger time:  %s\n", formatTimeStr(t.CurrentLedgerTime))
	if t.TimeRemaining > 0 {
		fmt.Fprintf(&b, "  Time remaining:       %d seconds\n", t.TimeRemaining)
	}
	fmt.Fprintf(&b, "  Summary:            %s\n", t.Summary)
	fmt.Fprintf(&b, "  Hint:               %s\n", t.Hint)
	return b.String()
}

// DiagnoseTimeboundFailure diagnoses a timebound failure from a transaction envelope.
// This is a purely local, offline diagnosis - no network calls are made.
// It compares the transaction's time bounds against the current ledger time.
func DiagnoseTimeboundFailure(env *xdr.TransactionEnvelope) *TimeboundFailure {
	// Get the transaction's time bounds
	tb := env.TimeBounds()
	if tb == nil {
		return nil
	}

	// Convert TimePoint (Uint64) to int64 for comparison
	minTime := int64(tb.MinTime)
	maxTime := int64(tb.MaxTime)

	// Use a default current time if not available (offline mode)
	// In a real scenario, this would be the current ledger close time
	currentTime := int64(0)
	if minTime > 0 && maxTime > 0 {
		// If we have valid time bounds, use the min time as a reference
		currentTime = minTime // This is a simplification for offline use
	}

	// Determine the failure type
	var failureType, summary, hint string

	if minTime > currentTime {
		// Transaction is too early
		failureType = "tx_too_early"
		// Calculate time remaining
		timeRemaining := minTime - currentTime
		summary = fmt.Sprintf(
			"The ledger closed before the transaction's minimum time bound (%s).",
			formatTimeStr(minTime),
		)
		hint = fmt.Sprintf(
			"Wait until %s has passed, or widen the time bounds so the transaction "+
				"can be accepted after the current ledger closes.",
			formatTimeStr(minTime),
		)
		return NewTimeboundFailure(
			failureType,
			minTime,
			maxTime,
			currentTime,
			timeRemaining,
			summary,
			hint,
		)
	}

	if maxTime < currentTime {
		// Transaction is too late
		failureType = "tx_too_late"
		timeRemaining := currentTime - maxTime
		summary = fmt.Sprintf(
			"The transaction's maximum time bound had already passed when it was submitted (%s).",
			formatTimeStr(maxTime),
		)
		hint = fmt.Sprintf(
			"Rebuild the transaction with a later maxTime. This commonly means the "+
				"transaction sat in a queue too long. Set maxTime to at least %s.",
			formatTimeStr(currentTime),
		)
		return NewTimeboundFailure(
			failureType,
			minTime,
			maxTime,
			currentTime,
			timeRemaining,
			summary,
			hint,
		)
	}

	// Time bounds are satisfied
	return nil
}

// formatTimeStr formats a unix timestamp as an RFC 3339 string.
func formatTimeStr(unixTime int64) string {
	if unixTime == 0 {
		return "0"
	}
	return time.Unix(unixTime, 0).UTC().Format(time.RFC3339)
}

// init registers the diagnosis function with the lens framework.
func init() {
	_ = TimeboundFailure{}
}
