package lens

import (
	"fmt"
	"strings"
)

// SimulationKind names the root-cause category of a simulateTransaction
// failure. It is a stable, machine-readable value for scripts and CI gates.
type SimulationKind string

const (
	// SimulationKindSuccess indicates simulation reported no error.
	SimulationKindSuccess SimulationKind = "success"
	// SimulationKindMissingFootprint indicates the footprint omits keys the
	// contract reads or writes.
	SimulationKindMissingFootprint SimulationKind = "missing_footprint"
	// SimulationKindCPUExceeded indicates the declared CPU budget was too small.
	SimulationKindCPUExceeded SimulationKind = "cpu_exceeded"
	// SimulationKindMemoryExceeded indicates the declared memory budget was too small.
	SimulationKindMemoryExceeded SimulationKind = "memory_exceeded"
	// SimulationKindInsufficientFee indicates the refundable fee cannot cover
	// rent and events.
	SimulationKindInsufficientFee SimulationKind = "insufficient_fee"
	// SimulationKindArchivedEntry indicates the footprint touches an archived entry.
	SimulationKindArchivedEntry SimulationKind = "archived_entry"
	// SimulationKindAuthMissing indicates required Soroban auth entries are absent.
	SimulationKindAuthMissing SimulationKind = "auth_missing"
	// SimulationKindContractTrapped indicates the contract panicked or returned an error.
	SimulationKindContractTrapped SimulationKind = "contract_trapped"
	// SimulationKindInvalidInput indicates the request itself was malformed.
	SimulationKindInvalidInput SimulationKind = "invalid_input"
	// SimulationKindUnknown covers errors that match no known pattern.
	SimulationKindUnknown SimulationKind = "unknown"
)

// SimulationResponse is the offline subset of a Soroban RPC
// simulateTransaction response that lens diagnoses. It mirrors the JSON field
// names so callers can unmarshal an RPC payload directly, but lens never
// fetches it: callers supply the response they already hold.
type SimulationResponse struct {
	// Error is the RPC error string. Empty means simulation succeeded.
	Error string `json:"error,omitempty"`
	// MissingKeys lists footprint keys the error names, when known.
	MissingKeys []string `json:"missingKeys,omitempty"`
	// CPUInstructionsUsed and CPUInstructionsLimit describe the CPU budget.
	CPUInstructionsUsed uint64 `json:"cpuInstructionsUsed,omitempty"`
	// CPUInstructionsLimit is the declared CPU budget, when known.
	CPUInstructionsLimit uint64 `json:"cpuInstructionsLimit,omitempty"`
	// MemoryBytesUsed and MemoryBytesLimit describe the memory budget.
	MemoryBytesUsed uint64 `json:"memoryBytesUsed,omitempty"`
	// MemoryBytesLimit is the declared memory budget, when known.
	MemoryBytesLimit uint64 `json:"memoryBytesLimit,omitempty"`
	// MinResourceFee is the minimum refundable fee simulation recommends, in stroops.
	MinResourceFee int64 `json:"minResourceFee,omitempty"`
}

// SimulationDiagnosis explains a simulateTransaction failure in plain English.
type SimulationDiagnosis struct {
	// Success reports whether simulation succeeded.
	Success bool `json:"success"`
	// Kind is the machine-readable root-cause category.
	Kind SimulationKind `json:"kind"`
	// Summary explains the failure in one or two sentences.
	Summary string `json:"summary"`
	// Hint suggests what to do about it. Empty on success.
	Hint string `json:"hint,omitempty"`
	// MissingKeys lists footprint keys to add, when the failure names them.
	MissingKeys []string `json:"missingKeys,omitempty"`
	// Detail carries the raw error text for log correlation.
	Detail string `json:"detail,omitempty"`
}

