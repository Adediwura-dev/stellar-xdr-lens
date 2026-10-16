package lens

import (
	"fmt"
	"strings"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// protocol21ReasonDefinitions defines the diagnostic explanations and actionable
// hints for result codes introduced or expanded in Stellar Protocol 21 and 22.
var protocol21ReasonDefinitions = map[string]reasonText{
	// Protocol 21 State Archival Phase 2 (CAP-0056)
	"TransactionResultCodeTxFrozenKeyAccessed": bad(
		"The transaction touched a ledger key that is currently frozen.",
		"The referenced entry has expired and is frozen under Protocol 21 state archival. Restore or unfreeze the entry before re-submitting.",
	),
	// Protocol 21 Soroban Archival & Refundable Fees
	"InvokeHostFunctionResultCodeInvokeHostFunctionEntryArchived": bad(
		"The contract call accessed a ledger entry that has been archived.",
		"Restore the archived entry with a restore_footprint operation before calling again.",
	),
	"InvokeHostFunctionResultCodeInvokeHostFunctionInsufficientRefundableFee": bad(
		"The refundable fee was too small to cover rent and events generated during contract execution.",
		"Raise the refundable resource fee. Contract simulation reports the required amount.",
	),
	"ExtendFootprintTtlResultCodeExtendFootprintTtlInsufficientRefundableFee": bad(
		"The refundable fee was too small to cover the rent this extension requires.",
		"Provide a higher refundable fee to cover the TTL extension.",
	),
	"RestoreFootprintResultCodeRestoreFootprintInsufficientRefundableFee": bad(
		"The refundable fee was too small to cover the rent and entry restoration.",
		"Increase the refundable fee according to simulation results.",
	),
}

// protocol21Constants lists the constants introduced in Protocol 21 and 22.
var protocol21Constants = map[string]bool{
	"TransactionResultCodeTxFrozenKeyAccessed":                                true,
	"InvokeHostFunctionResultCodeInvokeHostFunctionEntryArchived":             true,
	"InvokeHostFunctionResultCodeInvokeHostFunctionInsufficientRefundableFee": true,
	"ExtendFootprintTtlResultCodeExtendFootprintTtlInsufficientRefundableFee": true,
	"RestoreFootprintResultCodeRestoreFootprintInsufficientRefundableFee":     true,
}

func init() {
	// Register all Protocol 21/22 diagnostic explanations into the central registry.
	for constant, text := range protocol21ReasonDefinitions {
		registerReason(constant, text)
	}
}

// IsProtocol21Code reports whether a result code (either canonical short code
// like "tx_frozen_key_accessed" or generated constant name) was introduced in
// Protocol 21 or Protocol 22.
func IsProtocol21Code(codeOrConstant string) bool {
	trimmed := strings.TrimSpace(codeOrConstant)
	if protocol21Constants[trimmed] {
		return true
	}
	code := shortCode(trimmed)
	for constant := range protocol21Constants {
		if shortCode(constant) == code || shortCode(constant) == trimmed {
			return true
		}
	}
	return false
}

// Protocol21Reasons returns a copy of all Protocol 21 and Protocol 22 diagnostic
// explanations, keyed by their canonical short code.
func Protocol21Reasons() map[string]Reason {
	res := make(map[string]Reason, len(protocol21ReasonDefinitions))
	for constant, text := range protocol21ReasonDefinitions {
		code := shortCode(constant)
		res[code] = Reason{
			Code:     code,
			Constant: constant,
			Summary:  text.summary,
			Hint:     text.hint,
			Success:  text.success,
		}
	}
	return res
}

// ExplainProtocol21Code retrieves the plain-English explanation for a Protocol 21+
// result code or constant. Returns false if the code is not a Protocol 21+ code.
func ExplainProtocol21Code(codeOrConstant string) (Reason, bool) {
	if !IsProtocol21Code(codeOrConstant) {
		return Reason{}, false
	}
	// Try short code first
	if r, ok := ReasonByCode(codeOrConstant); ok {
		return r, true
	}
	// Try constant
	r := ExplainCode(codeOrConstant)
	return r, true
}

// ExplainHostErrorType returns a descriptive plain-English category name for a
// Soroban host ScErrorType.
func ExplainHostErrorType(t xdr.ScErrorType) string {
	switch t {
	case xdr.ScErrorTypeSceContract:
		return "Contract execution error"
	case xdr.ScErrorTypeSceWasmVm:
		return "WebAssembly VM runtime error"
	case xdr.ScErrorTypeSceContext:
		return "Host context or re-entrancy violation"
	case xdr.ScErrorTypeSceStorage:
		return "Contract storage or state access error"
	case xdr.ScErrorTypeSceObject:
		return "Host object reference handle error"
	case xdr.ScErrorTypeSceCrypto:
		return "Cryptographic or signature failure"
	case xdr.ScErrorTypeSceEvents:
		return "Contract event emission error"
	case xdr.ScErrorTypeSceBudget:
		return "Resource or CPU/memory budget limit exceeded"
	case xdr.ScErrorTypeSceValue:
		return "Host value conversion or format error"
	case xdr.ScErrorTypeSceAuth:
		return "Authentication or authorization verification failure"
	default:
		return fmt.Sprintf("Unknown host error type (%d)", t)
	}
}

// ExplainHostErrorCode returns a Reason for a standard Soroban host ScErrorCode.
func ExplainHostErrorCode(code xdr.ScErrorCode) Reason {
	switch code {
	case xdr.ScErrorCodeScecArithDomain:
		return Reason{
			Code:     "scec_arith_domain",
			Constant: "ScErrorCodeScecArithDomain",
			Summary:  "Arithmetic operation was undefined, resulting in an integer overflow, underflow, or division by zero.",
			Hint:     "Check arithmetic boundaries and use checked or saturating operations in the contract.",
		}
	case xdr.ScErrorCodeScecIndexBounds:
		return Reason{
			Code:     "scec_index_bounds",
			Constant: "ScErrorCodeScecIndexBounds",
			Summary:  "An array, vector, or buffer was indexed outside its allocated boundaries.",
			Hint:     "Verify vector and slice lengths before indexing elements.",
		}
	case xdr.ScErrorCodeScecInvalidInput:
		return Reason{
			Code:     "scec_invalid_input",
			Constant: "ScErrorCodeScecInvalidInput",
			Summary:  "The caller provided invalid or malformed arguments to the host function or contract.",
			Hint:     "Check the contract's function signature, types, and input constraints.",
		}
	case xdr.ScErrorCodeScecMissingValue:
		return Reason{
			Code:     "scec_missing_value",
			Constant: "ScErrorCodeScecMissingValue",
			Summary:  "A required value, contract storage key, or instance state was not found.",
			Hint:     "Confirm the contract instance and required storage keys were properly initialized.",
		}
	case xdr.ScErrorCodeScecExistingValue:
		return Reason{
			Code:     "scec_existing_value",
			Constant: "ScErrorCodeScecExistingValue",
			Summary:  "An entity or storage key could not be created because an entry with that identifier already exists.",
			Hint:     "Use update operations or ensure identifiers are unique.",
		}
	case xdr.ScErrorCodeScecExceededLimit:
		return Reason{
			Code:     "scec_exceeded_limit",
			Constant: "ScErrorCodeScecExceededLimit",
			Summary:  "A protocol, execution, or host limit (such as max events, contract size, or depth) was exceeded.",
			Hint:     "Reduce data payload sizes or event emissions to remain within protocol limits.",
		}
	case xdr.ScErrorCodeScecInvalidAction:
		return Reason{
			Code:     "scec_invalid_action",
			Constant: "ScErrorCodeScecInvalidAction",
			Summary:  "The action requested is not permitted in the current contract or host state.",
			Hint:     "Ensure prerequisites for this operation have been satisfied.",
		}
	case xdr.ScErrorCodeScecInternalError:
		return Reason{
			Code:     "scec_internal_error",
			Constant: "ScErrorCodeScecInternalError",
			Summary:  "The Soroban host detected an unexpected internal error.",
			Hint:     "This usually indicates a host runtime issue rather than invalid contract arguments.",
		}
	case xdr.ScErrorCodeScecUnexpectedType:
		return Reason{
			Code:     "scec_unexpected_type",
			Constant: "ScErrorCodeScecUnexpectedType",
			Summary:  "The provided SCVal type does not match what the host function or contract parameter expected.",
			Hint:     "Verify the parameter types against the contract specification.",
		}
	case xdr.ScErrorCodeScecUnexpectedSize:
		return Reason{
			Code:     "scec_unexpected_size",
			Constant: "ScErrorCodeScecUnexpectedSize",
			Summary:  "A data structure's size or byte length did not match the expected specification.",
			Hint:     "Check fixed-length arrays, hashes, or address byte lengths.",
		}
	default:
		return Reason{
			Code:     fmt.Sprintf("scec_%d", code),
			Constant: fmt.Sprintf("ScErrorCode(%d)", code),
			Summary:  fmt.Sprintf("Soroban host error code %d", code),
		}
	}
}

// ExplainScError converts an XDR ScError into a human-readable Reason.
// It is panic-safe against unpopulated union arms.
func ExplainScError(err xdr.ScError) (r Reason) {
	defer func() {
		if rec := recover(); rec != nil {
			r = Reason{
				Code:     "sc_error",
				Constant: err.Type.String(),
				Summary:  fmt.Sprintf("Soroban error: %s", ExplainHostErrorType(err.Type)),
			}
		}
	}()

	typeDesc := ExplainHostErrorType(err.Type)
	switch err.Type {
	case xdr.ScErrorTypeSceContract:
		if err.ContractCode != nil {
			return Reason{
				Code:     fmt.Sprintf("contract_error_%d", *err.ContractCode),
				Constant: "ScErrorTypeSceContract",
				Summary:  fmt.Sprintf("%s: contract returned error code %d", typeDesc, *err.ContractCode),
				Hint:     "Inspect the contract's custom error enum definitions for this numeric error code.",
			}
		}
		return Reason{
			Code:     "contract_error",
			Constant: "ScErrorTypeSceContract",
			Summary:  typeDesc,
		}
	default:
		if err.Code == nil {
			return Reason{
				Code:     "host_error",
				Constant: err.Type.String(),
				Summary:  typeDesc,
			}
		}
		code := *err.Code
		codeReason := ExplainHostErrorCode(code)
		return Reason{
			Code:     codeReason.Code,
			Constant: fmt.Sprintf("%s:%s", err.Type.String(), codeReason.Constant),
			Summary:  fmt.Sprintf("%s: %s", typeDesc, codeReason.Summary),
			Hint:     codeReason.Hint,
		}
	}
}