// DiagnoseSimulationError analyses a simulateTransaction error string and
// returns its root-cause diagnosis. Matching is case-insensitive substring
// matching over the known Soroban failure vocabulary, so it stays useful as
// RPC wording evolves: unrecognised errors yield KindUnknown rather than an
// error return. An empty error means success.
//
// The function is purely local: no network, no clock, no environment.
func DiagnoseSimulationError(errMsg string) (*SimulationDiagnosis, error) {
	trimmed := strings.TrimSpace(errMsg)
	if trimmed == "" {
		return &SimulationDiagnosis{
			Success: true,
			Kind:    SimulationKindSuccess,
			Summary: "Simulation succeeded with no reported error.",
		}, nil
	}
	lower := strings.ToLower(trimmed)
	kind := classifySimulationError(lower)
	d := &SimulationDiagnosis{
		Success: false,
		Kind:    kind,
		Detail:  trimmed,
	}
	d.Summary, d.Hint = simulationSummaryHint(kind, trimmed)
	return d, nil
}

// DiagnoseSimulationResponse diagnoses a full simulation response, preferring
// structured fields (missing keys, resource counters) over error-text
// matching when both are present.
func DiagnoseSimulationResponse(resp *SimulationResponse) (*SimulationDiagnosis, error) {
	if resp == nil {
		return nil, fmt.Errorf("%w: cannot diagnose nil simulation response", ErrInvalidArgument)
	}
	if strings.TrimSpace(resp.Error) == "" && len(resp.MissingKeys) == 0 {
		// Resource counters alone can still prove exhaustion even when the
		// error string is absent (some RPC versions omit it).
		if resp.CPUInstructionsLimit > 0 && resp.CPUInstructionsUsed > resp.CPUInstructionsLimit {
			d := &SimulationDiagnosis{Success: false, Kind: SimulationKindCPUExceeded}
			d.Summary, d.Hint = simulationSummaryHint(d.Kind, "")
			d.Detail = fmt.Sprintf("CPU instructions used %d exceed limit %d", resp.CPUInstructionsUsed, resp.CPUInstructionsLimit)
			return d, nil
		}
		if resp.MemoryBytesLimit > 0 && resp.MemoryBytesUsed > resp.MemoryBytesLimit {
			d := &SimulationDiagnosis{Success: false, Kind: SimulationKindMemoryExceeded}
			d.Summary, d.Hint = simulationSummaryHint(d.Kind, "")
			d.Detail = fmt.Sprintf("memory used %d bytes exceeds limit %d bytes", resp.MemoryBytesUsed, resp.MemoryBytesLimit)
			return d, nil
		}
		return &SimulationDiagnosis{
			Success: true,
			Kind:    SimulationKindSuccess,
			Summary: "Simulation succeeded with no reported error.",
		}, nil
	}
	d, err := DiagnoseSimulationError(resp.Error)
	if err != nil {
		return nil, err
	}
	// Structured keys beat anything guessed from prose.
	if len(resp.MissingKeys) > 0 {
		d.Kind = SimulationKindMissingFootprint
		d.MissingKeys = append([]string(nil), resp.MissingKeys...)
		d.Summary, d.Hint = simulationSummaryHint(d.Kind, resp.Error)
		if d.Detail == "" {
			d.Detail = strings.TrimSpace(resp.Error)
		}
	}
	// Attach resource detail when the counters are populated.
	if d.Kind == SimulationKindCPUExceeded && resp.CPUInstructionsLimit > 0 {
		d.Detail = fmt.Sprintf("%s (used %d of %d CPU instructions)", detailOr(d.Detail, "CPU budget exceeded"), resp.CPUInstructionsUsed, resp.CPUInstructionsLimit)
	}
	if d.Kind == SimulationKindMemoryExceeded && resp.MemoryBytesLimit > 0 {
		d.Detail = fmt.Sprintf("%s (used %d of %d memory bytes)", detailOr(d.Detail, "memory budget exceeded"), resp.MemoryBytesUsed, resp.MemoryBytesLimit)
	}
	if d.Kind == SimulationKindInsufficientFee && resp.MinResourceFee > 0 {
		d.Hint = fmt.Sprintf("%s Simulation recommends at least %d stroops of refundable fee.", d.Hint, resp.MinResourceFee)
	}
	return d, nil
}

// DiagnoseMissingFootprint builds a diagnosis for explicitly listed footprint
// keys, for callers that already extracted them from an RPC payload.
func DiagnoseMissingFootprint(missingKeys []string) (*SimulationDiagnosis, error) {
	if len(missingKeys) == 0 {
		return nil, fmt.Errorf("%w: missing footprint diagnosis requires at least one key", ErrInvalidArgument)
	}
	d := &SimulationDiagnosis{
		Success:     false,
		Kind:        SimulationKindMissingFootprint,
		MissingKeys: append([]string(nil), missingKeys...),
	}
	d.Summary, d.Hint = simulationSummaryHint(d.Kind, "")
	if len(missingKeys) == 1 {
		d.Summary = fmt.Sprintf("Simulation failed: the transaction footprint is missing ledger key %s.", missingKeys[0])
	} else {
		d.Summary = fmt.Sprintf("Simulation failed: the transaction footprint is missing %d ledger keys (%s).",
			len(missingKeys), strings.Join(missingKeys, ", "))
	}
	return d, nil
}

// DiagnoseResourceExhaustion builds a CPU/memory exhaustion diagnosis from
// used and limit counters, for callers that track budgets numerically.
func DiagnoseResourceExhaustion(cpuUsed, cpuLimit, memUsed, memLimit uint64) (*SimulationDiagnosis, error) {
	cpuOver := cpuLimit > 0 && cpuUsed > cpuLimit
	memOver := memLimit > 0 && memUsed > memLimit
	if !cpuOver && !memOver {
		return &SimulationDiagnosis{
			Success: true,
			Kind:    SimulationKindSuccess,
			Summary: "Resource usage fits inside the declared budgets.",
		}, nil
	}
	// CPU is the binding constraint in practice, so report it first when both fire.
	kind := SimulationKindMemoryExceeded
	if cpuOver {
		kind = SimulationKindCPUExceeded
	}
	d := &SimulationDiagnosis{Success: false, Kind: kind}
	d.Summary, d.Hint = simulationSummaryHint(kind, "")
	if cpuOver {
		d.Detail = fmt.Sprintf("CPU instructions used %d exceed limit %d", cpuUsed, cpuLimit)
	}
	if memOver && !cpuOver {
		d.Detail = fmt.Sprintf("memory used %d bytes exceeds limit %d bytes", memUsed, memLimit)
	}
	if cpuOver && memOver {
		d.Detail = fmt.Sprintf("CPU instructions used %d exceed limit %d; memory used %d bytes exceeds limit %d bytes",
			cpuUsed, cpuLimit, memUsed, memLimit)
	}
	return d, nil
}

func classifySimulationError(lower string) SimulationKind {
	switch {
	case containsAny(lower, []string{"missing footprint", "footprint key", "key not in footprint", "unknown ledger key", "readwrite footprint", "readonly footprint"}):
		return SimulationKindMissingFootprint
	case containsAny(lower, []string{"cpu", "instruction", "budget exceeded", "exceeded limit", "resource limit exceeded"}):
		// Disambiguate memory from CPU when both vocabularies appear.
		if containsAny(lower, []string{"memory", "mem ", "ram"}) && !containsAny(lower, []string{"cpu", "instruction"}) {
			return SimulationKindMemoryExceeded
		}
		if containsAny(lower, []string{"memory"}) && containsAny(lower, []string{"cpu", "instruction"}) {
			// Prefer the explicitly named resource when the message names one.
			if strings.Contains(lower, "memory") && strings.Index(lower, "memory") < strings.Index(lower, "cpu") {
				return SimulationKindMemoryExceeded
			}
		}
		if containsAny(lower, []string{"memory", "mem "}) {
			return SimulationKindMemoryExceeded
		}
		return SimulationKindCPUExceeded
	case containsAny(lower, []string{"insufficient refundable", "refundable fee", "min resource fee", "fee too small", "insufficient fee"}):
		return SimulationKindInsufficientFee
	case containsAny(lower, []string{"archived", "expired entry", "frozen key", "restore footprint", "needs restore", "ttl"}):
		return SimulationKindArchivedEntry
	case containsAny(lower, []string{"auth", "authorization", "signature missing", "not authorized by"}):
		// "not authorized" also appears in classic payment failures; in a
		// simulation context it still means a missing Soroban auth entry, so
		// keep the simulation-specific wording.
		return SimulationKindAuthMissing
	case containsAny(lower, []string{"trapped", "panic", "contract error", "scerror", "host error", "abort"}):
		return SimulationKindContractTrapped
	case containsAny(lower, []string{"malformed", "invalid input", "invalid footprint", "parse error", "bad request", "invalid"}):
		return SimulationKindInvalidInput
	default:
		return SimulationKindUnknown
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func simulationSummaryHint(kind SimulationKind, errMsg string) (summary, hint string) {
	errSuffix := ""
	if trimmed := strings.TrimSpace(errMsg); trimmed != "" {
		// Keep the raw text available in prose without letting a huge RPC blob dominate.
		if len(trimmed) > 220 {
			trimmed = trimmed[:220] + "..."
		}
		errSuffix = " RPC reported: " + trimmed
	}
	switch kind {
	case SimulationKindMissingFootprint:
		return "Simulation failed: the transaction footprint omits ledger keys the contract needs." + errSuffix,
			"Re-run simulation and use the footprint it returns verbatim; do not hand-edit readOnly/readWrite keys. Every key the contract reads or writes must be listed."
	case SimulationKindCPUExceeded:
		return "Simulation failed: the contract exceeded its declared CPU (instruction) budget." + errSuffix,
			"Raise the CPU instructions in the Soroban resource declaration, starting from the simulation's recommended values with headroom. If the cost keeps growing, the contract loop itself needs optimisation."
	case SimulationKindMemoryExceeded:
		return "Simulation failed: the contract exceeded its declared memory budget." + errSuffix,
			"Raise the memory bytes in the Soroban resource declaration from the simulation's recommended values. Persistently high usage means the contract holds too much state per call."
	case SimulationKindInsufficientFee:
		return "Simulation failed: the refundable fee cannot cover rent and events." + errSuffix,
			"Raise the refundable resource fee. Simulation reports the minimum required amount; add headroom for ledger growth."
	case SimulationKindArchivedEntry:
		return "Simulation failed: the footprint touches an archived (expired) ledger entry." + errSuffix,
			"Restore the entry with a restore-footprint operation before simulating again, or extend its TTL with extend-footprint-TTL."
	case SimulationKindAuthMissing:
		return "Simulation failed: required Soroban authorization entries are missing." + errSuffix,
			"Collect the missing auth entries (sign the authorizeEntry payloads with the invoking accounts) and resubmit them with the transaction."
	case SimulationKindContractTrapped:
		return "Simulation failed inside the contract itself: it panicked or returned an error." + errSuffix,
			"This is a contract bug or a bad argument, not a resource problem. Read the diagnostic events and the panic string, then fix the call or the contract."
	case SimulationKindInvalidInput:
		return "Simulation rejected the request as malformed." + errSuffix,
			"Validate the transaction envelope and the simulation request fields against the RPC schema before retrying."
	default:
		return "Simulation failed with an unrecognised error." + errSuffix,
			"Search the raw error text and re-run simulation with verbose logging; the RPC vocabulary may be newer than this build of lens."
	}
}

func detailOr(detail, fallback string) string {
	if strings.TrimSpace(detail) != "" {
		return detail
	}
	return fallback
}
